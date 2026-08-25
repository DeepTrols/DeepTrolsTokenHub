package console

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

// teamMember represents a member in the team list response. Balance is the
// member's personal-wallet spendable balance, serialized as a decimal string so
// the frontend never touches floats for money.
type teamMember struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Email   string `json:"email"`
	Role    string `json:"role"`
	Status  string `json:"status"`
	Balance string `json:"balance"`
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
			// The member's spendable balance comes from their personal wallet
			// (tenant_id IS NULL). A missing wallet simply leaves balance empty.
			if w, err := a.Wallets.FindByUser(r.Context(), m.UserID, nil); err == nil && w != nil {
				item.Balance = w.Balance.String()
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

type createSubAccountRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

// HandleCreateSubAccount lets an enterprise admin provision a sub-account bound
// to their tenant. The sub-account is an enterprise user with an active
// membership and an empty wallet; balance is handed out afterwards via
// HandleAllocateBalance. The three inserts run in one transaction so a failed
// step can never leave a half-created account behind.
func HandleCreateSubAccount(a *app.App) http.HandlerFunc {
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

		var req createSubAccountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}
		if _, err := mail.ParseAddress(req.Email); err != nil || req.Email == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid email"})
			return
		}
		if len(req.Password) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Password must be at least 8 characters"})
			return
		}
		if strings.TrimSpace(req.DisplayName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Display name is required"})
			return
		}
		if req.Role != string(domain.MembershipRoleAdmin) && req.Role != string(domain.MembershipRoleMember) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Role must be 'admin' or 'member'"})
			return
		}

		ctx := r.Context()
		if _, err := a.Users.FindByEmail(ctx, req.Email); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Email already registered"})
			return
		} else if !errors.Is(err, user.ErrNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
			return
		}

		now := time.Now().UTC()
		subID := uuid.New()
		tx, err := a.Pool.Begin(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create sub-account"})
			return
		}
		defer tx.Rollback(ctx)

		// 1. User row. A concurrent registration may have claimed the email, so a
		// unique-violation is reported as a conflict, not a server error.
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, role, status, user_type, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'user', 'active', 'enterprise', $5, $5)`,
			subID, req.Email, string(hash), req.DisplayName, now,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Email already registered"})
				return
			}
			log.Printf("HandleCreateSubAccount: create user: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create sub-account"})
			return
		}

		// 2. Membership row.
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'active', $5, $5, $5)`,
			uuid.New(), tenantID, subID, req.Role, now,
		); err != nil {
			log.Printf("HandleCreateSubAccount: create membership: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create sub-account"})
			return
		}

		// 3. Wallet row (zero balance; balance is allocated by the team admin).
		if _, err := tx.Exec(ctx,
			`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
			 VALUES ($1, $2, '0', '0', 'CNY', 0, $3, $3)`,
			uuid.New(), subID, now,
		); err != nil {
			log.Printf("HandleCreateSubAccount: create wallet: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create sub-account"})
			return
		}

		if err := tx.Commit(ctx); err != nil {
			log.Printf("HandleCreateSubAccount: commit: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create sub-account"})
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{
			"id":           subID.String(),
			"email":        req.Email,
			"display_name": req.DisplayName,
			"role":         req.Role,
		})
	}
}
