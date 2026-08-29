package console

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	providerpkg "github.com/deeptrols/api/internal/provider"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
)

const catalogModelsURL = "https://basellm.github.io/llm-metadata/api/newapi/models.json"

type catalogModel struct {
	Description     *string  `json:"description"`
	Icon            *string  `json:"icon"`
	ModelName       string   `json:"model_name"`
	PricePerMInput  *float64 `json:"price_per_m_input"`
	PricePerMOutput *float64 `json:"price_per_m_output"`
	PricePerMCacheR *float64 `json:"price_per_m_cache_read"`
	PricePerMCacheW *float64 `json:"price_per_m_cache_write"`
	Status          *int     `json:"status"`
	Tags            *string  `json:"tags"`
	VendorName      *string  `json:"vendor_name"`
}

type catalogEnvelope struct {
	Data []catalogModel `json:"data"`
}

type overwriteField struct {
	ModelName string   `json:"model_name"`
	Fields    []string `json:"fields"`
}

func fetchCatalog(ctx context.Context) ([]catalogModel, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, catalogModelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, &catalogHTTPError{status: resp.StatusCode, body: strings.TrimSpace(string(body))}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	// Try envelope {"data":[...]} first, then a bare array.
	var env catalogEnvelope
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Data) > 0 {
		return env.Data, nil
	}
	var arr []catalogModel
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	return nil, &catalogHTTPError{status: 0, body: "unexpected catalog payload"}
}

type catalogHTTPError struct {
	status int
	body   string
}

func (e *catalogHTTPError) Error() string {
	if e.status != 0 {
		return "catalog: HTTP " + http.StatusText(e.status)
	}
	return "catalog: " + e.body
}

