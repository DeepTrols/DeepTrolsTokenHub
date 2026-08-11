package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// tenantListResponse is the JSON shape for a tenant in list responses.
type tenantListResponse struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Status       string  `json:"status"`
	OwnerID      *string `json:"owner_id,omitempty"`
	StatusReason string  `json:"status_reason,omitempty"`
	CreatedAt    string  `json:"created_at"`
}

// tenantDetailResponse is the JSON shape for a single tenant.
type tenantDetailResponse struct {
	ID               string         `json:"id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	OwnerID          *string        `json:"owner_id,omitempty"`
	BrandConfig      map[string]any `json:"brand_config,omitempty"`
	RuntimeConfig    map[string]any `json:"runtime_config,omitempty"`
	SettlementConfig map[string]any `json:"settlement_config,omitempty"`
	StatusReason     string         `json:"status_reason,omitempty"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at,omitempty"`
}

// createTenantRequest is the request body for HandleCreateTenant.
type createTenantRequest struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	OwnerID string `json:"owner_id,omitempty"`
}

// updateTenantRequest is the request body for HandleUpdateTenant.
type updateTenantRequest struct {
	Name         *string        `json:"name,omitempty"`
	Status       *string        `json:"status,omitempty"`
	StatusReason *string        `json:"status_reason,omitempty"`
	BrandConfig  map[string]any `json:"brand_config,omitempty"`
}

// HandleListTenants returns all tenants ordered by creation date descending.
func HandleListTenants(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenants, err := a.Tenants.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list tenants"})
			return
		}

		response := make([]tenantListResponse, 0, len(tenants))
		for _, t := range tenants {
			item := tenantListResponse{
				ID:           t.ID.String(),
				Code:         t.Code,
				Name:         t.Name,
				Status:       string(t.Status),
				StatusReason: t.StatusReason,
				CreatedAt:    t.CreatedAt.Format(time.RFC3339),
			}
			if t.OwnerID != nil {
				ownerStr := t.OwnerID.String()
				item.OwnerID = &ownerStr
			}
			response = append(response, item)
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data":  response,
			"total": len(response),
		})
	}
}

// HandleGetTenant returns a single tenant with all its domains.
func HandleGetTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		detail := tenantDetailResponse{
			ID:               tn.ID.String(),
			Code:             tn.Code,
			Name:             tn.Name,
			Status:           string(tn.Status),
			BrandConfig:      tn.BrandConfig,
			RuntimeConfig:    tn.RuntimeConfig,
			SettlementConfig: tn.SettlementConfig,
			StatusReason:     tn.StatusReason,
			CreatedAt:        tn.CreatedAt.Format(time.RFC3339),
			UpdatedAt:        tn.UpdatedAt.Format(time.RFC3339),
		}
		if tn.OwnerID != nil {
			ownerStr := tn.OwnerID.String()
			detail.OwnerID = &ownerStr
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": detail,
		})
	}
}

// HandleCreateTenant creates a new tenant with status "pending_review".
func HandleCreateTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		var req createTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if req.Code == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "code is required"})
			return
		}
		if req.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
			return
		}

		// Check code uniqueness
		existing, err := a.Tenants.FindByCode(r.Context(), req.Code)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("HandleCreateTenant: FindByCode error: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}
		if existing != nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Tenant code already exists"})
			return
		}

		now := time.Now().UTC()
		tn := &domain.Tenant{
			ID:        uuid.New(),
			Code:      req.Code,
			Name:      req.Name,
			Status:    domain.TenantStatusPendingReview,
			CreatedAt: now,
			UpdatedAt: now,
		}

		if req.OwnerID != "" {
			ownerID, err := uuid.Parse(req.OwnerID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid owner_id"})
				return
			}
			tn.OwnerID = &ownerID
		}

		if err := a.Tenants.Create(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": tenantListResponse{
				ID:        tn.ID.String(),
				Code:      tn.Code,
				Name:      tn.Name,
				Status:    string(tn.Status),
				CreatedAt: tn.CreatedAt.Format(time.RFC3339),
			},
		})
	}
}

// HandleUpdateTenant updates a tenant's fields including status with transition validation.
func HandleUpdateTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		var req updateTenantRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		// Apply field updates
		if req.Name != nil {
			tn.Name = *req.Name
		}
		if req.StatusReason != nil {
			tn.StatusReason = *req.StatusReason
		}
		if req.BrandConfig != nil {
			tn.BrandConfig = req.BrandConfig
		}

		// Status change with transition validation
		if req.Status != nil {
			newStatus := domain.TenantStatus(*req.Status)

			// The platform tenant's lifecycle is managed by the bootstrap; it
			// must not be suspended or terminated from the admin console.
			if isPlatformTenant(tn) && newStatus != tn.Status {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "The platform tenant's status cannot be changed"})
				return
			}

			// Validate it's a known status
			if !isValidTenantStatus(newStatus) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status value"})
				return
			}

			// If status actually changes, validate the transition
			if newStatus != tn.Status {
				if !isValidTenantTransition(tn, newStatus) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid status transition"})
					return
				}
				tn.Status = newStatus
			}
		}

		tn.UpdatedAt = time.Now().UTC()

		if err := a.Tenants.Update(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update tenant"})
			return
		}

		resp := tenantDetailResponse{
			ID:           tn.ID.String(),
			Code:         tn.Code,
			Name:         tn.Name,
			Status:       string(tn.Status),
			StatusReason: tn.StatusReason,
			UpdatedAt:    tn.UpdatedAt.Format(time.RFC3339),
		}
		if tn.OwnerID != nil {
			ownerStr := tn.OwnerID.String()
			resp.OwnerID = &ownerStr
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"data": resp,
		})
	}
}

// HandleDeleteTenant terminates a tenant by setting status to "terminated".
func HandleDeleteTenant(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}

		tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
			return
		}

		tn, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
			return
		}

		// The platform tenant is bootstrap-owned; it must not be terminable.
		if rejectPlatformTenantMutation(w, tn) {
			return
		}

		tn.Status = domain.TenantStatusTerminated
		tn.StatusReason = "admin action"
		tn.UpdatedAt = time.Now().UTC()

		if err := a.Tenants.Update(r.Context(), tn); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to terminate tenant"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "terminated",
			"id":     tn.ID.String(),
		})
	}
}

// isValidTenantStatus returns true if the status is one of the defined TenantStatus values.
func isValidTenantStatus(s domain.TenantStatus) bool {
	switch s {
	case domain.TenantStatusPendingReview,
		domain.TenantStatusActive,
		domain.TenantStatusSuspended,
		domain.TenantStatusTerminated,
		domain.TenantStatusRejected:
		return true
	default:
		return false
	}
}

// isValidTenantTransition checks if the transition from the tenant's current status to the new status is allowed.
func isValidTenantTransition(tn *domain.Tenant, newStatus domain.TenantStatus) bool {
	for _, allowed := range tn.ValidTransitions() {
		if allowed == newStatus {
			return true
		}
	}
	return false
}
