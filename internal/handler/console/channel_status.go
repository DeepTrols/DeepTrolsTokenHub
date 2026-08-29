package console

import (
	"encoding/json"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// HandleSetChannelStatus toggles a channel between active and inactive
// (new-api channel enable/disable semantics for the channels table).
func HandleSetChannelStatus(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Status != "active" && req.Status != "inactive" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or inactive"})
			return
		}
		tag, err := a.Pool.Exec(r.Context(),
			`UPDATE channels SET status = $1, updated_at = NOW() WHERE id = $2`,
			req.Status, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update channel status"})
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Channel not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": req.Status})
	}
}

// HandleSetProviderStatus toggles every channel that shares a provider
// credential (base_url + api_key) between active and inactive.
func HandleSetProviderStatus(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid provider id"})
			return
		}
		var req struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Status != "active" && req.Status != "inactive" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "status must be active or inactive"})
			return
		}
		tag, err := a.Pool.Exec(r.Context(),
			`UPDATE channels SET status = $1, updated_at = NOW()
			 WHERE id IN (
			   SELECT ci2.channel_id FROM channel_instances ci1
			   JOIN channel_instances ci2
			     ON ci2.base_url = ci1.base_url
			    AND ci2.config->>'api_key' = ci1.config->>'api_key'
			   WHERE ci1.channel_id = $2
			 )`, req.Status, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update provider status"})
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Provider not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": req.Status})
	}
}
