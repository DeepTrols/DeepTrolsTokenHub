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
	"github.com/deeptrols/api/internal/pkg/totp"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code,omitempty"`
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
	ID          string `json:"id"`
	Email       string `json:"email"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	TOTPEnabled bool   `json:"totp_enabled"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
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

			// If TOTP is enabled, require a valid TOTP code.
			if dbUser.TOTPEnabled {
				if req.TOTPCode == "" {
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "TOTP code required", "mfa_required": "true"})
					return
				}
				valid, err := totp.Validate(dbUser.TOTPSecret, req.TOTPCode, 1)
				if err != nil || !valid {
					recordLoginHistory(ctx, a, dbUser.ID, ipAddress, userAgent, false)
					writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Invalid TOTP code"})
					return
				}
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

// HandleTOTPSetup generates a TOTP secret and stores it for the authenticated user.
func HandleTOTPSetup(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		dbUser, err := a.Users.FindByID(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "User not found"})
			return
		}

		if dbUser.TOTPEnabled {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "TOTP already enabled"})
			return
		}

		secret, err := totp.GenerateSecret()
		if err != nil {
			log.Printf("HandleTOTPSetup: generate secret: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to generate TOTP secret"})
			return
		}

		// Store the secret (unverified — only committed after verify)
		_, err = a.Pool.Exec(r.Context(),
			`UPDATE users SET totp_secret = $2, updated_at = NOW() WHERE id = $1`,
			userID, secret)
		if err != nil {
			log.Printf("HandleTOTPSetup: store secret: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to store TOTP secret"})
			return
		}

		qrURL := totp.GenerateKeyURI(secret, dbUser.Email, a.Config.TOTP.Issuer)

		writeJSON(w, http.StatusOK, map[string]string{
			"secret": secret,
			"qr_url": qrURL,
		})
	}
}

// HandleTOTPVerify validates a TOTP code and enables MFA for the user.
func HandleTOTPVerify(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := jwtutil.UserIDFromContext(r.Context())
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "Not authenticated"})
			return
		}

		var req struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
			return
		}

		if len(req.Code) != 6 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Code must be 6 digits"})
			return
		}

		dbUser, err := a.Users.FindByID(r.Context(), userID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "User not found"})
			return
		}

		if dbUser.TOTPSecret == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "TOTP not set up — call /auth/totp/setup first"})
			return
		}

		valid, err := totp.Validate(dbUser.TOTPSecret, req.Code, 1)
		if err != nil {
			log.Printf("HandleTOTPVerify: validate: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Verification failed"})
			return
		}

		if !valid {
			writeJSON(w, http.StatusOK, map[string]interface{}{"verified": false, "error": "Invalid code"})
			return
		}

		// Enable TOTP
		_, err = a.Pool.Exec(r.Context(),
			`UPDATE users SET totp_enabled = true, updated_at = NOW() WHERE id = $1`,
			userID)
		if err != nil {
			log.Printf("HandleTOTPVerify: enable: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to enable TOTP"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{"verified": true})
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
		writeJSON(w, http.StatusOK, meResponse{
			ID: dbUser.ID.String(), Email: dbUser.Email, Name: dbUser.DisplayName,
			Role: dbUser.Role, Status: string(dbUser.Status),
			TOTPEnabled: dbUser.TOTPEnabled,
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
		u := &domain.User{
			ID: uuid.New(), Email: req.Email, PasswordHash: string(hash),
			DisplayName: req.Name, Role: "user", Status: domain.UserStatusActive,
			CreatedAt: now, UpdatedAt: now,
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
	tok, err := jwtutil.GenerateToken(u.ID, u.Email, u.DisplayName, u.Role, a.Config.JWT.Secret, a.Config.JWT.ExpiryHours)
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
		`INSERT INTO users (id, email, password_hash, display_name, role, status, created_at, updated_at)
		 VALUES ($1, $2, $3, 'Administrator', 'admin', 'active', NOW(), NOW())
		 ON CONFLICT (id) DO UPDATE SET email=$2, password_hash=$3, role='admin', status='active'`,
		adminID, a.Config.Bootstrap.AdminEmail, string(hash),
	)
	return adminID
}

func generateBootstrapJWT(a *app.App) (token, expiry string, err error) {
	expiryTime := fmt.Sprintf("%d", a.Config.JWT.ExpiryHours)
	tok, err := jwtutil.GenerateToken(uuid.Nil, a.Config.Bootstrap.AdminEmail, "Administrator", "admin", a.Config.JWT.Secret, a.Config.JWT.ExpiryHours)
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
