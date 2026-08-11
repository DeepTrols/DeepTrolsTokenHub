package console

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type usageLogResponse struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	RequestID    string `json:"request_id"`
	APIKeyID     string `json:"api_key_id"`
	APIKeyName   string `json:"api_key_name"`
	Status       string `json:"status"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Cost         string `json:"cost"`
	CreatedAt    string `json:"created_at"`
}

// HandleListUsage returns usage logs for the authenticated user.
func HandleListUsage(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		filter := usage.UsageFilter{
			ModelCode: r.URL.Query().Get("model"),
			Status:    r.URL.Query().Get("status"),
			Limit:     20,
			Offset:    0,
		}

		if k := r.URL.Query().Get("api_key_id"); k != "" {
			if _, err := uuid.Parse(k); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid api_key_id"})
				return
			}
			filter.APIKeyID = k
		}

		if f := r.URL.Query().Get("from"); f != "" {
			t, err := time.Parse(time.RFC3339, f)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid from"})
				return
			}
			filter.From = t.Format(time.RFC3339)
		}
		if t := r.URL.Query().Get("to"); t != "" {
			parsed, err := time.Parse(time.RFC3339, t)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid to"})
				return
			}
			filter.To = parsed.Format(time.RFC3339)
		}

		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				filter.Limit = parsed
			}
		}
		if o := r.URL.Query().Get("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				filter.Offset = parsed
			}
		}

		logs, total, err := a.Usage.ListByUser(r.Context(), userID, filter)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list usage logs"})
			return
		}

		keyNames, err := resolveAPIKeyNames(r.Context(), a, userID, logs)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to resolve API key names"})
			return
		}

		response := make([]usageLogResponse, 0, len(logs))
		for _, l := range logs {
			var inputTokens, outputTokens int64
			if l.UsageNormalized != nil {
				if t, ok := getFloatFromMap(l.UsageNormalized, "input_tokens"); ok {
					inputTokens = int64(t)
				}
				if t, ok := getFloatFromMap(l.UsageNormalized, "output_tokens"); ok {
					outputTokens = int64(t)
				}
			}

			response = append(response, usageLogResponse{
				ID:           l.ID.String(),
				Model:        l.PublicModelCode,
				RequestID:    l.RequestID,
				APIKeyID:     l.APIKeyID.String(),
				APIKeyName:   keyNames[l.APIKeyID],
				Status:       string(l.Status),
				InputTokens:  inputTokens,
				OutputTokens: outputTokens,
				Cost:         l.FinalCost.String(),
				CreatedAt:    l.CreatedAt.Format(time.RFC3339),
			})
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": total,
		})
	}
}

// resolveAPIKeyNames returns the display names for the API keys referenced by
// the given usage logs, keyed by API key ID. Keys without a matching api_keys
// row for the user resolve to an empty string. Scoping the lookup to userID
// keeps a caller from learning another user's key names.
func resolveAPIKeyNames(ctx context.Context, a *app.App, userID uuid.UUID, logs []domain.UsageLog) (map[uuid.UUID]string, error) {
	names := make(map[uuid.UUID]string, len(logs))
	ids := make([]uuid.UUID, 0, len(logs))
	for _, l := range logs {
		if _, seen := names[l.APIKeyID]; seen {
			continue
		}
		names[l.APIKeyID] = ""
		ids = append(ids, l.APIKeyID)
	}
	if len(ids) == 0 {
		return names, nil
	}

	rows, err := a.Pool.Query(ctx,
		`SELECT id, name FROM api_keys WHERE id = ANY($1) AND user_id = $2`, ids, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve api key names: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("resolve api key names scan: %w", err)
		}
		names[id] = name
	}
	return names, rows.Err()
}

// getFloatFromMap extracts a float64 value from a map by key.
func getFloatFromMap(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := toFloat64(v)
	return f, ok
}

// toFloat64 attempts to convert various numeric types to float64.
func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	default:
		return 0, false
	}
}

type chargeLineResponse struct {
	Dimension   string `json:"dimension"`
	UnitName    string `json:"unit_name"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	LineCost    string `json:"line_cost"`
	PriceSource string `json:"price_source,omitempty"`
}

// HandleGetUsageChargeLines returns the per-dimension charge lines for a
// usage log owned by the authenticated user.
func HandleGetUsageChargeLines(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		logID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid usage log ID"})
			return
		}

		// Verify the usage log belongs to the authenticated user.
		var ownerID uuid.UUID
		err = a.Pool.QueryRow(r.Context(),
			`SELECT user_id FROM usage_logs WHERE id = $1`, logID).Scan(&ownerID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Usage log not found"})
			return
		}
		if ownerID != userID {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Access denied"})
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT dimension, unit_name, quantity, unit_price, line_cost, COALESCE(price_source, '')
			 FROM charge_lines WHERE usage_log_id = $1 ORDER BY dimension`, logID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query charge lines"})
			return
		}
		defer rows.Close()

		var lines []chargeLineResponse
		for rows.Next() {
			var l chargeLineResponse
			var unitPrice, lineCost string
			if err := rows.Scan(&l.Dimension, &l.UnitName, &l.Quantity, &unitPrice, &lineCost, &l.PriceSource); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read charge line"})
				return
			}
			l.UnitPrice = unitPrice
			l.LineCost = lineCost
			lines = append(lines, l)
		}
		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate charge lines"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": lines, "total": len(lines),
		})
	}
}
