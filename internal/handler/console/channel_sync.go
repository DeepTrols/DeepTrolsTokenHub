package console

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func resolveChannel(a *app.App, r *http.Request, id uuid.UUID) (*domain.Channel, []domain.ChannelInstance, error) {
	ch, err := a.Channels.FindByID(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	insts, err := a.Channels.ListInstances(r.Context(), id)
	if err != nil {
		return nil, nil, err
	}
	return ch, insts, nil
}

func pickActiveInstance(insts []domain.ChannelInstance) *domain.ChannelInstance {
	for i := range insts {
		if insts[i].Status == domain.InstanceStatusActive {
			return &insts[i]
		}
	}
	return nil
}

func instanceAPIKey(inst *domain.ChannelInstance) string {
	if inst == nil || inst.Config == nil {
		return ""
	}
	if v, ok := inst.Config["api_key"].(string); ok {
		return v
	}
	return ""
}

func channelProvider(a *app.App, r *http.Request, ch *domain.Channel) string {
	if ch.ModelID != uuid.Nil {
		if m, err := a.Models.FindByID(r.Context(), ch.ModelID); err == nil {
			return m.Provider
		}
	}
	return "openai"
}

// HandleChannelTest probes a channel instance's upstream /v1/models and reports
// latency + model count (real connectivity check) without generating usage.
func HandleChannelTest(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		ch, insts, err := resolveChannel(a, r, id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Channel not found"})
			return
		}
		inst := pickActiveInstance(insts)
		if inst == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No active instance"})
			return
		}
		provider := channelProvider(a, r, ch)
		start := time.Now()
		models, err := discoverModels(provider, inst.BaseURL, instanceAPIKey(inst))
		ms := time.Since(start).Milliseconds()
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "ms": ms, "models": 0, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ms": ms, "models": len(models)})
	}
}

type bindingView struct {
	ModelID  uuid.UUID
	Code     string
	Enabled  bool
	Upstream string
}

