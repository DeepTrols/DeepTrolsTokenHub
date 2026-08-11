package console

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// teamMember represents a member in the team list response.
type teamMember struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// isTenantAdmin checks that the authenticated user is an admin or owner of their
// enterprise tenant. Returns the tenant ID or an error.
func isTenantAdmin(r *http.Request, a *app.App) (uuid.UUID, error) {
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
	// A suspended or left admin must not retain administrative access; only an
	// active membership passes the guard.
	if m.Status != domain.MembershipStatusActive {
		return uuid.Nil, membership.ErrNotFound
	}
	if !m.IsAdminOrOwner() {
		return uuid.Nil, membership.ErrNotFound
	}
	return tenantID, nil
}

// HandleListTeamMembers returns all active members of the user's enterprise tenant.
func HandleListTeamMembers(a *app.App) http.HandlerFunc {
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

		members, err := a.Memberships.FindByTenantID(r.Context(), tenantID)
		if err != nil {
			log.Printf("HandleListTeamMembers: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to list members"})
			return
		}

		// Members who left are omitted; active and suspended members are shown
		// with their status so admins can manage them.
		items := make([]teamMember, 0, len(members))
		for _, m := range members {
			if m.Status == domain.MembershipStatusLeft {
				continue
			}
			item := teamMember{ID: m.UserID.String(), Role: string(m.Role), Status: string(m.Status)}
			if u, err := a.Users.FindByID(r.Context(), m.UserID); err == nil && u != nil {
				item.Name = u.DisplayName
				item.Email = u.Email
			}
			items = append(items, item)
		}

		writeJSON(w, http.StatusOK, map[string]any{"members": items})
	}
}

// HandleRemoveMember removes a member from the enterprise tenant.
func HandleRemoveMember(a *app.App) http.HandlerFunc {
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

		targetUserID := chi.URLParam(r, "userId")
		if targetUserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "User ID is required"})
			return
		}
		targetID, err := uuid.Parse(targetUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		// Prevent self-removal.
		currentUserID, _ := jwtutil.UserIDFromContext(r.Context())
		if targetID == currentUserID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot remove yourself"})
			return
		}

		m, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found"})
			return
		}

		// Only owners can remove admins; cannot remove the owner.
		if m.Role == domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot remove the tenant owner"})
			return
		}
		currentMembership, _ := a.Memberships.FindByUserAndTenant(r.Context(), currentUserID, tenantID)
		if m.Role == domain.MembershipRoleAdmin && currentMembership != nil && currentMembership.Role != domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only the owner can remove admins"})
			return
		}

		if err := a.Memberships.Delete(r.Context(), m.ID); err != nil {
			log.Printf("HandleRemoveMember: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to remove member"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
	}
}

type changeRoleRequest struct {
	Role string `json:"role"`
}

// HandleChangeMemberRole updates a member's role within the enterprise tenant.
func HandleChangeMemberRole(a *app.App) http.HandlerFunc {
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

		targetUserID := chi.URLParam(r, "userId")
		if targetUserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "User ID is required"})
			return
		}
		targetID, err := uuid.Parse(targetUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		var req changeRoleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Role != string(domain.MembershipRoleAdmin) && req.Role != string(domain.MembershipRoleMember) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Role must be 'admin' or 'member'"})
			return
		}

		// Only owners can change roles.
		currentUserID, _ := jwtutil.UserIDFromContext(r.Context())
		currentMembership, err := a.Memberships.FindByUserAndTenant(r.Context(), currentUserID, tenantID)
		if err != nil || currentMembership.Role != domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only the tenant owner can change roles"})
			return
		}

		m, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found"})
			return
		}
		// Only active memberships can receive a role change. Suspended and left
		// members are not part of the operating team.
		if m.Status != domain.MembershipStatusActive {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot change the role of a non-active member"})
			return
		}
		if m.Role == domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot change the owner's role"})
			return
		}

		if err := a.Memberships.UpdateRole(r.Context(), m.ID, domain.MembershipRole(req.Role)); err != nil {
			log.Printf("HandleChangeMemberRole: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to change role"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}

type suspendMemberRequest struct {
	Status string `json:"status"`
}

// HandleSuspendMember toggles a member between active and suspended. Admins can
// suspend members; only owners can suspend other admins. The owner can never be
// suspended, and a user cannot suspend themselves.
func HandleSuspendMember(a *app.App) http.HandlerFunc {
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

		targetUserID := chi.URLParam(r, "userId")
		if targetUserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "User ID is required"})
			return
		}
		targetID, err := uuid.Parse(targetUserID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid user ID"})
			return
		}

		var req suspendMemberRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if req.Status != string(domain.MembershipStatusActive) && req.Status != string(domain.MembershipStatusSuspended) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Status must be 'active' or 'suspended'"})
			return
		}

		// Prevent self-suspension.
		currentUserID, _ := jwtutil.UserIDFromContext(r.Context())
		if targetID == currentUserID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cannot change your own status"})
			return
		}

		m, err := a.Memberships.FindByUserAndTenant(r.Context(), targetID, tenantID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Member not found"})
			return
		}
		if m.Role == domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Cannot suspend the tenant owner"})
			return
		}
		currentMembership, _ := a.Memberships.FindByUserAndTenant(r.Context(), currentUserID, tenantID)
		if m.Role == domain.MembershipRoleAdmin && currentMembership != nil && currentMembership.Role != domain.MembershipRoleOwner {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Only the owner can suspend admins"})
			return
		}

		if err := a.Memberships.UpdateStatus(r.Context(), m.ID, domain.MembershipStatus(req.Status)); err != nil {
			log.Printf("HandleSuspendMember: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to update member status"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
	}
}
