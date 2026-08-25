package console

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
)

type simulateRoutingRequest struct {
	Model    string  `json:"model"`
	TenantID *string `json:"tenant_id"`
}

type simulatedRoute struct {
	ChannelID     string `json:"channel_id"`
	ChannelName   string `json:"channel_name"`
	HealthScore   int    `json:"health_score"`
	HealthStatus  string `json:"health_status"`
	Strategy      string `json:"strategy"`
	StickySession bool   `json:"sticky_session"`
	InstanceID    string `json:"instance_id"`
	BaseURL       string `json:"base_url"`
	UpstreamModel string `json:"upstream_model"`
	CurrentLoad   int    `json:"current_load"`
}

// HandleSimulateRouting previews the router's ordered candidates for a model
// (+ optional tenant), powering the admin routing simulator.
func HandleSimulateRouting(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var req simulateRoutingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Model == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model is required"})
			return
		}
		var tenantID *uuid.UUID
		if req.TenantID != nil && *req.TenantID != "" {
			id, err := uuid.Parse(*req.TenantID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant_id"})
				return
			}
			tenantID = &id
		}

		identity := &domain.RequestIdentity{
			UserID:    uuid.New(),
			TenantID:  tenantID,
			RequestID: "simulate",
		}
		candidates, err := a.Router.RouteCandidates(r.Context(), identity, req.Model, 5)
		if err != nil {
			switch {
			case errors.Is(err, gw.ErrModelNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Model not found in catalog"})
			case errors.Is(err, gw.ErrTenantNotAllowed):
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "Model not available to this tenant"})
			case errors.Is(err, gw.ErrNoChannelAvailable):
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "No routable channel available"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		out := make([]simulatedRoute, 0, len(candidates))
		for _, c := range candidates {
			load := 0
			if c.Instance != nil {
				load = c.Instance.CurrentLoad
			}
			out = append(out, simulatedRoute{
				ChannelID:     c.Channel.ID.String(),
				ChannelName:   c.Channel.Name,
				HealthScore:   c.Channel.HealthScore,
				HealthStatus:  string(c.Channel.HealthStatus),
				Strategy:      string(c.Channel.Strategy),
				StickySession: c.Channel.StickySession,
				InstanceID:    c.Instance.ID.String(),
				BaseURL:       c.Instance.BaseURL,
				UpstreamModel: c.UpstreamModel,
				CurrentLoad:   load,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}
