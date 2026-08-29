package console

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	providerpkg "github.com/deeptrols/api/internal/provider"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type modelResponse struct {
	ID              string            `json:"id"`
	Code            string            `json:"code"`
	Provider        string            `json:"provider"`
	Category        string            `json:"category"`
	DisplayName     string            `json:"display_name"`
	Description     string            `json:"description,omitempty"`
	ContextWindow   int               `json:"context_window"`
	MaxOutputTokens int               `json:"max_output_tokens,omitempty"`
	Status          string            `json:"status,omitempty"`
	Pricing         []pricingRow      `json:"pricings"`
	PricingMap      map[string]string `json:"pricing"`
}

type loginHistoryResponse struct {
	IPAddress string `json:"ip_address"`
	UserAgent string `json:"user_agent"`
	Success   bool   `json:"success"`
	CreatedAt string `json:"created_at"`
}

// HandleListModels returns all active models with their pricing information.
func HandleListModels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		models, err := a.Models.ListActive(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list models"})
			return
		}

		response := make([]modelResponse, 0, len(models))
		for _, m := range models {
			if !providerpkg.IsDomesticProvider(m.Provider) {
				continue
			}
			pricing, err := fetchModelPricing(ctx, a, m.ID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch model pricing"})
				return
			}

			response = append(response, modelResponse{
				ID:              m.ID.String(),
				Code:            m.Code,
				Provider:        m.Provider,
				Category:        string(m.Category),
				DisplayName:     m.DisplayName,
				Description:     m.Description,
				ContextWindow:   m.ContextWindow,
				MaxOutputTokens: m.MaxOutputTokens,
				Status:          string(m.Status),
				Pricing:         pricing,
				PricingMap:      pricingToMap(pricing),
			})
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

type pricingRow struct {
	Dimension  string         `json:"dimension"`
	UnitName   string         `json:"unit_name"`
	UnitPrice  string         `json:"unit_price"`
	PriceType  string         `json:"price_type"`
	Period     string         `json:"period"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

// fetchModelPricing queries model_pricing for a given model and returns pricing rows.
func fetchModelPricing(ctx context.Context, a *app.App, modelID interface{}) ([]pricingRow, error) {
	rows, err := a.Pool.Query(ctx,
		`SELECT pricing_dimension, unit_name, unit_price, price_type, period, COALESCE(conditions, '{}'::jsonb) FROM model_pricing
		 WHERE model_id = $1 AND is_active = TRUE
		 ORDER BY CASE WHEN price_type = 'sell' THEN 0 ELSE 1 END, pricing_dimension, period`, modelID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pricing []pricingRow
	for rows.Next() {
		var p pricingRow
		var condJSON []byte
		if err := rows.Scan(&p.Dimension, &p.UnitName, &p.UnitPrice, &p.PriceType, &p.Period, &condJSON); err != nil {
			return nil, err
		}
		p.UnitPrice = trimDecimalPrice(p.UnitPrice)
		if len(condJSON) > 0 && string(condJSON) != "{}" {
			if err := json.Unmarshal(condJSON, &p.Conditions); err != nil {
				return nil, fmt.Errorf("decode conditions for %s: %w", p.Dimension, err)
			}
		}
		pricing = append(pricing, p)
	}
	return pricing, rows.Err()
}

// trimDecimalPrice removes trailing zeros from a decimal price string,
// but keeps at least 2 decimal places for consistent display.
func trimDecimalPrice(s string) string {
	if !strings.Contains(s, ".") {
		s = s + ".00"
		return s
	}
	// Ensure at least 2 decimal places, then trim excess trailing zeros.
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	fracPart := parts[1]
	// Pad to at least 2 decimal places.
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	// Keep first 2 decimal places, trim additional trailing zeros.
	required := fracPart[:2]
	rest := fracPart[2:]
	rest = strings.TrimRight(rest, "0")
	if rest != "" {
		return intPart + "." + required + rest
	}
	return intPart + "." + required
}

// pricingToMap converts pricing rows into a dimension -> price map for
// display-oriented consumers (e.g. the model marketplace page). Sell rows are
// preferred; among them, off_peak is shown first for stable display.
func pricingToMap(rows []pricingRow) map[string]string {
	if len(rows) == 0 {
		return nil
	}
	m := make(map[string]string, len(rows))
	for _, p := range rows {
		if _, ok := m[p.Dimension]; ok {
			continue // first row wins (sell before cost, off_peak before peak)
		}
		m[p.Dimension] = p.UnitPrice
	}
	return m
}

// HandleLoginHistory returns login history for the authenticated user.
func HandleLoginHistory(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT ip_address, user_agent, success, created_at
			 FROM login_history
			 WHERE user_id = $1
			 ORDER BY created_at DESC
			 LIMIT 50`, userID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query login history"})
			return
		}
		defer rows.Close()

		history := make([]loginHistoryResponse, 0)
		for rows.Next() {
			var h loginHistoryResponse
			var createdAt time.Time
			if err := rows.Scan(&h.IPAddress, &h.UserAgent, &h.Success, &createdAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read login history"})
				return
			}
			h.CreatedAt = createdAt.Format(time.RFC3339)
			history = append(history, h)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate login history"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  history,
			"total": len(history),
		})
	}
}

