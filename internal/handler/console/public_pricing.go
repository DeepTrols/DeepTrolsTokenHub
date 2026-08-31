package console

import (
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	providerpkg "github.com/deeptrols/api/internal/provider"
)

// HandlePublicPricing returns active domestic models with their pricing for
// the unauthenticated /pricing page. No sensitive
// fields are exposed: only catalog + pricing rows.
func HandlePublicPricing(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !settingBool(a, r, "models_public_visible") {
			writeJSON(w, http.StatusOK, map[string]any{"data": []any{}, "total": 0})
			return
		}
		models, err := a.Models.ListActive(r.Context())
		if err != nil {
			log.Printf("console: public pricing models: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load models"})
			return
		}
		response := make([]modelResponse, 0, len(models))
		for _, m := range models {
			if !providerpkg.IsDomesticProvider(m.Provider) {
				continue
			}
			pricing, err := fetchModelPricing(r.Context(), a, m.ID)
			if err != nil {
				log.Printf("console: public pricing model %s: %v", m.Code, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load pricing"})
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
		writeJSON(w, http.StatusOK, map[string]any{"data": response, "total": len(response)})
	}
}
