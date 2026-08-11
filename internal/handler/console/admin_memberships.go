package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// adminMemberItem represents a member of a tenant in the admin member list response.
type adminMemberItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Status   string `json:"status"`
	JoinedAt string `json:"joined_at"`
}

// loadTenantForAdmin resolves the tenant from the URL param and verifies it exists.
// On failure it writes the error response and returns ok=false.
func loadTenantForAdmin(w http.ResponseWriter, a *app.App, r *http.Request) (*domain.Tenant, bool) {
	tenantID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid tenant ID"})
		return nil, false
	}
	t, err := a.Tenants.FindByID(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
		return nil, false
	}
	return t, true
}

// HandleAdminListTenantMembers lists all members of a tenant, including
// non-active statuses so an admin can manage suspended/left members. Admin only.
func HandleAdminListTenantMembers(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		tenant, ok := loadTenantForAdmin(w, a, r)
		if !ok {
			return
		}

		members, err := a.Memberships.FindByTenantID(r.Context(), tenant.ID)
		if err != nil {
			log.Printf("HandleAdminListTenantMembers: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list members"})
			return
		}

		items := make([]adminMemberItem, 0, len(members))
		for _, m := range members {
			item := adminMemberItem{
				ID:       m.UserID.String(),
				Role:     string(m.Role),
				Status:   string(m.Status),
				JoinedAt: m.JoinedAt.Format(time.RFC3339),
			}
			if u, err := a.Users.FindByID(r.Context(), m.UserID); err == nil && u != nil {
				item.Name = u.DisplayName
				item.Email = u.Email
			}
			items = append(items, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"data": items, "total": len(items)})
	}
}

type addTenantMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// HandleAdminAddTenantMember directly adds an existing user as a tenant member.
// The owner role is never assignable here. Admin only.
func HandleAdminAddTenantMember(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		tenant, ok := loadTenantForAdmin(w, a, r)
		if !ok {
			return
		}

		var req addTenantMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Email == "" || !strings.Contains(req.Email, "@") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Valid email is required"})
			return
		}
		if req.Role != string(domain.MembershipRoleAdmin) && req.Role != string(domain.MembershipRoleMember) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Role must be 'admin' or 'member'"})
			return
		}

		u, err := a.Users.FindByEmail(r.Context(), strings.TrimSpace(req.Email))
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "User not found"})
				return
			}
			log.Printf("HandleAdminAddTenantMember: FindByEmail: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to look up user"})
			return
		}

		if _, err := a.Memberships.FindByUserAndTenant(r.Context(), u.ID, tenant.ID); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "User is already a member of this tenant"})
			return
		}

		now := time.Now().UTC()
		m := &domain.TenantMembership{
			ID:       uuid.New(),
			TenantID: tenant.ID,
			UserID:   u.ID,
			Role:     domain.MembershipRole(req.Role),
			Status:   domain.MembershipStatusActive,
			JoinedAt: now,
		}
		if err := a.Memberships.Create(r.Context(), m); err != nil {
			if errors.Is(err, membership.ErrAlreadyExists) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "User is already a member of this tenant"})
				return
			}
			log.Printf("HandleAdminAddTenantMember: Create: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to add member"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]string{"id": m.ID.String()})
	}
}

// HandleAdminRemoveTenantMember removes a member from a tenant. The tenant owner
// cannot be removed. Admin only.
func HandleAdminRemoveTenantMember(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		tenant, ok := loadTenantForAdmin(w, a, r)
		if !ok {
			return
		}
		targetID, err := uuid.Parse(chi.URLParam(r, "userId"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		m, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenant.ID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found"})
			return
		}
		if m.Role == domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot remove the tenant owner"})
			return
		}

		if err := a.Memberships.Delete(r.Context(), m.ID); err != nil {
			log.Printf("HandleAdminRemoveTenantMember: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove member"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	}
}

type adminChangeRoleRequest struct {
	Role string `json:"role"`
}

// HandleAdminChangeTenantMemberRole updates a member's role within a tenant.
// The owner role is never assignable here. Admin only.
func HandleAdminChangeTenantMemberRole(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if rejectNonAdmin(w, r) {
			return
		}
		tenant, ok := loadTenantForAdmin(w, a, r)
		if !ok {
			return
		}
		targetID, err := uuid.Parse(chi.URLParam(r, "userId"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		var req adminChangeRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Role != string(domain.MembershipRoleAdmin) && req.Role != string(domain.MembershipRoleMember) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Role must be 'admin' or 'member'"})
			return
		}

		m, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenant.ID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found"})
			return
		}
		if m.Role == domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot change the owner's role"})
			return
		}

		if err := a.Memberships.UpdateRole(r.Context(), m.ID, domain.MembershipRole(req.Role)); err != nil {
			log.Printf("HandleAdminChangeTenantMemberRole: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to change role"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}
