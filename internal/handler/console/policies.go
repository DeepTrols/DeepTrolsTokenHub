package console

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// policyResponse is the JSON shape returned for a single route policy.
type policyResponse struct {
	ID                    string            `json:"id"`
	Name                  string            `json:"name"`
	TenantID              *string           `json:"tenant_id"`
	UserLevel             string            `json:"user_level"`
	ModelID               *string           `json:"model_id"`
	Priority              int               `json:"priority"`
	CandidateChannelIDs   []string          `json:"candidate_channel_ids"`
	CandidateChannelNames map[string]string `json:"candidate_channel_names"`
	FallbackPolicy        string            `json:"fallback_policy"`
	IsActive              bool              `json:"is_active"`
	CreatedAt             string            `json:"created_at"`
	UpdatedAt             string            `json:"updated_at"`
}

// createPolicyRequest is the request body for HandleCreatePolicy.
type createPolicyRequest struct {
	Name                string   `json:"name"`
	TenantID            *string  `json:"tenant_id"`
	UserLevel           string   `json:"user_level"`
	ModelID             *string  `json:"model_id"`
	Priority            int      `json:"priority"`
	CandidateChannelIDs []string `json:"candidate_channel_ids"`
	FallbackPolicy      string   `json:"fallback_policy"`
}

// updatePolicyRequest is the request body for HandleUpdatePolicy.
// All fields are optional — only non-nil fields are applied.
// For tenant_id and model_id, an explicit null in JSON sets the field to nil (clears it).
type updatePolicyRequest struct {
	Name                *string   `json:"name"`
	TenantID            *string   `json:"tenant_id"` // nil = not present, set = explicit value
	UserLevel           *string   `json:"user_level"`
	ModelID             *string   `json:"model_id"` // nil = not present, set = explicit value
	Priority            *int      `json:"priority"`
	CandidateChannelIDs *[]string `json:"candidate_channel_ids"`
	FallbackPolicy      *string   `json:"fallback_policy"`
}

// validFallbackPolicies is the set of allowed fallback_policy values.
var validFallbackPolicies = map[string]bool{
	"disabled":       true,
	"tenant_default": true,
	"shared_allowed": true,
	"next_policy":    true,
}

// isValidFallbackPolicy returns true if the value is an allowed fallback_policy.
func isValidFallbackPolicy(policy string) bool {
	return validFallbackPolicies[policy]
}

// resolveChannelNames fetches channel names for a list of channel UUIDs.
func resolveChannelNames(a *app.App, r *http.Request, channelIDs []string) map[string]string {
	names := make(map[string]string)
	if len(channelIDs) == 0 {
		return names
	}

	// Build the UUID array for the query.
	uuids := make([]uuid.UUID, 0, len(channelIDs))
	for _, idStr := range channelIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			continue
		}
		uuids = append(uuids, id)
	}
	if len(uuids) == 0 {
		return names
	}

	rows, err := a.Pool.Query(r.Context(),
		`SELECT id, name FROM channels WHERE id = ANY($1)`, uuids,
	)
	if err != nil {
		log.Printf("resolveChannelNames: query error: %v", err)
		return names
	}
	defer rows.Close()

	for rows.Next() {
		var id uuid.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			log.Printf("resolveChannelNames: scan error: %v", err)
			continue
		}
		names[id.String()] = name
	}
	return names
}

// buildPolicyResponse converts a DB row into a policyResponse including channel names.
func buildPolicyResponse(a *app.App, r *http.Request,
	id uuid.UUID, name string, tenantID *uuid.UUID, userLevel string,
	modelID *uuid.UUID, priority int, channelIDs []uuid.UUID,
	fallbackPolicy string, isActive bool, createdAt, updatedAt time.Time) policyResponse {

	var tidStr *string
	if tenantID != nil {
		s := tenantID.String()
		tidStr = &s
	}

	var midStr *string
	if modelID != nil {
		s := modelID.String()
		midStr = &s
	}

	chStrs := make([]string, len(channelIDs))
	chUUIDs := make([]string, len(channelIDs))
	for i, id := range channelIDs {
		s := id.String()
		chStrs[i] = s
		chUUIDs[i] = s
	}

	chNames := resolveChannelNames(a, r, chUUIDs)

	return policyResponse{
		ID:                    id.String(),
		Name:                  name,
		TenantID:              tidStr,
		UserLevel:             userLevel,
		ModelID:               midStr,
		Priority:              priority,
		CandidateChannelIDs:   chStrs,
		CandidateChannelNames: chNames,
		FallbackPolicy:        fallbackPolicy,
		IsActive:              isActive,
		CreatedAt:             createdAt.Format(time.RFC3339),
		UpdatedAt:             updatedAt.Format(time.RFC3339),
	}
}