// HandleCatalogPreview fetches the upstream model catalog and marks which codes
// already exist in the platform model catalog.
func HandleCatalogPreview(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		models, err := fetchCatalog(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		existing := map[string]bool{}
		if rows, err := a.Pool.Query(r.Context(), `SELECT code FROM models`); err == nil {
			defer rows.Close()
			for rows.Next() {
				var c string
				if rows.Scan(&c) == nil {
					existing[c] = true
				}
			}
		}
		items := make([]map[string]any, 0, len(models))
		for _, m := range models {
			if m.ModelName == "" || !providerpkg.IsDomesticProvider(strOr(m.VendorName)) {
				continue
			}
			items = append(items, map[string]any{
				"model_name":  m.ModelName,
				"vendor":      strOr(m.VendorName),
				"exists":      existing[m.ModelName],
				"description": strOr(m.Description),
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": items, "total": len(items)})
	}
}

// HandleCatalogSync imports missing models from the upstream catalog into the
// platform model catalog (with catalog prices as default pricing rows).
func HandleCatalogSync(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var raw struct {
			Overwrite json.RawMessage `json:"overwrite"`
		}
		_ = json.NewDecoder(r.Body).Decode(&raw)
		blanket := false
		var fieldOverwrites []overwriteField
		if len(raw.Overwrite) > 0 && string(raw.Overwrite) != "null" {
			var b bool
			if json.Unmarshal(raw.Overwrite, &b) == nil {
				blanket = b
			} else {
				_ = json.Unmarshal(raw.Overwrite, &fieldOverwrites)
			}
		}
		allowOverwrite := func(modelName, field string) bool {
			if blanket {
				return true
			}
			for _, fo := range fieldOverwrites {
				if fo.ModelName != modelName {
					continue
				}
				if len(fo.Fields) == 0 {
					return true
				}
				for _, f := range fo.Fields {
					if f == field {
						return true
					}
				}
			}
			return false
		}
		shouldUpdateModel := func(modelName string) bool {
			if blanket {
				return true
			}
			for _, fo := range fieldOverwrites {
				if fo.ModelName == modelName {
					return true
				}
			}
			return false
		}
		models, err := fetchCatalog(r.Context())
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		tx, err := a.Pool.Begin(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start tx"})
			return
		}
		defer tx.Rollback(r.Context())

		existing := map[string]bool{}
		if rows, err := tx.Query(r.Context(), `SELECT code FROM models`); err == nil {
			for rows.Next() {
				var c string
				if rows.Scan(&c) == nil {
					existing[c] = true
				}
			}
			rows.Close()
		}

		created := 0
		updated := 0
		createdList := []string{}
		for _, m := range models {
			if m.ModelName == "" {
				continue
			}
			if !providerpkg.IsDomesticProvider(strOr(m.VendorName)) {
				continue
			}
			if existing[m.ModelName] {
				if shouldUpdateModel(m.ModelName) {
					updateCatalogModel(tx, r.Context(), m, func(field string) bool { return allowOverwrite(m.ModelName, field) })
					updated++
				}
				continue
			}
			provider := strings.ToLower(strOr(m.VendorName))
			if provider == "" {
				provider = "other"
			}
			status := "active"
			if m.Status != nil && *m.Status != 1 {
				status = "inactive"
			}
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO models (code, provider, category, display_name, description, status, release_stage)
				 VALUES ($1, $2, 'chat', $1, $3, $4, 'GA')`,
				m.ModelName, provider, strOr(m.Description), status); err != nil {
				log.Printf("console: catalog sync: insert model %s: %v", m.ModelName, err)
				continue
			}
			// Find the inserted model id.
			var modelID string
			_ = tx.QueryRow(r.Context(), `SELECT id::text FROM models WHERE code = $1`, m.ModelName).Scan(&modelID)
			insertCatalogPricing(tx, r.Context(), modelID, m)
			created++
			createdList = append(createdList, m.ModelName)
			existing[m.ModelName] = true
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"created": created, "updated": updated, "created_list": createdList, "total": len(models)})
	}
}

// updateCatalogModel refreshes an existing model's description and sell pricing
// dims from the catalog. Pricing rows are updated in place (or inserted if the
// dimension is missing), never removed, so admin edits to other dims survive.
func updateCatalogModel(tx pgx.Tx, ctx context.Context, m catalogModel, allow func(field string) bool) {
	var modelID string
	if err := tx.QueryRow(ctx, `SELECT id::text FROM models WHERE code = $1`, m.ModelName).Scan(&modelID); err != nil || modelID == "" {
		return
	}
	if allow("description") {
		if _, err := tx.Exec(ctx, `UPDATE models SET description = $1 WHERE id = $2`, strOr(m.Description), modelID); err != nil {
			log.Printf("console: catalog sync: update desc %s: %v", m.ModelName, err)
		}
	}
	dims := []struct {
		dim string
		p   *float64
	}{
		{"input", m.PricePerMInput},
		{"output", m.PricePerMOutput},
		{"cache_read", m.PricePerMCacheR},
		{"cache_write", m.PricePerMCacheW},
	}
	for _, d := range dims {
		if !allow(d.dim) {
			continue
		}
		tag, err := tx.Exec(ctx,
			`UPDATE model_pricing SET unit_price = $1 WHERE model_id = $2 AND request_type = 'chat' AND pricing_dimension = $3 AND price_type = 'sell'`,
			price(d.p), modelID, d.dim)
		if err == nil && tag.RowsAffected() == 0 {
			if _, err := tx.Exec(ctx,
				`INSERT INTO model_pricing (model_id, request_type, pricing_dimension, unit_name, unit_price, price_type, is_active)
				 VALUES ($1, 'chat', $2, '1M tokens', $3, 'sell', true)`, modelID, d.dim, price(d.p)); err != nil {
				log.Printf("console: catalog sync: insert pricing %s: %v", d.dim, err)
			}
		}
	}
}

func insertCatalogPricing(tx pgx.Tx, ctx context.Context, modelID string, m catalogModel) {
	if modelID == "" {
		return
	}
	dims := []struct {
		dim string
		p   *float64
	}{
		{"input", m.PricePerMInput},
		{"output", m.PricePerMOutput},
		{"cache_read", m.PricePerMCacheR},
		{"cache_write", m.PricePerMCacheW},
	}
	for _, d := range dims {
		if _, err := tx.Exec(ctx,
			`INSERT INTO model_pricing (model_id, request_type, pricing_dimension, unit_name, unit_price, is_active)
			 VALUES ($1, 'chat', $2, '1M tokens', $3, true)`,
			modelID, d.dim, price(d.p)); err != nil {
			log.Printf("console: catalog sync: insert pricing %s: %v", d.dim, err)
		}
	}
}

func strOr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func price(v *float64) string {
	if v == nil {
		return "0"
	}
	return decimal.NewFromFloat(*v).String()
}
