// Package auth is deprecated. RequestIdentity has moved to internal/domain.
// The Service type and ValidateAPIKey are no longer used — the gateway
// middleware (internal/handler/middleware/auth.go) calls the API key repository
// directly with HMAC-SHA256 via pkg/keyhash.
package auth
