package middleware

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/ratelimit"
)

// LoginRateLimit returns middleware that rate-limits requests by IP address
// using the provided RateLimiter. When the limit is exceeded, it returns 429
// with a JSON error body and a Retry-After header.
func LoginRateLimit(limiter ratelimit.RateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := "rl:login:" + extractIPFromRemoteAddr(r.RemoteAddr)

			allowed, retryAfter, err := limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				// Fail-open on limiter errors (should be rare since a
				// FallbackRateLimiter swallows them), so a limiter outage
				// never takes down the service.
				log.Printf("ratelimit: login allow error: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				writeRateLimited(w, "Too many requests, please try again later", retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GatewayRateLimit rate-limits by API key ID (from GatewayAuth context) or IP
// fallback, using the provided RateLimiter.
func GatewayRateLimit(limiter ratelimit.RateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := ""
			if v, _ := r.Context().Value(CtxAPIKeyID).(string); v != "" {
				identifier = v
			} else {
				identifier = extractIPFromRemoteAddr(r.RemoteAddr)
			}
			key := "rl:gw:" + identifier

			allowed, retryAfter, err := limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				log.Printf("ratelimit: gateway allow error: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				writeRateLimited(w, "Rate limit exceeded", retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// TeamRateLimit rate-limits team management actions by console user ID (from
// ConsoleAuth context) with an IP fallback. Applied to the /team group so an
// authenticated user cannot spam invites, status toggles, or ownership
// transfers. Must run after ConsoleAuth, which sets CtxUserID.
func TeamRateLimit(limiter ratelimit.RateLimiter, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			identifier := ""
			if v, _ := r.Context().Value(CtxUserID).(string); v != "" {
				identifier = v
			} else {
				identifier = extractIPFromRemoteAddr(r.RemoteAddr)
			}
			key := "rl:team:" + identifier

			allowed, retryAfter, err := limiter.Allow(r.Context(), key, limit, window)
			if err != nil {
				log.Printf("ratelimit: team allow error: %v", err)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				writeRateLimited(w, "Too many requests, please try again later", retryAfter)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// writeRateLimited writes a 429 response with a Retry-After header.
// The header is ceil(remaining seconds)+1 so a client retrying at the exact
// window boundary is nudged just past it.
func writeRateLimited(w http.ResponseWriter, message string, retryAfter time.Duration) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// extractIPFromRemoteAddr extracts the IP portion from a remote address
// string in the format "host:port" or "ip:port".
func extractIPFromRemoteAddr(remoteAddr string) string {
	if idx := strings.LastIndex(remoteAddr, ":"); idx != -1 {
		return remoteAddr[:idx]
	}
	return remoteAddr
}
