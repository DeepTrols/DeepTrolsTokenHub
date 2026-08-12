package console

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token  string       `json:"token"`
	Expiry string       `json:"expiry"`
	UserID string       `json:"user_id"`
	Email  string       `json:"email"`
	Name   string       `json:"name"`
	User   *userProfile `json:"user,omitempty"`
}

type userProfile struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type meResponse struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Status       string `json:"status"`
	UserType     string `json:"user_type"`
	Phone        string `json:"phone,omitempty"`
	AvatarURL    string `json:"avatar_url,omitempty"`
	TenantID     string `json:"tenant_id,omitempty"`
	TenantName   string `json:"tenant_name,omitempty"`
	TenantRole   string `json:"tenant_role,omitempty"`
	TenantStatus string `json:"tenant_status,omitempty"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// enterpriseRegisterRequest is the self-service registration payload for
// enterprise accounts. The tenant is created pending_review and approved by a
// platform admin before it can consume quota or billing.
type enterpriseRegisterRequest struct {
	CompanyName string `json:"company_name"`
	ContactName string `json:"contact_name"`
	Email       string `json:"email"`
	Password    string `json:"password"`
}

type registerResponse struct {
	Token string      `json:"token"`
	User  userProfile `json:"user"`
}

// HandleLogin authenticates a user and returns a JWT token.
func HandleLogin(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req loginRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		ctx := r.Context()
		ipAddress := extractIP(r.RemoteAddr)
		userAgent := r.UserAgent()

		// Try database lookup first.
		dbUser, err := a.Users.FindByEmail(ctx, req.Email)
		if err == nil {
			// User found in DB. Verify password.
			if dbUser.Status != domain.UserStatusActive {
				recordLoginHistory(ctx, a, dbUser.ID, ipAddress, userAgent, false)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Account is not active"})
				return
			}
			// Verify password with bcrypt.
			passwordOK := bcrypt.CompareHashAndPassword([]byte(dbUser.PasswordHash), []byte(req.Password)) == nil
			if !passwordOK {
				recordLoginHistory(ctx, a, dbUser.ID, ipAddress, userAgent, false)
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
				return
			}

			// Generate JWT.
			token, expiry, err := generateLoginJWT(a, dbUser)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
				return
			}

			recordLoginHistory(ctx, a, dbUser.ID, ipAddress, userAgent, true)

			setAuthCookie(w, token, a.Config)
			writeJSON(w, http.StatusOK, loginResponse{
				Token: token, Expiry: expiry,
				UserID: dbUser.ID.String(), Email: dbUser.Email, Name: dbUser.DisplayName,
				User: &userProfile{ID: dbUser.ID.String(), Email: dbUser.Email, Name: dbUser.DisplayName},
			})
			return
		}

		// Not found or other DB error: try bootstrap admin fallback.
		if !errors.Is(err, user.ErrNotFound) {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}

		// Bootstrap admin fallback.
		if req.Email == a.Config.Bootstrap.AdminEmail && subtle.ConstantTimeCompare([]byte(req.Password), []byte(a.Config.Bootstrap.AdminPassword)) == 1 {
			bootstrapID := ensureBootstrapAdmin(r.Context(), a)
			token, expiry, err := generateBootstrapJWT(a)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
				return
			}
			recordLoginHistory(ctx, a, bootstrapID, ipAddress, userAgent, true)
			setAuthCookie(w, token, a.Config)
			writeJSON(w, http.StatusOK, loginResponse{
				Token: token, Expiry: expiry,
				UserID: bootstrapID.String(), Email: a.Config.Bootstrap.AdminEmail, Name: "Administrator",
				User: &userProfile{ID: bootstrapID.String(), Email: a.Config.Bootstrap.AdminEmail, Name: "Administrator"},
			})
			return
		}

		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid email or password"})
	}
}

// HandleMe returns the authenticated user's profile.
func HandleMe(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}
		dbUser, err := a.Users.FindByID(r.Context(), userID)
		if err != nil {
			if errors.Is(err, user.ErrNotFound) {
				writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "User not found"})
			} else {
				log.Printf("HandleMe: FindByID error: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			}
			return
		}
		tenantID := ""
		tenantName := ""
		tenantRole := ""
		tenantStatus := ""
		if m, err := a.Memberships.FindByUserID(r.Context(), dbUser.ID); err == nil && m != nil && m.Status == domain.MembershipStatusActive {
			tenantID = m.TenantID.String()
			tenantRole = string(m.Role)
			if t, err := a.Tenants.FindByID(r.Context(), m.TenantID); err == nil && t != nil {
				tenantName = t.Name
				tenantStatus = string(t.Status)
			}
		}
		writeJSON(w, http.StatusOK, meResponse{
			ID: dbUser.ID.String(), Email: dbUser.Email, Name: dbUser.DisplayName,
			Role: dbUser.Role, Status: string(dbUser.Status),
			UserType:     string(dbUser.UserType),
			Phone:        dbUser.Phone,
			AvatarURL:    dbUser.AvatarURL,
			TenantID:     tenantID,
			TenantName:   tenantName,
			TenantRole:   tenantRole,
			TenantStatus: tenantStatus,
		})
	}
}

// HandleRegister creates a new user account with wallet and returns a JWT token.
func HandleRegister(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req registerRequest
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
		if strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Name is required"})
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
		userType := domain.UserTypePersonal
		u := &domain.User{
			ID: uuid.New(), Email: req.Email, PasswordHash: string(hash),
			DisplayName: req.Name, Role: "user", Status: domain.UserStatusActive,
			UserType: userType, CreatedAt: now, UpdatedAt: now,
		}

		if err := a.Users.Create(ctx, u); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
			return
		}

		// Create wallet. Bonus balance is granted only when the demo money
		// faucet is enabled (ENABLE_FAKE_PAYMENT=true); production = 0.
		bonus := "0"
		if a.Config.FakePayment {
			bonus = "1000"
		}
		a.Pool.Exec(ctx,
			`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
			 VALUES ($1, $2, $3, '0', 'CNY', 0, $4, $4)`,
			uuid.New(), u.ID, bonus, now,
		)

		token, _, err := generateLoginJWT(a, u)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
			return
		}
		setAuthCookie(w, token, a.Config)
		writeJSON(w, http.StatusCreated, registerResponse{
			Token: token,
			User:  userProfile{ID: u.ID.String(), Email: u.Email, Name: u.DisplayName},
		})
	}
}

// HandleRegisterEnterprise self-service registration for enterprise accounts.
// The user, a pending_review tenant, an owner membership, and a zero-balance
// wallet are created in one transaction so a failure never leaves a partial
// account behind. Enterprise accounts receive no signup bonus: the wallet is
// funded by the platform after the tenant is approved.
func HandleRegisterEnterprise(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Cap the request body so a multi-megabyte payload cannot exhaust memory
		// on this unauthenticated endpoint (also protected by per-IP rate limit).
		r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
		var req enterpriseRegisterRequest
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
		if strings.TrimSpace(req.CompanyName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Company name is required"})
			return
		}
		if len(req.CompanyName) > 255 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Company name is too long"})
			return
		}
		if strings.TrimSpace(req.ContactName) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Contact name is required"})
			return
		}
		if len(req.ContactName) > 255 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Contact name is too long"})
			return
		}

		ctx := r.Context()
		if _, err := a.Users.FindByEmail(ctx, req.Email); err == nil {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "Email already registered"})
			return
		} else if !errors.Is(err, user.ErrNotFound) {
			log.Printf("HandleRegisterEnterprise: FindByEmail: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Internal server error"})
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), 12)
		if err != nil {
			log.Printf("HandleRegisterEnterprise: hash password: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to hash password"})
			return
		}

		// Derive the tenant code before opening the transaction so the uniqueness
		// check does not hold a write lock.
		code, err := deriveTenantCode(ctx, a.Tenants, req.CompanyName)
		if err != nil {
			log.Printf("HandleRegisterEnterprise: derive tenant code: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		now := time.Now().UTC()
		userID := uuid.New()
		tenantID := uuid.New()

		tx, err := a.Pool.Begin(ctx)
		if err != nil {
			log.Printf("HandleRegisterEnterprise: begin tx: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
			return
		}
		defer tx.Rollback(ctx)

		// 1. User account (enterprise type, active).
		if _, err := tx.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, display_name, role, status, user_type, created_at, updated_at)
			 VALUES ($1, $2, $3, $4, 'user', 'active', 'enterprise', $5, $5)`,
			userID, req.Email, string(hash), req.ContactName, now,
		); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				// A concurrent registration claimed the email between the lookup
				// above and the insert.
				writeJSON(w, http.StatusConflict, map[string]string{"error": "Email already registered"})
				return
			}
			log.Printf("HandleRegisterEnterprise: create user: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
			return
		}

		// 2. Tenant awaiting platform review.
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
				'', $6, '', '',
				$7, $7
			)`,
			tenantID, code, req.CompanyName, domain.TenantStatusPendingReview, userID, req.Email, now,
		); err != nil {
			log.Printf("HandleRegisterEnterprise: create tenant: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		// 3. Owner membership so the account can reach its own tenant.
		if _, err := tx.Exec(ctx,
			`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status, joined_at, created_at, updated_at)
			 VALUES ($1, $2, $3, 'owner', 'active', $4, $4, $4)`,
			uuid.New(), tenantID, userID, now,
		); err != nil {
			log.Printf("HandleRegisterEnterprise: create membership: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create tenant"})
			return
		}

		// 4. Zero-balance wallet. Enterprise accounts never receive the personal
		// signup bonus; the balance is funded after approval.
		if _, err := tx.Exec(ctx,
			`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
			 VALUES ($1, $2, '0', '0', 'CNY', 0, $3, $3)`,
			uuid.New(), userID, now,
		); err != nil {
			log.Printf("HandleRegisterEnterprise: create wallet: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
			return
		}

		if err := tx.Commit(ctx); err != nil {
			log.Printf("HandleRegisterEnterprise: commit: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create account"})
			return
		}

		u := &domain.User{
			ID: userID, Email: req.Email, DisplayName: req.ContactName,
			Role: "user", Status: domain.UserStatusActive,
			UserType: domain.UserTypeEnterprise, CreatedAt: now, UpdatedAt: now,
		}
		token, _, err := generateLoginJWT(a, u)
		if err != nil {
			log.Printf("HandleRegisterEnterprise: generate token: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
			return
		}
		setAuthCookie(w, token, a.Config)
		writeJSON(w, http.StatusCreated, registerResponse{
			Token: token,
			User:  userProfile{ID: u.ID.String(), Email: u.Email, Name: u.DisplayName},
		})
	}
}

// deriveTenantCode converts a company name into a unique tenant code. The code
// is slugified (lowercased, non-ASCII dropped, other characters collapsed to
// dashes) and, if the base code is already taken, suffixed with -1, -2, ...
// until an available code is found.
func deriveTenantCode(ctx context.Context, repos tenant.Repository, companyName string) (string, error) {
	base := slugify(companyName)
	if base == "" {
		base = "tenant"
	}
	code := base
	for i := 1; i <= 100; i++ {
		existing, err := repos.FindByCode(ctx, code)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("look up tenant code %q: %w", code, err)
		}
		if existing == nil {
			return code, nil
		}
		code = fmt.Sprintf("%s-%d", base, i)
	}
	return "", errors.New("could not derive a unique tenant code")
}

// slugify lowercases s and collapses every run outside [a-z0-9] into a single
// dash. Leading/trailing dashes are trimmed. Returns "" when no ASCII
// alphanumeric rune survives (e.g. a CJK-only company name).
func slugify(s string) string {
	var b strings.Builder
	dashPending := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dashPending = false
		} else if b.Len() > 0 && !dashPending {
			b.WriteByte('-')
			dashPending = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// HandleLogout clears the auth cookie and returns a success message.
func HandleLogout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearAuthCookie(w, a.Config)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
	}
}

// Helper functions

// setAuthCookie writes an httpOnly, Secure, SameSite cookie containing
// the JWT token. Must be called before writing the response body.
func setAuthCookie(w http.ResponseWriter, token string, cfg *config.Config) {
	cookie := &http.Cookie{
		Name:     cfg.Cookie.Name,
		Value:    token,
		Path:     "/",
		MaxAge:   cfg.Cookie.MaxAgeSeconds,
		HttpOnly: true,
		Secure:   cfg.Cookie.Secure,
		SameSite: parseSameSite(cfg.Cookie.SameSite),
	}
	http.SetCookie(w, cookie)
}

// clearAuthCookie removes the auth cookie by setting MaxAge=-1.
func clearAuthCookie(w http.ResponseWriter, cfg *config.Config) {
	cookie := &http.Cookie{
		Name:     cfg.Cookie.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cfg.Cookie.Secure,
		SameSite: parseSameSite(cfg.Cookie.SameSite),
	}
	http.SetCookie(w, cookie)
}

func parseSameSite(val string) http.SameSite {
	switch val {
	case "Lax":
		return http.SameSiteLaxMode
	case "None":
		return http.SameSiteNoneMode
	case "Strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteDefaultMode
	}
}

func generateLoginJWT(a *app.App, u *domain.User) (string, string, error) {
	userType := string(u.UserType)
	tenantID := ""
	tenantRole := ""
	if m, err := a.Memberships.FindByUserID(context.Background(), u.ID); err == nil && m != nil && m.Status == domain.MembershipStatusActive {
		tenantID = m.TenantID.String()
		tenantRole = string(m.Role)
	}
	tok, err := jwtutil.GenerateToken(u.ID, u.Email, u.DisplayName, u.Role, userType, tenantID, tenantRole, a.Config.JWT.Secret, a.Config.JWT.ExpiryHours)
	return tok, fmt.Sprintf("%d", a.Config.JWT.ExpiryHours), err
}

func ensureBootstrapAdmin(ctx context.Context, a *app.App) uuid.UUID {
	adminID := uuid.Nil
	hash, err := bcrypt.GenerateFromPassword([]byte(a.Config.Bootstrap.AdminPassword), 12)
	if err != nil {
		log.Printf("ensureBootstrapAdmin: failed to hash password: %v", err)
		return adminID
	}
	a.Pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, display_name, role, status, user_type, created_at, updated_at)
		 VALUES ($1, $2, $3, 'Administrator', 'admin', 'active', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET email=$2, password_hash=$3, role='admin', status='active'`,
		adminID, a.Config.Bootstrap.AdminEmail, string(hash),
	)
	return adminID
}

func generateBootstrapJWT(a *app.App) (token, expiry string, err error) {
	expiryTime := fmt.Sprintf("%d", a.Config.JWT.ExpiryHours)
	tok, err := jwtutil.GenerateToken(uuid.Nil, a.Config.Bootstrap.AdminEmail, "Administrator", "admin", string(domain.UserTypePersonal), "", "", a.Config.JWT.Secret, a.Config.JWT.ExpiryHours)
	if err != nil {
		return "", "", fmt.Errorf("generateBootstrapJWT: %w", err)
	}
	return tok, expiryTime, nil
}

func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func recordLoginHistory(ctx context.Context, a *app.App, userID uuid.UUID, ipAddress, userAgent string, success bool) {
	_, err := a.Pool.Exec(ctx,
		`INSERT INTO login_history (id, user_id, ip_address, user_agent, success, created_at)
		 VALUES (uuid_generate_v4(), $1, $2, $3, $4, NOW())`,
		userID, ipAddress, userAgent, success,
	)
	if err != nil {
		log.Printf("console: failed to record login_history: %v", err)
	}
}
