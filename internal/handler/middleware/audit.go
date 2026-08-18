package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// auditOldValueHolder is a mutable snapshot slot shared by reference between
// the audit middleware and the handler. A context value is immutable once the
// request is created, so the handler cannot attach a pre-mutation snapshot by
// calling r.WithContext — it mutates the holder instead, and the middleware
// reads it after the handler returns.
type auditOldValueHolder struct {
	mu sync.Mutex
	v  any
}

func (h *auditOldValueHolder) set(v any) {
	h.mu.Lock()
	h.v = v
	h.mu.Unlock()
}

func (h *auditOldValueHolder) get() any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.v
}

type auditOldValueKey struct{}

// WithAuditOldValue injects a mutable old-value holder into ctx.
func WithAuditOldValue(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditOldValueKey{}, &auditOldValueHolder{})
}

// SetAuditOldValue stores a JSON-able snapshot on the holder carried by ctx.
// It is a no-op when AuditAdminWrite has not injected a holder.
func SetAuditOldValue(ctx context.Context, v any) {
	if h, ok := ctx.Value(auditOldValueKey{}).(*auditOldValueHolder); ok {
		h.set(v)
	}
}

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

				action := method + " " + r.URL.Path
				ip := extractIPFromRemoteAddr(r.RemoteAddr)
				// Inject the mutable holder and record AFTER the handler runs
				// so a snapshot attached by the handler (e.g. the tenant row
				// before a hard delete) is persisted. defer keeps the audit
				// write even if the handler panics.
				ctx := WithAuditOldValue(r.Context())
				defer func() {
					holder := ctx.Value(auditOldValueKey{}).(*auditOldValueHolder)
					recordAudit(r.Context(), pool, actorID, action, resourceType, resourceID, ip, holder.get())
				}()
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
