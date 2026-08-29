package console

import (
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/google/uuid"
)

// HandleBatchTestChannels probes every active channel's first active instance
// and returns per-channel connectivity results (new-api batch-channel-test).
func HandleBatchTestChannels(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		rows, err := a.Pool.Query(r.Context(),
			`SELECT c.id, c.name, c.model_id FROM channels c WHERE c.status = 'active' ORDER BY c.name`)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query channels"})
			return
		}
		type chRow struct {
			id, name, modelID string
		}
		var channels []chRow
		for rows.Next() {
			var c chRow
			if rows.Scan(&c.id, &c.name, &c.modelID) == nil {
				channels = append(channels, c)
			}
		}
		rows.Close()

		results := make([]map[string]any, 0, len(channels))
		for _, c := range channels {
			res := map[string]any{"id": c.id, "name": c.name, "ok": false, "ms": 0, "models": 0, "error": "no active instance"}
			chID, err := uuid.Parse(c.id)
			if err != nil {
				res["error"] = err.Error()
				results = append(results, res)
				continue
			}
			modelID, _ := uuid.Parse(c.modelID)
			insts, err := a.Channels.ListInstances(r.Context(), chID)
			if err != nil {
				res["error"] = err.Error()
				results = append(results, res)
				continue
			}
			inst := pickActiveInstance(insts)
			if inst == nil {
				results = append(results, res)
				continue
			}
			provider := "openai"
			if m, err := a.Models.FindByID(r.Context(), modelID); err == nil {
				provider = m.Provider
			}
			models, err := discoverModels(provider, inst.BaseURL, instanceAPIKey(inst))
			if err != nil {
				res["error"] = err.Error()
			} else {
				res["ok"] = true
				res["models"] = len(models)
				res["error"] = ""
			}
			results = append(results, res)
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(results)})
	}
}