// HandleListPolicies returns all route policies with resolved channel names.
func HandleListPolicies(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		rows, err := a.Pool.Query(r.Context(),
			`SELECT rp.id, rp.name, rp.tenant_id, rp.user_level, rp.model_id,
			        rp.priority, rp.candidate_channel_ids, rp.fallback_policy,
			        rp.is_active, rp.created_at, rp.updated_at
			 FROM route_policies rp
			 ORDER BY rp.priority DESC, rp.created_at DESC`,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to query policies"})
			return
		}
		defer rows.Close()

		response := make([]policyResponse, 0)
		for rows.Next() {
			var (
				id                              uuid.UUID
				name, userLevel, fallbackPolicy string
				tenantID, modelID               *uuid.UUID
				priority                        int
				channelIDs                      []uuid.UUID
				isActive                        bool
				createdAt, updatedAt            time.Time
			)
			if err := rows.Scan(&id, &name, &tenantID, &userLevel, &modelID,
				&priority, &channelIDs, &fallbackPolicy, &isActive, &createdAt, &updatedAt); err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to read policy"})
				return
			}

			response = append(response, buildPolicyResponse(a, r,
				id, name, tenantID, userLevel, modelID, priority, channelIDs,
				fallbackPolicy, isActive, createdAt, updatedAt))
		}

		if err := rows.Err(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to iterate policies"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleCreatePolicy creates a new route policy.
func HandleCreatePolicy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req createPolicyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		// Validate required fields.
		if req.Name == "" || req.UserLevel == "" || req.FallbackPolicy == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name, user_level, and fallback_policy are required"})
			return
		}

		// Validate fallback_policy.
		if !isValidFallbackPolicy(req.FallbackPolicy) {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "Invalid fallback_policy. Must be one of: disabled, tenant_default, shared_allowed, next_policy",
			})
			return
		}

		// Parse optional tenant_id.
		var tenantID *uuid.UUID
		if req.TenantID != nil {
			tid, err := uuid.Parse(*req.TenantID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant_id"})
				return
			}
			tenantID = &tid
		}

		// Parse optional model_id.
		var modelID *uuid.UUID
		if req.ModelID != nil {
			mid, err := uuid.Parse(*req.ModelID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model_id"})
				return
			}
			modelID = &mid
		}

		// Parse candidate channel IDs.
		channelUUIDs := make([]uuid.UUID, 0, len(req.CandidateChannelIDs))
		for _, idStr := range req.CandidateChannelIDs {
			chID, err := uuid.Parse(idStr)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid candidate_channel_id: " + idStr})
				return
			}
			channelUUIDs = append(channelUUIDs, chID)
		}

		policyID := uuid.New()
		now := time.Now().UTC()

		_, err := a.Pool.Exec(r.Context(),
			`INSERT INTO route_policies (id, name, tenant_id, user_level, model_id, priority, candidate_channel_ids, fallback_policy, is_active, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, true, $9, $9)`,
			policyID, req.Name, tenantID, req.UserLevel, modelID, req.Priority,
			channelUUIDs, req.FallbackPolicy, now,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create policy"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": buildPolicyResponse(a, r,
				policyID, req.Name, tenantID, req.UserLevel, modelID, req.Priority,
				channelUUIDs, req.FallbackPolicy, true, now, now),
		})
	}
}

