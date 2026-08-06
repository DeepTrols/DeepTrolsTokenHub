package console

import (
	"encoding/json"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/shopspring/decimal"
)

// HandleSetMarkup sets a global markup rate for all model pricing:
// unit_price = upstream_cost * markup_rate. markup_rate must be >= 1.
// Admin only. This is the self-operated platform's profit lever.
func HandleSetMarkup(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req struct {
			MarkupRate string `json:"markup_rate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		rate, err := decimal.NewFromString(req.MarkupRate)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid markup rate"})
			return
		}
		if rate.LessThan(decimal.NewFromInt(1)) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Markup rate must be >= 1"})
			return
		}

		// Update every pricing row: unit_price = upstream_cost * rate.
		// Only rows with a known upstream_cost are updated; zero-cost rows
		// keep a flat price (1x) to avoid zeroing them.
		tag, err := a.Pool.Exec(r.Context(),
			`UPDATE model_pricing
			 SET unit_price = ROUND(upstream_cost * $1, 6), updated_at = NOW()
			 WHERE upstream_cost > 0`,
			rate)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to apply markup"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"status":       "updated",
			"markup_rate":  rate.String(),
			"rows_updated": tag.RowsAffected(),
		})
	}
}

// HandleGetMarkup returns the current pricing overview for admin visibility.
func HandleGetMarkup(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var pricingRows, models int
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM model_pricing`).Scan(&pricingRows); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query pricing"})
			return
		}
		if err := a.Pool.QueryRow(r.Context(),
			`SELECT COUNT(*) FROM models WHERE status = 'active'`).Scan(&models); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query models"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"pricing_rows":  pricingRows,
			"active_models": models,
		})
	}
}