// createModelRequest is the request body for HandleCreateModel.
type createModelRequest struct {
	Code            string                  `json:"code"`
	Provider        string                  `json:"provider"`
	Category        string                  `json:"category"`
	DisplayName     string                  `json:"display_name,omitempty"`
	Description     string                  `json:"description,omitempty"`
	ContextWindow   int                     `json:"context_window,omitempty"`
	MaxOutputTokens int                     `json:"max_output_tokens,omitempty"`
	Pricings        []createModelPricingReq `json:"pricings,omitempty"`
}

type createModelPricingReq struct {
	Dimension  string         `json:"dimension"`
	UnitName   string         `json:"unit_name"`
	UnitPrice  string         `json:"unit_price"`
	Period     string         `json:"period,omitempty"`
	PriceType  string         `json:"price_type,omitempty"`
	Conditions map[string]any `json:"conditions,omitempty"`
}

// HandleCreateModel handles POST /api/console/models.
func HandleCreateModel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		var req createModelRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Code == "" || req.Provider == "" || req.Category == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code, provider, and category are required"})
			return
		}

		if !isValidModelCategory(req.Category) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid category"})
			return
		}

		modelID := uuid.New()
		now := time.Now().UTC()
		dbCtx := r.Context()

		tx, err := a.Pool.Begin(dbCtx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
			return
		}
		defer tx.Rollback(dbCtx)

		// Check for duplicate code
		var existingID uuid.UUID
		err = tx.QueryRow(dbCtx, `SELECT id FROM models WHERE code = $1`, req.Code).Scan(&existingID)
		if err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Model code already exists"})
			return
		}

		displayName := req.DisplayName
		if displayName == "" {
			displayName = req.Code
		}

		_, err = tx.Exec(dbCtx,
			`INSERT INTO models (id, code, provider, category, display_name, description, context_window, max_output_tokens, status, release_stage, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active', 'GA', $9, $9)`,
			modelID, req.Code, req.Provider, req.Category, displayName, req.Description, req.ContextWindow, req.MaxOutputTokens, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create model"})
			return
		}

		for _, p := range req.Pricings {
			conditions, err := pricingConditionsForInsert(p)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			_, err = tx.Exec(dbCtx,
				`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active, period, price_type, conditions, created_at, updated_at)
				 VALUES ($1, $2, $3, $4, $5, $6, 'CNY', TRUE, COALESCE(NULLIF($7, ''), 'off_peak'), COALESCE(NULLIF($8, ''), 'sell'), $9, $10, $10)`,
				uuid.New(), modelID, req.Category, p.Dimension, p.UnitName, p.UnitPrice, p.Period, normalizePricingType(p.PriceType), conditions, now,
			)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to insert model pricing"})
				return
			}
		}

		if err := tx.Commit(dbCtx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": map[string]interface{}{
				"id":             modelID.String(),
				"code":           req.Code,
				"provider":       req.Provider,
				"category":       req.Category,
				"display_name":   displayName,
				"context_window": req.ContextWindow,
				"status":         "active",
				"created_at":     now.Format(time.RFC3339),
			},
		})
	}
}

