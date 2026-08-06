package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// recordAudit inserts a row into audit_logs. Errors are logged, never returned:
// a failed audit write must not fail the underlying business operation.
func recordAudit(ctx context.Context, pool *pgxpool.Pool, actorID *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, ip string) {
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, actor_type, tenant_id, action, resource_type, resource_id, ip_address, created_at)
		 VALUES ($1, 'user', NULL, $2, $3, $4, $5, $6)`,
		actorID, action, resourceType, resourceID, ip, time.Now().UTC(),
	)
	if err != nil {
		log.Printf("audit: record %s %s: %v", action, resourceType, err)
	}
}

// AuditAdminWrite returns middleware that records every admin write request
// (POST/PUT/DELETE) to audit_logs. The actor is taken from the JWT context
// (set by ConsoleAuth); the action is "HTTP_METHOD /path"; the resource type
// is the first path segment after /admin.
func AuditAdminWrite(pool *pgxpool.Pool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := r.Method
			if method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete {
				var actorID *uuid.UUID
				if uid, err := jwtutil.UserIDFromContext(r.Context()); err == nil {
					actorID = &uid
				}

				resourceType := "unknown"
				segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
				// Path is /api/admin/{resource}/...
				for i, seg := range segments {
					if seg == "admin" && i+1 < len(segments) {
						resourceType = segments[i+1]
						break
					}
				}

				var resourceID *uuid.UUID
				for _, seg := range segments {
					if id, err := uuid.Parse(seg); err == nil {
						resourceID = &id
						break
					}
				}

				recordAudit(r.Context(), pool, actorID, method+" "+r.URL.Path, resourceType, resourceID, extractIPFromRemoteAddr(r.RemoteAddr))
			}
			next.ServeHTTP(w, r)
		})
	}
}
