package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"

	"github.com/deeptrols/api/internal/repository/tenant"
)

// TenantIdentification resolves tenant from Host header.
//
//   - Known platform hosts (localhost, 127.0.0.1, ::1, plus anything listed in
//     PLATFORM_HOSTS, e.g. docker service names) pass through with no tenant.
//   - A host bound to an active tenant domain resolves to that tenant.
//   - An unknown host, an inactive tenant, or a tenant-lookup DB failure is
//     REJECTED (fail-closed): we never let an unverifiable request fall through
//     to the platform.
func TenantIdentification(repo tenant.Repository, platformHosts []string) func(http.Handler) http.Handler {
	platform := make(map[string]bool, len(platformHosts)+4)
	for _, h := range []string{"localhost", "127.0.0.1", "::1", "[::1]", "0.0.0.0"} {
		platform[h] = true
	}
	for _, h := range platformHosts {
		platform[strings.ToLower(h)] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := stripPort(r.Host)
			tenantID := ""

			if host != "" {
				t, tenantErr := repo.FindByDomain(r.Context(), host)
				switch {
				case tenantErr == nil && t != nil && t.AllowTraffic():
					tenantID = t.ID.String()
				case tenantErr == nil:
					// Known DB state, but this host is not bound to an active
					// tenant — reject unless it is a platform-internal host.
					if !platform[strings.ToLower(host)] {
						rejectUnknownTenant(w)
						return
					}
				default:
					// DB error → fail-closed: cannot verify the tenant, so
					// reject rather than fall through to the platform.
					log.Printf("tenant: lookup error host=%s: %v", host, tenantErr)
					rejectUnknownTenant(w)
					return
				}
			}

			ctx := context.WithValue(r.Context(), CtxTenantID, tenantID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func rejectUnknownTenant(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte(`{"error":{"type":"unknown_tenant","message":"Unknown tenant domain"}}`))
}

// stripPort removes the port from a host string, handling IPv6 literals
// correctly ("[::1]:8080" → "[::1]", "example.com:8080" → "example.com").
func stripPort(host string) string {
	if strings.HasPrefix(host, "[") {
		if idx := strings.Index(host, "]"); idx != -1 {
			return host[:idx+1]
		}
		return host
	}
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		return host[:idx]
	}
	return host
}