// HandleUpdateModel handles PUT /api/console/models/{id}.
func HandleUpdateModel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		modelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model ID"})
			return
		}

		dbCtx := r.Context()
		model, err := a.Models.FindByID(dbCtx, modelID)
		if err != nil || model == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Model not found"})
			return
		}

		var req struct {
			DisplayName     *string                 `json:"display_name,omitempty"`
			Description     *string                 `json:"description,omitempty"`
			Status          *string                 `json:"status,omitempty"`
			Provider        *string                 `json:"provider,omitempty"`
			ContextWindow   *int                    `json:"context_window,omitempty"`
			MaxOutputTokens *int                    `json:"max_output_tokens,omitempty"`
			Pricings        []createModelPricingReq `json:"pricings,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		tx, err := a.Pool.Begin(dbCtx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to begin transaction"})
			return
		}
		defer tx.Rollback(dbCtx)

		now := time.Now().UTC()

		// Snapshot the current sell prices for the audit trail before the
		// mutation so price changes are attributable (who changed what).
		var beforePricing []map[string]string
		priceRows, qerr := tx.Query(dbCtx,
			`SELECT pricing_dimension, unit_price, period FROM model_pricing WHERE model_id = $1 AND price_type = 'sell' AND is_active = TRUE`,
			modelID)
		if qerr == nil {
			for priceRows.Next() {
				var dim, price, period string
				if err := priceRows.Scan(&dim, &price, &period); err == nil {
					beforePricing = append(beforePricing, map[string]string{"dimension": dim, "unit_price": price, "period": period})
				}
			}
			priceRows.Close()
		}
		middleware.SetAuditOldValue(r.Context(), map[string]any{
			"model_id": modelID.String(),
			"pricing":  beforePricing,
		})

		// Update model fields
		displayName := model.DisplayName
		if req.DisplayName != nil {
			displayName = *req.DisplayName
		}
		status := string(model.Status)
		if req.Status != nil {
			status = *req.Status
		}
		contextWindow := model.ContextWindow
		if req.ContextWindow != nil {
			contextWindow = *req.ContextWindow
		}
		pv := model.Provider
		if req.Provider != nil {
			pv = *req.Provider
		}

		_, err = tx.Exec(dbCtx,
			`UPDATE models SET display_name = $1, provider = $6, status = $2, context_window = $3, updated_at = $4 WHERE id = $5`,
			displayName, status, contextWindow, now, modelID, pv,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update model"})
			return
		}
		if req.Description != nil || req.MaxOutputTokens != nil {
			_, err = tx.Exec(dbCtx,
				`UPDATE models SET description = COALESCE($1, description), max_output_tokens = COALESCE($2, max_output_tokens), updated_at = $3 WHERE id = $4`,
				req.Description, req.MaxOutputTokens, now, modelID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update model metadata"})
				return
			}
		}

		// Replace sell pricing rows. Cost rows (price_type='cost') are managed
		// by provider cost sync and must survive model edits.
		if req.Pricings != nil {
			_, err = tx.Exec(dbCtx, `DELETE FROM model_pricing WHERE model_id = $1 AND price_type = 'sell'`, modelID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete old pricing"})
				return
			}

			for _, p := range req.Pricings {
				if p.PriceType == domain.PriceTypeCost {
					// Cost rows are owned by provider cost sync; a model edit
					// must never re-insert them as sell rows.
					continue
				}
				conditions, err := pricingConditionsForInsert(p)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				_, err = tx.Exec(dbCtx,
					`INSERT INTO model_pricing (id, model_id, request_type, pricing_dimension, unit_name, unit_price, currency, is_active, period, price_type, conditions, price_version, created_at, updated_at)
					 VALUES ($1, $2, $3, $4, $5, $6, 'CNY', TRUE, COALESCE(NULLIF($7, ''), 'off_peak'), $9, $10,
					         (SELECT COALESCE(MAX(price_version), 0) + 1 FROM model_pricing WHERE model_id = $2), $8, $8)`,
					uuid.New(), modelID, string(model.Category), p.Dimension, p.UnitName, p.UnitPrice, p.Period, now, normalizePricingType(p.PriceType), conditions,
				)
				if err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to insert new pricing"})
					return
				}
			}
		}

		if err := tx.Commit(dbCtx); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit transaction"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"id":             model.ID.String(),
				"code":           model.Code,
				"provider":       model.Provider,
				"category":       string(model.Category),
				"display_name":   displayName,
				"context_window": contextWindow,
				"status":         status,
				"updated_at":     now.Format(time.RFC3339),
			},
		})
	}
}

// HandleDeleteModel handles DELETE /api/console/models/{id}.
func HandleDeleteModel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		modelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model ID"})
			return
		}

		dbCtx := r.Context()
		model, err := a.Models.FindByID(dbCtx, modelID)
		if err != nil || model == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Model not found"})
			return
		}

		now := time.Now().UTC()
		_, err = a.Pool.Exec(dbCtx,
			`UPDATE models SET status = 'inactive', updated_at = $1 WHERE id = $2`,
			now, modelID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete model"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "deleted",
			"id":     modelID.String(),
		})
	}
}

// HandleGetModel handles GET /api/console/models/{id}.
func HandleGetModel(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		modelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model ID"})
			return
		}

		dbCtx := r.Context()
		model, err := a.Models.FindByID(dbCtx, modelID)
		if err != nil || model == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Model not found"})
			return
		}

		pricing, err := fetchModelPricing(dbCtx, a, modelID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch model pricing"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": map[string]interface{}{
				"id":             model.ID.String(),
				"code":           model.Code,
				"provider":       model.Provider,
				"category":       string(model.Category),
				"display_name":   model.DisplayName,
				"description":    model.Description,
				"context_window": model.ContextWindow,
				"status":         string(model.Status),
				"release_stage":  string(model.ReleaseStage),
				"pricings":       pricing,
				"pricing":        pricingToMap(pricing),
				"created_at":     model.CreatedAt.Format(time.RFC3339),
				"updated_at":     model.UpdatedAt.Format(time.RFC3339),
			},
		})
	}
}

// isValidModelCategory checks if the given category is a valid domain.ModelCategory.
func isValidModelCategory(category string) bool {
	switch domain.ModelCategory(category) {
	case domain.ModelCategoryChat, domain.ModelCategoryEmbedding,
		domain.ModelCategoryImage, domain.ModelCategoryAudio, domain.ModelCategoryVideo:
		return true
	default:
		return false
	}
}

// normalizePricingType maps a client-supplied price type onto the two allowed
// values, defaulting to sell (the type admin editors actually maintain).
func normalizePricingType(t string) string {
	switch t {
	case domain.PriceTypeCost, domain.PriceTypeSell:
		return t
	default:
		return domain.PriceTypeSell
	}
}

// pricingConditionsForInsert validates tier conditions and serializes them for
// JSONB storage. Empty conditions are stored as NULL so the platform unique
// index (COALESCE(conditions, '{}')) still treats them as the generic row.
func pricingConditionsForInsert(p createModelPricingReq) (any, error) {
	if len(p.Conditions) == 0 {
		return nil, nil
	}
	if err := validatePricingConditions(p.Conditions); err != nil {
		return nil, fmt.Errorf("invalid pricing conditions for %s: %w", p.Dimension, err)
	}
	b, err := json.Marshal(p.Conditions)
	if err != nil {
		return nil, fmt.Errorf("encode pricing conditions for %s: %w", p.Dimension, err)
	}
	return string(b), nil
}

// validatePricingConditions enforces the conditions the billing engine
// actually understands (inclusive total-token bounds) and rejects unknown
// keys, so a typo cannot silently disable a price tier.
func validatePricingConditions(c map[string]any) error {
	var min, max int64
	hasMin, hasMax := false, false
	for k, v := range c {
		switch k {
		case "min_total_tokens":
			n, ok := conditionInt(v)
			if !ok {
				return fmt.Errorf("min_total_tokens must be a non-negative integer")
			}
			min, hasMin = n, true
		case "max_total_tokens":
			n, ok := conditionInt(v)
			if !ok {
				return fmt.Errorf("max_total_tokens must be a non-negative integer")
			}
			max, hasMax = n, true
		default:
			return fmt.Errorf("unsupported condition key %q", k)
		}
	}
	if hasMin && hasMax && min > max {
		return fmt.Errorf("min_total_tokens must not exceed max_total_tokens")
	}
	return nil
}

func conditionInt(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 || math.Trunc(n) != n {
			return 0, false
		}
		return int64(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return n, true
	case int:
		if n < 0 {
			return 0, false
		}
		return int64(n), true
	case string:
		parsed, err := strconv.ParseInt(n, 10, 64)
		if err != nil || parsed < 0 {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}
