package console

import (
	"net/http"
	"strings"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// HandleProviderModelsPreview discovers upstream models for a provider
// (credential) and marks which already exist in the platform model catalog.
func HandleProviderModelsPreview(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		providerID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider ID"})
			return
		}
		var pv, baseURL, apiKey string
		err = a.Pool.QueryRow(r.Context(),
			`SELECT ci.config->>'provider', ci.base_url, ci.config->>'api_key'
			 FROM channel_instances ci JOIN channels ch ON ci.channel_id = ch.id
			 WHERE ch.id = $1 AND ci.status = 'active' LIMIT 1`, providerID).Scan(&pv, &baseURL, &apiKey)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Provider not found"})
			return
		}
		if baseURL == "" {
			if u, ok := defaultBaseURLs[pv]; ok {
				baseURL = u
			}
		}
		discovered, err := discoverModelsFn(pv, baseURL, apiKey)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Model discovery failed: " + err.Error()})
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
		items := []map[string]any{}
		for _, m := range discovered {
			if !matchesProvider(pv, strings.ToLower(m.ID)) {
				continue
			}
			items = append(items, map[string]any{"id": m.ID, "exists": existing[m.ID]})
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": items})
	}
}
