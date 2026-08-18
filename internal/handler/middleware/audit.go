package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CtxAuditOldValue carries a JSON-able snapshot of the resource being
// mutated (e.g. the full tenant before a hard delete). Handlers set it on the
// request context; AuditAdminWrite persists it into audit_logs.old_value so
// the operator can reconstruct what was removed.
type ctxAuditOldValueKey struct{}

// CtxAuditOldValue is the context key for the audit old-value snapshot.
var CtxAuditOldValue = ctxAuditOldValueKey{}

// recordAudit inserts a row into audit_logs. Errors are logged, never returned:
// a failed audit write must not fail the underlying business operation.
func recordAudit(ctx context.Context, pool *pgxpool.Pool, actorID *uuid.UUID, action, resourceType string, resourceID *uuid.UUID, ip string, oldValue any) {
	var oldValueJSON []byte
	if oldValue != nil {
		if b, err := json.Marshal(oldValue); err == nil {
			oldValueJSON = b
		} else {
			log.Printf("audit: marshal old_value for %s %s: %v", action, resourceType, err)
		}
	}

	_, err := pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, actor_type, tenant_id, action, resource_type, resource_id, old_value, ip_address, created_at)
		 VALUES ($1, 'user', NULL, $2, $3, $4, $5, $6, $7)`,
		actorID, action, resourceType, resourceID, oldValueJSON, ip, time.Now().UTC(),
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

				oldValue := r.Context().Value(CtxAuditOldValue)
				recordAudit(r.Context(), pool, actorID, method+" "+r.URL.Path, resourceType, resourceID, extractIPFromRemoteAddr(r.RemoteAddr), oldValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}
