package console

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
)

// HandlePublicSite returns the unauthenticated brand payload for login page
// and frontend bootstrapping.
func HandlePublicSite(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, err := a.Settings.PublicSite(r.Context())
		if err != nil {
			log.Printf("console: public site: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load site info"})
			return
		}
		writeJSON(w, http.StatusOK, site)
	}
}

// HandleGetSiteSettings returns the full site & branding config (admin).
func HandleGetSiteSettings(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := a.Settings.All(r.Context())
		if err != nil {
			log.Printf("console: get site settings: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load site settings"})
			return
		}
		writeJSON(w, http.StatusOK, all)
	}
}

// HandleUpdateSiteSettings validates and persists site & branding config.
func HandleUpdateSiteSettings(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var kv map[string]string
		if err := json.NewDecoder(r.Body).Decode(&kv); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if len(kv) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No settings provided"})
			return
		}
		if err := a.Settings.Update(r.Context(), kv); err != nil {
			log.Printf("console: update site settings: %v", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
