package console

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/guardrails"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type detectionItemPayload struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	DetectorType  string         `json:"detector_type"`
	Action        string         `json:"action"`
	ConfigVersion int            `json:"config_version"`
	Config        map[string]any `json:"config"`
}

type bindingPayload struct {
	ID            string `json:"id"`
	ScopeType     string `json:"scope_type"`
	ScopeID       string `json:"scope_id"`
	Checkpoint    string `json:"checkpoint"`
	Protocol      string `json:"protocol"`
	ConfigVersion int    `json:"config_version"`
}

type guardrailPolicyPayload struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	Status         string                 `json:"status"`
	ConfigVersion  int                    `json:"config_version"`
	DetectionItems []detectionItemPayload `json:"detection_items"`
	Bindings       []bindingPayload       `json:"bindings"`
	CreatedAt      string                 `json:"created_at"`
	UpdatedAt      string                 `json:"updated_at"`
}

func HandleListGuardrailPolicies(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		policies, err := a.GuardrailsPolicies.LoadPolicies(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list guardrail policies"})
			return
		}
		out := make([]guardrailPolicyPayload, 0, len(policies))
		for _, p := range policies {
			out = append(out, guardrailPolicyFromDomain(p))
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": out, "total": len(out)})
	}
}

func HandleSaveGuardrailPolicy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		var payload guardrailPolicyPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		policy := guardrails.Policy{
			ID:            payload.ID,
			Name:          payload.Name,
			Description:   payload.Description,
			Status:        payload.Status,
			ConfigVersion: payload.ConfigVersion,
		}
		if policy.ConfigVersion == 0 {
			policy.ConfigVersion = guardrails.CurrentConfigVersion
		}
		if policy.ID == "" {
			policy.ID = uuid.New().String()
		}
		for _, item := range payload.DetectionItems {
			di := guardrails.DetectionItem{
				ID: item.ID, PolicyID: policy.ID, Name: item.Name,
				DetectorType: item.DetectorType, Action: item.Action,
				ConfigVersion: item.ConfigVersion, Config: item.Config,
			}
			if di.ID == "" {
				di.ID = uuid.New().String()
			}
			if di.ConfigVersion == 0 {
				di.ConfigVersion = guardrails.CurrentConfigVersion
			}
			policy.DetectionItems = append(policy.DetectionItems, di)
		}
		for _, b := range payload.Bindings {
			binding := guardrails.Binding{
				ID: b.ID, PolicyID: policy.ID, ScopeType: b.ScopeType,
				ScopeID: b.ScopeID, Checkpoint: b.Checkpoint, Protocol: b.Protocol,
				ConfigVersion: b.ConfigVersion,
			}
			if binding.ID == "" {
				binding.ID = uuid.New().String()
			}
			if binding.ConfigVersion == 0 {
				binding.ConfigVersion = guardrails.CurrentConfigVersion
			}
			policy.Bindings = append(policy.Bindings, binding)
		}
		normalized, err := guardrails.NormalizePolicy(policy)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if err := a.GuardrailsPolicies.SavePolicy(r.Context(), normalized); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to save guardrail policy"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"id": normalized.ID})
	}
}

func HandleDeleteGuardrailPolicy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		if err := a.GuardrailsPolicies.DeletePolicy(r.Context(), chi.URLParam(r, "id")); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
	}
}

func guardrailPolicyFromDomain(p guardrails.Policy) guardrailPolicyPayload {
	out := guardrailPolicyPayload{
		ID: p.ID, Name: p.Name, Description: p.Description,
		Status: p.Status, ConfigVersion: p.ConfigVersion,
		CreatedAt: p.CreatedAt.Format(time.RFC3339), UpdatedAt: p.UpdatedAt.Format(time.RFC3339),
		DetectionItems: []detectionItemPayload{}, Bindings: []bindingPayload{},
	}
	for _, item := range p.DetectionItems {
		out.DetectionItems = append(out.DetectionItems, detectionItemPayload{
			ID: item.ID, Name: item.Name, DetectorType: item.DetectorType,
			Action: item.Action, ConfigVersion: item.ConfigVersion, Config: item.Config,
		})
	}
	for _, b := range p.Bindings {
		out.Bindings = append(out.Bindings, bindingPayload{
			ID: b.ID, ScopeType: b.ScopeType, ScopeID: b.ScopeID,
			Checkpoint: b.Checkpoint, Protocol: b.Protocol, ConfigVersion: b.ConfigVersion,
		})
	}
	return out
}
