package console

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
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

// createTenantRequest is the request body for HandleCreateTenant. An owner can
// be provisioned with the tenant so a new enterprise is not a dead tenant:
// either owner_id (an existing user) or owner_email + owner_password (an
// existing account that must match, or a new enterprise account to create).
type createTenantRequest struct {
	Code          string `json:"code"`
	Name          string `json:"name"`
	OwnerID       string `json:"owner_id,omitempty"`
	OwnerEmail    string `json:"owner_email,omitempty"`
	OwnerPassword string `json:"owner_password,omitempty"`
}

// Owner-provisioning sentinel errors. The handler maps each to a specific HTTP
// status so callers get actionable feedback instead of an opaque 500.
var (
	errOwnerPasswordMismatch = errors.New("owner password does not match the existing account")
	errOwnerPasswordTooShort = errors.New("owner password must be at least 8 characters")
	errOwnerEmailTaken       = errors.New("owner email is already registered")
)

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
//
// An owner can be provisioned in the same transaction so the new enterprise is
// not a dead tenant:
//   - owner_id: an existing user becomes the tenant's owner (must exist).
//   - owner_email + owner_password: if an account with the email already
//     exists, the supplied password must match it and that account becomes the
//     owner; otherwise a new enterprise user (plus a zero-balance wallet) is
//     created and made the owner. Either path is atomic with the tenant insert,
//     so a failed create never leaves an orphan owner account behind.
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
		if req.OwnerID != "" && req.OwnerEmail != "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provide either owner_id or owner_email, not both"})
			return
		}
		if req.OwnerEmail != "" && req.OwnerPassword == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "owner_password is required when owner_email is provided"})
			return
		}
		// Owner email must be a well-formed address, matching the validation in
		// auth registration and sub-account creation. Otherwise a typo like
		// "ceo@acme" would provision an account that can never log in.
		if req.OwnerEmail != "" {
			if _, err := mail.ParseAddress(req.OwnerEmail); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid owner email format"})
				return
			}
		}

		ctx := r.Context()

		// Resolve an explicit owner_id up front: it must be a well-formed UUID
		// naming a real user, otherwise the membership FK would fail at commit
		// time with an opaque 500 instead of an actionable 400.
		var ownerID *uuid.UUID
		if req.OwnerID != "" {
			parsed, err := uuid.Parse(req.OwnerID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid owner_id"})
				return
			}
			if _, err := a.Users.FindByID(ctx, parsed); err != nil {
				if errors.Is(err, user.ErrNotFound) {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Owner user not found"})
					return
				}
				log.Printf("HandleCreateTenant: find owner: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
				return
			}
			ownerID = &parsed
		}

		// Check code uniqueness.
		existing, err := a.Tenants.FindByCode(ctx, req.Code)
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
		tenantID := uuid.New()

		tx, err := a.Pool.Begin(ctx)
		if err != nil {
			log.Printf("HandleCreateTenant: begin tx: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}
		defer tx.Rollback(ctx)

		// Provision the owner by email. The email lookup runs outside the
		// transaction (it reads via the pool); a concurrent insert is caught by
		// the unique-constraint handling inside resolveTenantOwnerUser. The
		// user/wallet/membership rows are created inside the transaction so a
		// failed tenant insert rolls them all back.
		if req.OwnerEmail != "" {
			ownerID, err = resolveTenantOwnerUser(ctx, tx, a, req.OwnerEmail, req.OwnerPassword)
			if err != nil {
				switch {
				case errors.Is(err, errOwnerPasswordMismatch):
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Owner password is incorrect"})
				case errors.Is(err, errOwnerPasswordTooShort):
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Owner password must be at least 8 characters"})
				case errors.Is(err, errOwnerEmailTaken):
					writeJSON(w, http.StatusConflict, map[string]string{"error": "Owner email is already registered"})
				default:
					log.Printf("HandleCreateTenant: resolve owner: %v", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to provision owner"})
				}
				return
			}
		}

		// 1. Tenant row.
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenants (
				id, code, name, status, owner_id,
				brand_config, runtime_config, settlement_config,
				status_reason,
				credit_code, contact_email, contact_phone, business_license,
				created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				'{}'::jsonb, '{}'::jsonb, '{}'::jsonb,
				'',
				'', '', '', '',
				$6, $6
			)`,
			tenantID, req.Code, req.Name, domain.TenantStatusPendingReview, ownerID, now,
		); err != nil {
			log.Printf("HandleCreateTenant: create tenant: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		// 2. Owner membership, so the new enterprise is reachable by someone.
		if ownerID != nil {
			if _, err := tx.Exec(ctx,
				`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
				 VALUES ($1, $2, $3, 'owner', 'active', $4, $4, $4)`,
				uuid.New(), tenantID, *ownerID, now,
			); err != nil {
				log.Printf("HandleCreateTenant: create owner membership: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create owner membership"})
				return
			}
		}

		if err := tx.Commit(ctx); err != nil {
			log.Printf("HandleCreateTenant: commit: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		resp := tenantListResponse{
			ID:        tenantID.String(),
			Code:      req.Code,
			Name:      req.Name,
			Status:    string(domain.TenantStatusPendingReview),
			CreatedAt: now.Format(time.RFC3339),
		}
		if ownerID != nil {
			ownerStr := ownerID.String()
			resp.OwnerID = &ownerStr
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{
			"data": resp,
		})
	}
}

// resolveTenantOwnerUser ensures an owner account exists for the given email and
// returns its ID. If an account already exists, the supplied password must match
// so a platform admin cannot silently claim an arbitrary account. Otherwise a
// new enterprise user and a zero-balance wallet are created inside the caller's
// transaction, keeping user and tenant creation atomic.
func resolveTenantOwnerUser(ctx context.Context, tx pgx.Tx, a *app.App, email, password string) (*uuid.UUID, error) {
	existing, err := a.Users.FindByEmail(ctx, email)
	if err == nil {
		if bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(password)) != nil {
			return nil, errOwnerPasswordMismatch
		}
		id := existing.ID
		return &id, nil
	}
	if !errors.Is(err, user.ErrNotFound) {
		return nil, fmt.Errorf("find owner by email: %w", err)
	}

	if len(password) < 8 {
		return nil, errOwnerPasswordTooShort
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return nil, fmt.Errorf("hash owner password: %w", err)
	}

	newID := uuid.New()
	now := time.Now().UTC()
	if _, err := tx.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role, status, user_type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'user', 'active', 'enterprise', $5, $5)`,
		newID, email, string(hash), emailLocalPart(email), now,
	); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, errOwnerEmailTaken
		}
		return nil, fmt.Errorf("create owner user: %w", err)
	}

	// The owner is a real platform user: give them a zero-balance wallet just
	// like any other account.
	if _, err := tx.Exec(ctx,
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '0', '0', 'CNY', 0, $3, $3)`,
		uuid.New(), newID, now,
	); err != nil {
		return nil, fmt.Errorf("create owner wallet: %w", err)
	}

	return &newID, nil
}

// emailLocalPart returns the portion of an email before the '@', used as the
// default display name when provisioning an owner account.
func emailLocalPart(email string) string {
	if i := strings.Index(email, "@"); i > 0 {
		return email[:i]
	}
	return email
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

// HandleDeleteTenant permanently deletes a tenant and all tenant-owned rows
// (quota pools/allocations/ledger, models, memberships, invitations).
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

		// The platform tenant is bootstrap-owned; it must not be deletable.
		if rejectPlatformTenantMutation(w, tn) {
			return
		}

		// The admin audit middleware records only the tenant UUID; the operator
		// log must carry the human-readable identity since the row is about to
		// be permanently removed.
		log.Printf("HandleDeleteTenant: deleting tenant code=%s name=%s id=%s", tn.Code, tn.Name, tn.ID)

		if err := a.Tenants.Delete(r.Context(), tn.ID); err != nil {
			if errors.Is(err, tenant.ErrNotFound) {
				// Row vanished between FindByID and Delete; treat as already gone.
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "Tenant not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete tenant"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{
			"status": "deleted",
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
