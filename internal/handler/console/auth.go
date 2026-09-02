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
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
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
	Email      string `json:"email"`
	Password   string `json:"password"`
	Name       string `json:"name"`
	InviteCode string `json:"invite_code"`
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

			completeConsoleLogin(a, w, r, dbUser.ID, token)
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
			completeConsoleLogin(a, w, r, bootstrapID, token)
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
		if !settingBool(a, r, "register_enabled") {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "Registration is disabled"})
			return
		}
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
			UserType: userType, InviteCode: generateInviteCode(), CreatedAt: now, UpdatedAt: now,
		}

		// Resolve an optional inviter before creating the account (no
		// self-invite is possible: this user does not exist yet).
		var invitedBy *uuid.UUID
		inviteReward := decimal.Zero
		if code := strings.TrimSpace(req.InviteCode); code != "" {
			var inviterID uuid.UUID
			err = a.Pool.QueryRow(ctx,
				`SELECT id FROM users WHERE invite_code = $1 AND status = 'active'`,
				code).Scan(&inviterID)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "无效的邀请码"})
				return
			}
			invitedBy = &inviterID
			inviteReward = inviteRewardSetting(a, r)
		}
		u.InvitedBy = invitedBy

		if err := a.Users.Create(ctx, u); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create user"})
			return
		}
		// Persist the invite fields (repo Create covers the base columns).
		_, _ = a.Pool.Exec(ctx,
			`UPDATE users SET invite_code = $2, invited_by = $3 WHERE id = $1`,
			u.ID, u.InviteCode, invitedBy)

		// Create the wallet through the repository. The signup bonus (demo
		// faucet ENABLE_FAKE_PAYMENT=true only; production = 0) is granted via
		// an idempotent, ledgered TopUp — never a bare balance write — so the
		// wallet balance always reconciles with wallet_transactions.
		bonus := decimal.Zero
		if a.Config.FakePayment {
			bonus = SignupBonusUser
		}
		newWallet, err := ProvisionUserWallet(ctx, a.Wallets, u.ID, bonus)
		if err != nil {
			log.Printf("HandleRegister: provision wallet: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to create wallet"})
			return
		}

		// Invite rewards: both sides credited through idempotent TopUps.
		if invitedBy != nil && inviteReward.IsPositive() {
			if _, err := a.Wallets.TopUp(ctx, newWallet.ID, inviteReward, "invite:"+u.ID.String()); err != nil {
				log.Printf("HandleRegister: invitee reward: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to grant invite reward"})
				return
			}
			inviterWal, err := a.Wallets.FindByUser(ctx, *invitedBy, nil)
			if err != nil || inviterWal == nil {
				log.Printf("HandleRegister: inviter wallet lookup %s: %v", invitedBy, err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to grant invite reward"})
				return
			}
			if _, err := a.Wallets.TopUp(ctx, inviterWal.ID, inviteReward, "invite:"+u.ID.String()+":inviter"); err != nil {
				log.Printf("HandleRegister: inviter reward: %v", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to grant invite reward"})
				return
			}
		}

		token, _, err := generateLoginJWT(a, u)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate token"})
			return
		}
		completeConsoleLogin(a, w, r, u.ID, token)
		writeJSON(w, http.StatusCreated, registerResponse{
			Token: token,
			User:  userProfile{ID: u.ID.String(), Email: u.Email, Name: u.DisplayName},
		})
	}
}

// settingBool reads a boolean system setting, tolerating both JSON booleans and
// JSON-string booleans (as written by the admin API). Defaults to true.
func settingBool(a *app.App, r *http.Request, key string) bool {
	if a == nil || a.Settings == nil {
		return true
	}
	all, err := a.Settings.All(r.Context())
	if err != nil {
		return true
	}
	if v, ok := all[key]; ok {
		var b bool
		if json.Unmarshal(v, &b) == nil {
			return b
		}
		var s string
		if json.Unmarshal(v, &s) == nil {
			if p, err := strconv.ParseBool(s); err == nil {
				return p
			}
		}
	}
	return true
}

// HandleLogout clears the auth cookie and returns a success message.
func HandleLogout(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(a.Config.Cookie.Name); err == nil {
			revokeSessionToken(a, r, cookie.Value)
		}
		clearAuthCookie(w, a.Config)
		writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out"})
	}
}

// completeConsoleLogin sets the auth cookie and records the login session.
func completeConsoleLogin(a *app.App, w http.ResponseWriter, r *http.Request, userID uuid.UUID, token string) {
	setAuthCookie(w, token, a.Config)
	recordAuthSession(a, r, userID, token)
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
