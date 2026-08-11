package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/pkg/keyhash"
	"github.com/google/uuid"
)

type contextKey string

const (
	CtxAPIKeyID    contextKey = "api_key_id"
	CtxTenantID    contextKey = "tenant_id"
	CtxRequestID   contextKey = "request_id"
	CtxRequestType contextKey = "request_type"
)

// Re-export jwtutil context keys for convenience.
// Values stored at these keys in ConsoleAuth can be extracted
// via jwtutil.UserIDFromContext / jwtutil.EmailFromContext.
//
// Note: CtxTenantID (gateway domain-based tenant) is intentionally NOT
// aliased to jwtutil.CtxTenantIDKey — the gateway chain and console chain
// use the same string key "tenant_id" but store different kinds of tenant
// identifiers (gateway: domain-mapped; console: enterprise membership).
var (
	CtxUserID     = jwtutil.CtxUserIDKey
	CtxEmail      = jwtutil.CtxEmailKey
	CtxUserName   = jwtutil.CtxUserNameKey
	CtxRoleKey    = jwtutil.CtxRoleKey
	CtxUserType   = jwtutil.CtxUserTypeKey
	CtxTenantRole = jwtutil.CtxTenantRoleKey
)

// GatewayAuth validates API Key for OpenAI-compatible endpoints.
// It hashes the Bearer token, looks it up in the database, and stores
// the resolved identity (key ID, user ID) in the request context.
func GatewayAuth(application *app.App) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				writeGatewayError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
				return
			}

			apiKey := strings.TrimPrefix(authHeader, "Bearer ")
			if apiKey == "" {
				writeGatewayError(w, http.StatusUnauthorized, "Empty API key")
				return
			}

			// Resolve request ID (case-insensitive header check).
			requestID := r.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = r.Header.Get("x-request-id")
			}
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Hash the plaintext key and look it up in the database.
			keyHash := keyhash.Hash(apiKey, application.Config.Encryption.Key)
			key, err := application.APIKeys.FindByHash(r.Context(), keyHash)
			if err != nil || key == nil {
				writeGatewayError(w, http.StatusUnauthorized, "Invalid API key")
				return
			}

			if !key.IsUsable() {
				writeGatewayError(w, http.StatusForbidden, "API key is disabled")
				return
			}

			// Store resolved identity in context.
			ctx := r.Context()
			ctx = context.WithValue(ctx, CtxAPIKeyID, key.ID.String())
			ctx = context.WithValue(ctx, CtxUserID, key.UserID.String())
			ctx = context.WithValue(ctx, CtxRequestID, requestID)
			ctx = context.WithValue(ctx, CtxRequestType, "chat")

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeGatewayError writes a JSON error response with the standard gateway format.
func writeGatewayError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": message,
			"type":    "auth_error",
		},
	})
}

// ConsoleAuth validates JWT for console API endpoints.
// It reads the token from an httpOnly cookie first, then falls back
// to the Authorization Bearer header.
//
// After JWT signature validation the user is re-fetched from the database on
// EVERY request so that disabled/deleted accounts are rejected immediately and
// role changes (e.g. admin demoted to user) take effect on the next request,
// not at JWT expiry (stateless-token revocation).
func ConsoleAuth(application *app.App) func(http.Handler) http.Handler {
	cfg := application.Config
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ""

			// Try cookie first.
			if cookie, err := r.Cookie(cfg.Cookie.Name); err == nil && cookie.Value != "" {
				token = cookie.Value
			}

			// Fall back to Authorization header.
			if token == "" {
				authHeader := r.Header.Get("Authorization")
				if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
					token = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if token == "" {
				writeAuthError(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
				return
			}

			claims, err := jwtutil.ParseToken(token, cfg.JWT.Secret)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "Invalid or expired token")
				return
			}

			// Live account check: reject deleted/disabled users immediately
			// even though their JWT is still technically valid.
			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "Invalid token subject")
				return
			}
			dbUser, err := application.Users.FindByID(r.Context(), userID)
			if err != nil || dbUser == nil || dbUser.Status != domain.UserStatusActive {
				writeAuthError(w, http.StatusUnauthorized, "Account is not active")
				return
			}

			ctx := r.Context()
			ctx = context.WithValue(ctx, CtxUserID, dbUser.ID.String())
			ctx = context.WithValue(ctx, CtxEmail, dbUser.Email)
			ctx = context.WithValue(ctx, CtxUserName, dbUser.DisplayName)
			// Role always comes from the DB (live), never from the token.
			ctx = context.WithValue(ctx, CtxRoleKey, dbUser.Role)
			// User type from the DB.
			ctx = context.WithValue(ctx, CtxUserType, string(dbUser.UserType))

			// Look up enterprise membership (personal users won't have one).
			// Gracefully skip when Memberships is nil (e.g. in tests that use a partial App).
			if application.Memberships != nil {
				membership, err := application.Memberships.FindByUserID(ctx, dbUser.ID)
				if err == nil && membership != nil && membership.Status == domain.MembershipStatusActive {
					ctx = context.WithValue(ctx, jwtutil.CtxTenantIDKey, membership.TenantID.String())
					ctx = context.WithValue(ctx, CtxTenantRole, string(membership.Role))
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// AdminAuth checks that the request context contains role="admin".
// It must be used after ConsoleAuth, which sets the role in context.
// Returns 403 Forbidden if the role is missing or not "admin".
func AdminAuth() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, err := jwtutil.RoleFromContext(r.Context())
			if err != nil || role != "admin" {
				writeAuthError(w, http.StatusForbidden, "Admin access required")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