// HandleUpdatePolicy updates an existing route policy.
func HandleUpdatePolicy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		policyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
			return
		}

		// Decode raw JSON body to detect which fields are present and handle
		// nullable UUID fields (tenant_id, model_id) where we need to distinguish
		// "absent" (do not update) from "explicit null" (set to nil).
		var rawBody map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		// Check which nullable UUID fields are present and whether they are explicitly null.
		_, hasTenantID := rawBody["tenant_id"]
		isTenantNull := hasTenantID && string(rawBody["tenant_id"]) == "null"

		_, hasModelID := rawBody["model_id"]
		isModelNull := hasModelID && string(rawBody["model_id"]) == "null"

		// Decode into typed struct via re-marshaling.
		typedBytes, err := json.Marshal(rawBody)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		var req updatePolicyRequest
		if err := json.Unmarshal(typedBytes, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		dbCtx := r.Context()

		// Check policy exists and read current values.
		var (
			currentName, currentUserLevel, currentFallback string
			currentTenantID, currentModelID                *uuid.UUID
			currentPriority                                int
			currentChannelIDs                              []uuid.UUID
			currentIsActive                                bool
			currentCreatedAt                               time.Time
		)
		err = a.Pool.QueryRow(dbCtx,
			`SELECT name, tenant_id, user_level, model_id, priority, candidate_channel_ids, fallback_policy, is_active, created_at
			 FROM route_policies WHERE id = $1`, policyID,
		).Scan(&currentName, &currentTenantID, &currentUserLevel, &currentModelID,
			&currentPriority, &currentChannelIDs, &currentFallback, &currentIsActive, &currentCreatedAt)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Policy not found"})
			return
		}

		// Merge: apply only non-nil fields.
		name := currentName
		if req.Name != nil {
			name = *req.Name
		}

		tenantID := currentTenantID
		if isTenantNull {
			tenantID = nil
		} else if req.TenantID != nil {
			tid, err := uuid.Parse(*req.TenantID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant_id"})
				return
			}
			tenantID = &tid
		}

		userLevel := currentUserLevel
		if req.UserLevel != nil {
			userLevel = *req.UserLevel
		}

		modelID := currentModelID
		if isModelNull {
			modelID = nil
		} else if req.ModelID != nil {
			mid, err := uuid.Parse(*req.ModelID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid model_id"})
				return
			}
			modelID = &mid
		}

		priority := currentPriority
		if req.Priority != nil {
			priority = *req.Priority
		}

		channelIDs := currentChannelIDs
		if req.CandidateChannelIDs != nil {
			newIDs := make([]uuid.UUID, 0, len(*req.CandidateChannelIDs))
			for _, idStr := range *req.CandidateChannelIDs {
				chID, err := uuid.Parse(idStr)
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid candidate_channel_id: " + idStr})
					return
				}
				newIDs = append(newIDs, chID)
			}
			channelIDs = newIDs
		}

		fallback := currentFallback
		if req.FallbackPolicy != nil {
			if !isValidFallbackPolicy(*req.FallbackPolicy) {
				writeJSON(w, http.StatusBadRequest, map[string]string{
					"error": "Invalid fallback_policy. Must be one of: disabled, tenant_default, shared_allowed, next_policy",
				})
				return
			}
			fallback = *req.FallbackPolicy
		}

		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`UPDATE route_policies
			 SET name = $1, tenant_id = $2, user_level = $3, model_id = $4,
			     priority = $5, candidate_channel_ids = $6, fallback_policy = $7, updated_at = $8
			 WHERE id = $9`,
			name, tenantID, userLevel, modelID, priority, channelIDs, fallback, now, policyID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update policy"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": buildPolicyResponse(a, r,
				policyID, name, tenantID, userLevel, modelID, priority,
				channelIDs, fallback, currentIsActive, currentCreatedAt, now),
		})
	}
}

// HandleDeletePolicy soft-deletes a route policy by setting is_active to false.
func HandleDeletePolicy(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		policyID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid policy ID"})
			return
		}

		dbCtx := r.Context()

		// Check policy exists.
		var isActive bool
		err = a.Pool.QueryRow(dbCtx,
			`SELECT is_active FROM route_policies WHERE id = $1`, policyID,
		).Scan(&isActive)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Policy not found"})
			return
		}

		now := time.Now().UTC()

		_, err = a.Pool.Exec(dbCtx,
			`UPDATE route_policies SET is_active = false, updated_at = $1 WHERE id = $2`,
			now, policyID,
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to deactivate policy"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "deleted",
			"id":     policyID.String(),
		})
	}
}
