package console

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/service/setting"
)

// HandleChatPresets returns the configured external chat-client presets
// (new-api chat2link parity). The admin writes them as a JSON array under the
// `chat_presets` system setting; this endpoint tolerates both a raw JSON array
// and the JSON-string-wrapped form persisted by the settings API.
func HandleChatPresets(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := a.Settings.All(r.Context())
		if err != nil {
			log.Printf("console: chat presets: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load chat presets"})
			return
		}
		raw, ok := all[setting.KeyChatPresets]
		if !ok {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		var presets []map[string]string
		if err := json.Unmarshal(raw, &presets); err != nil {
			// Fall back to JSON-string-wrapped array (settings API form).
			var s string
			if json.Unmarshal(raw, &s) == nil {
				if json.Unmarshal([]byte(s), &presets) != nil {
					presets = nil
				}
			}
		}
		if presets == nil {
			presets = []map[string]string{}
		}
		writeJSON(w, http.StatusOK, presets)
	}
}
