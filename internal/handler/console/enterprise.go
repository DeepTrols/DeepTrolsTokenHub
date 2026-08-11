package console

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/invitation"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// enterpriseResponse is the shape returned by GET /api/console/enterprise.
// Sensitive settlement data (credit_code, business_license, settlement_config)
// is omitted for regular members; only tenant admins and owners receive it.
type enterpriseResponse struct {
	ID               string         `json:"id"`
	Code             string         `json:"code"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	CreditCode       string         `json:"credit_code,omitempty"`
	ContactEmail     string         `json:"contact_email"`
	ContactPhone     string         `json:"contact_phone"`
	BusinessLicense  string         `json:"business_license,omitempty"`
	BrandConfig      map[string]any `json:"brand_config"`
	RuntimeConfig    map[string]any `json:"runtime_config"`
	SettlementConfig map[string]any `json:"settlement_config,omitempty"`
	MemberCount      int            `json:"member_count"`
}

// enterpriseInvitationItem represents an invitation in the list response.
type enterpriseInvitationItem struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at"`
}

// isTenantMember resolves the tenant ID from context and verifies the user has
// an active membership in it. Returns the tenant ID or an error.
func isTenantMember(r *http.Request, a *app.App) (uuid.UUID, error) {
	userID, err := jwtutil.UserIDFromContext(r.Context())
	if err != nil {
		return uuid.Nil, err
	}
	tenantID, err := jwtutil.TenantIDFromContext(r.Context())
	if err != nil || tenantID == uuid.Nil {
		return uuid.Nil, membership.ErrNotFound
	}
	m, err := a.Memberships.FindByUserAndTenant(r.Context(), userID, tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	if m.Status != domain.MembershipStatusActive {
		return uuid.Nil, membership.ErrNotFound
	}
	return tenantID, nil
}

// isTenantOwner verifies the authenticated user is the owner of their tenant.
// Returns the tenant ID or an error.
func isTenantOwner(r *http.Request, a *app.App) (uuid.UUID, error) {
	userID, err := jwtutil.UserIDFromContext(r.Context())
	if err != nil {
		return uuid.Nil, err
	}
	tenantID, err := jwtutil.TenantIDFromContext(r.Context())
	if err != nil || tenantID == uuid.Nil {
		return uuid.Nil, membership.ErrNotFound
	}
	m, err := a.Memberships.FindByUserAndTenant(r.Context(), userID, tenantID)
	if err != nil {
		return uuid.Nil, err
	}
	// A suspended owner must not retain ownership privileges.
	if m.Status != domain.MembershipStatusActive {
		return uuid.Nil, membership.ErrNotFound
	}
	if m.Role != domain.MembershipRoleOwner {
		return uuid.Nil, membership.ErrNotFound
	}
	return tenantID, nil
}

// HandleGetEnterprise returns the current tenant's settings for any active member.
func HandleGetEnterprise(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantMember(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Enterprise access required"})
			return
		}

		t, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleGetEnterprise: FindByID: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load enterprise"})
			return
		}

		resp := enterpriseResponse{
			ID:            t.ID.String(),
			Code:          t.Code,
			Name:          t.Name,
			Status:        string(t.Status),
			ContactEmail:  t.ContactEmail,
			ContactPhone:  t.ContactPhone,
			BrandConfig:   t.BrandConfig,
			RuntimeConfig: t.RuntimeConfig,
		}

		// Settlement data is sensitive and only returned to admins/owners.
		if _, err := isTenantAdmin(r, a); err == nil {
			resp.CreditCode = t.CreditCode
			resp.BusinessLicense = t.BusinessLicense
			resp.SettlementConfig = t.SettlementConfig
		}

		members, err := a.Memberships.FindByTenantID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleGetEnterprise: member count: %v", err)
		} else {
			for _, m := range members {
				if m.Status == domain.MembershipStatusActive {
					resp.MemberCount++
				}
			}
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

type updateEnterpriseRequest struct {
	Name         *string `json:"name,omitempty"`
	ContactEmail *string `json:"contact_email,omitempty"`
	ContactPhone *string `json:"contact_phone,omitempty"`
}

// HandleUpdateEnterprise updates the enterprise name and contact info. Admin+.
func HandleUpdateEnterprise(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		var req updateEnterpriseRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		t, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleUpdateEnterprise: FindByID: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load enterprise"})
			return
		}

		if req.Name != nil {
			name := strings.TrimSpace(*req.Name)
			if name == "" {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Enterprise name cannot be empty"})
				return
			}
			t.Name = name
		}
		if req.ContactEmail != nil {
			email := strings.TrimSpace(*req.ContactEmail)
			if email != "" && !strings.Contains(email, "@") {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid contact email is required"})
				return
			}
			t.ContactEmail = email
		}
		if req.ContactPhone != nil {
			t.ContactPhone = strings.TrimSpace(*req.ContactPhone)
		}

		t.UpdatedAt = time.Now().UTC()
		if err := a.Tenants.Update(r.Context(), t); err != nil {
			log.Printf("HandleUpdateEnterprise: Update: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update enterprise"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

type updateBrandRequest struct {
	BrandConfig map[string]any `json:"brand_config"`
}

// HandleUpdateEnterpriseBrand updates the tenant brand config JSON. Owner only.
func HandleUpdateEnterpriseBrand(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantOwner(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Owner access required"})
			return
		}

		// Cap the request body so brand_config cannot be used to stage an
		// arbitrarily large payload in the database.
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)

		var req updateBrandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.BrandConfig == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "brand_config is required"})
			return
		}
		if len(req.BrandConfig) > 50 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "brand_config exceeds the maximum of 50 keys"})
			return
		}

		t, err := a.Tenants.FindByID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleUpdateEnterpriseBrand: FindByID: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load enterprise"})
			return
		}

		t.BrandConfig = req.BrandConfig
		t.UpdatedAt = time.Now().UTC()
		if err := a.Tenants.Update(r.Context(), t); err != nil {
			log.Printf("HandleUpdateEnterpriseBrand: Update: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update brand config"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

// HandleListPendingInvitations returns pending invitations for the tenant. Admin+.
func HandleListPendingInvitations(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		invs, err := a.Invitations.ListByTenantID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleListPendingInvitations: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list invitations"})
			return
		}

		items := make([]enterpriseInvitationItem, 0, len(invs))
		for _, inv := range invs {
			if inv.Status != domain.InvitationStatusPending {
				continue
			}
			items = append(items, enterpriseInvitationItem{
				ID:        inv.ID.String(),
				Email:     inv.Email,
				Role:      string(inv.Role),
				Status:    string(inv.Status),
				CreatedAt: inv.CreatedAt.Format(time.RFC3339),
				ExpiresAt: inv.ExpiresAt.Format(time.RFC3339),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{"invitations": items, "total": len(items)})
	}
}

// HandleCancelInvitation cancels a pending invitation. Admin+.
func HandleCancelInvitation(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := jwtutil.UserIDFromContext(r.Context()); err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantAdmin(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Tenant admin access required"})
			return
		}

		invID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid invitation ID"})
			return
		}

		// The invitation must belong to this tenant.
		invs, err := a.Invitations.ListByTenantID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleCancelInvitation: ListByTenantID: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load invitations"})
			return
		}
		var found bool
		for _, inv := range invs {
			if inv.ID == invID {
				found = true
				if inv.Status != domain.InvitationStatusPending {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Only pending invitations can be cancelled"})
					return
				}
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invitation not found"})
			return
		}

		if err := a.Invitations.UpdateStatus(r.Context(), invID, domain.InvitationStatusCancelled); err != nil {
			if err == invitation.ErrNotFound {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Invitation not found"})
				return
			}
			log.Printf("HandleCancelInvitation: UpdateStatus: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to cancel invitation"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
	}
}

type transferOwnershipRequest struct {
	TargetUserID string `json:"target_user_id"`
}

// HandleTransferOwnership atomically transfers ownership to another admin of the
// same tenant. The current owner becomes an admin. Owner only.
func HandleTransferOwnership(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentUserID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		tenantID, err := isTenantOwner(r, a)
		if err != nil {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Owner access required"})
			return
		}

		// The platform tenant's ownership is bound to the system administrator
		// by the bootstrap; transferring it would be fought on the next boot.
		pt, findErr := a.Tenants.FindByID(r.Context(), tenantID)
		if findErr == nil && isPlatformTenant(pt) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "The platform tenant's ownership cannot be transferred"})
			return
		}

		var req transferOwnershipRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		targetID, err := uuid.Parse(req.TargetUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid target user ID"})
			return
		}
		if targetID == currentUserID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot transfer ownership to yourself"})
			return
		}

		target, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Target member not found"})
			return
		}
		if target.Role != domain.MembershipRoleAdmin {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Ownership can only be transferred to an admin"})
			return
		}
		if target.Status != domain.MembershipStatusActive {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Target member must be active"})
			return
		}

		tx, err := a.Pool.Begin(r.Context())
		if err != nil {
			log.Printf("HandleTransferOwnership: Begin: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to transfer ownership"})
			return
		}
		defer tx.Rollback(r.Context())

		// Lock the current owner's membership row so two concurrent transfers
		// cannot both pass the owner check and promote two different owners.
		var lockedRole string
		err = tx.QueryRow(r.Context(),
			`SELECT role FROM tenant_memberships WHERE user_id = $1 AND tenant_id = $2 FOR UPDATE`,
			currentUserID, tenantID,
		).Scan(&lockedRole)
		if err != nil || domain.MembershipRole(lockedRole) != domain.MembershipRoleOwner {
			log.Printf("HandleTransferOwnership: owner lock: err=%v role=%q", err, lockedRole)
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Ownership has already been transferred"})
			return
		}

		// Current owner -> admin; target admin -> owner; tenant owner_id -> target.
		// Each statement must affect exactly one row; 0 rows means state changed
		// underneath us, so abort the transfer rather than half-apply it.
		cmd, err := tx.Exec(r.Context(),
			`UPDATE tenant_memberships SET role = 'admin', updated_at = NOW() WHERE user_id = $1 AND tenant_id = $2`,
			currentUserID, tenantID)
		if err != nil || cmd.RowsAffected() != 1 {
			log.Printf("HandleTransferOwnership: demote owner: err=%v affected=%d", err, cmd.RowsAffected())
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to transfer ownership"})
			return
		}
		cmd, err = tx.Exec(r.Context(),
			`UPDATE tenant_memberships SET role = 'owner', updated_at = NOW() WHERE user_id = $1 AND tenant_id = $2`,
			targetID, tenantID)
		if err != nil || cmd.RowsAffected() != 1 {
			log.Printf("HandleTransferOwnership: promote target: err=%v affected=%d", err, cmd.RowsAffected())
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to transfer ownership"})
			return
		}
		cmd, err = tx.Exec(r.Context(),
			`UPDATE tenants SET owner_id = $1, updated_at = NOW() WHERE id = $2`,
			targetID, tenantID)
		if err != nil || cmd.RowsAffected() != 1 {
			log.Printf("HandleTransferOwnership: update tenant owner: err=%v affected=%d", err, cmd.RowsAffected())
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to transfer ownership"})
			return
		}

		if err := tx.Commit(r.Context()); err != nil {
			log.Printf("HandleTransferOwnership: Commit: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to transfer ownership"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "transferred"})
	}
}