func loadBindings(a *app.App, r *http.Request, channelID uuid.UUID) (map[string]bindingView, error) {
	rows, err := a.Pool.Query(r.Context(),
		`SELECT cm.upstream_model, cm.enabled, cm.model_id, m.code
		 FROM channel_models cm JOIN models m ON m.id = cm.model_id
		 WHERE cm.channel_id = $1`, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bindingView{}
	for rows.Next() {
		var b bindingView
		if err := rows.Scan(&b.Upstream, &b.Enabled, &b.ModelID, &b.Code); err != nil {
			return nil, err
		}
		out[b.Upstream] = b
	}
	return out, rows.Err()
}

type previewItem struct {
	Upstream string `json:"upstream"`
	Code     string `json:"code"`
	ModelID  string `json:"model_id"`
	Status   string `json:"status"` // new | bound | disabled
	Enabled  bool   `json:"enabled"`
}

// classifyPreview is pure and unit-testable: groups discovered upstream models
// as new / bound / disabled based on existing channel_models bindings.
func classifyPreview(upstream []string, bindings map[string]bindingView) []previewItem {
	seen := map[string]bool{}
	items := []previewItem{}
	for _, id := range upstream {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		item := previewItem{Upstream: id, Code: id}
		if b, ok := bindings[id]; ok {
			item.Code = b.Code
			item.ModelID = b.ModelID.String()
			item.Enabled = b.Enabled
			if b.Enabled {
				item.Status = "bound"
			} else {
				item.Status = "disabled"
			}
		} else {
			item.Status = "new"
		}
		items = append(items, item)
	}
	return items
}

// HandleChannelModelsPreview discovers upstream models and classifies them
// against existing bindings (new / bound / disabled) for preview + confirm UI.
func HandleChannelModelsPreview(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		ch, insts, err := resolveChannel(a, r, id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Channel not found"})
			return
		}
		inst := pickActiveInstance(insts)
		if inst == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No active instance"})
			return
		}
		provider := channelProvider(a, r, ch)
		models, err := discoverModels(provider, inst.BaseURL, instanceAPIKey(inst))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		upstream := make([]string, 0, len(models))
		for _, m := range models {
			upstream = append(upstream, m.ID)
		}
		bindings, err := loadBindings(a, r, id)
		if err != nil {
			log.Printf("console: preview bindings: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load bindings"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": classifyPreview(upstream, bindings)})
	}
}

// HandleChannelModelsSync applies selected upstream model bindings, optionally
// auto-creating missing platform models (default pricing 0).
func HandleChannelModelsSync(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		ch, insts, err := resolveChannel(a, r, id)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Channel not found"})
			return
		}
		inst := pickActiveInstance(insts)
		if inst == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "No active instance"})
			return
		}
		var req struct {
			ModelIDs   []string `json:"model_ids"`
			AutoCreate bool     `json:"auto_create"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		provider := channelProvider(a, r, ch)
		models, err := discoverModels(provider, inst.BaseURL, instanceAPIKey(inst))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		// Filter to selected upstream ids (all if none provided).
		sel := map[string]bool{}
		if len(req.ModelIDs) == 0 {
			for _, m := range models {
				sel[m.ID] = true
			}
		} else {
			for _, m := range req.ModelIDs {
				sel[m] = true
			}
		}
		tx, err := a.Pool.Begin(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to start tx"})
			return
		}
		defer tx.Rollback(r.Context())
		applied, created, skipped := 0, 0, 0
		for _, m := range models {
			if !sel[m.ID] {
				continue
			}
			var modelID uuid.UUID
			err := tx.QueryRow(r.Context(), `SELECT id FROM models WHERE code = $1`, m.ID).Scan(&modelID)
			if err != nil {
				modelID = uuid.New()
				if !req.AutoCreate {
					skipped++
					continue
				}
				if _, err := tx.Exec(r.Context(),
					`INSERT INTO models (id, code, provider, category, display_name, status, release_stage)
					 VALUES ($1, $2, $3, 'chat', $2, 'active', 'beta')`,
					modelID, m.ID, provider); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create model"})
					return
				}
				if _, err := tx.Exec(r.Context(),
					`INSERT INTO model_pricing (model_id, request_type, pricing_dimension, unit_name, unit_price, is_active)
					 VALUES ($1, 'chat', 'input', 'token', 0, true)`, modelID); err != nil {
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to seed pricing"})
					return
				}
				created++
			}
			if _, err := tx.Exec(r.Context(),
				`INSERT INTO channel_models (channel_id, model_id, upstream_model, enabled)
				 VALUES ($1, $2, $3, true)
				 ON CONFLICT (channel_id, model_id) DO UPDATE SET enabled = true`,
				id, modelID, m.ID); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to bind model"})
				return
			}
			applied++
		}
		if err := tx.Commit(r.Context()); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to commit"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"applied": applied, "created": created, "skipped": skipped})
	}
}

// HandleChannelModelsList returns a channel's model bindings.
func HandleChannelModelsList(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		bindings, err := loadBindings(a, r, id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load bindings"})
			return
		}
		items := make([]previewItem, 0, len(bindings))
		for _, b := range bindings {
			status := "bound"
			if !b.Enabled {
				status = "disabled"
			}
			items = append(items, previewItem{Upstream: b.Upstream, Code: b.Code, ModelID: b.ModelID.String(), Status: status, Enabled: b.Enabled})
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": items})
	}
}

// HandleSetChannelModelEnabled toggles a channel↔model binding's enabled flag.
func HandleSetChannelModelEnabled(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		channelID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid channel id"})
			return
		}
		modelID, err := uuid.Parse(chi.URLParam(r, "modelId"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model id"})
			return
		}
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		tag, err := a.Pool.Exec(r.Context(),
			`UPDATE channel_models SET enabled = $1 WHERE channel_id = $2 AND model_id = $3`,
			req.Enabled, channelID, modelID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update binding"})
			return
		}
		if tag.RowsAffected() == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Binding not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
