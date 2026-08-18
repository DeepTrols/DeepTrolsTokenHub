package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

func TestAuditAdminWrite_Integration(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateTables(t, pool, "audit_logs")

	mw := AuditAdminWrite(pool)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/admin/models", nil)
	req.RemoteAddr = "10.0.0.9:1234"
	req = req.WithContext(contextWithUser(req.Context()))
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	var count int
	err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM audit_logs WHERE action LIKE 'POST %'`).Scan(&count)
	if err != nil {
		t.Fatalf("query audit count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 audit row, got %d", count)
	}
}

// TestAuditAdminWrite_Integration_WithOldValue verifies that a handler can
// attach an old-value snapshot to the request context and the audit
// middleware persists it into audit_logs.old_value (used by hard-delete
// endpoints where the row is gone after the operation).
func TestAuditAdminWrite_Integration_WithOldValue(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateTables(t, pool, "audit_logs")

	mw := AuditAdminWrite(pool)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/tenants/11111111-1111-1111-1111-111111111111", nil)
	req.RemoteAddr = "10.0.0.10:1234"
	ctx := contextWithUser(req.Context())
	ctx = context.WithValue(ctx, CtxAuditOldValue, map[string]any{
		"code":   "acme",
		"name":   "Acme Corp",
		"status": "active",
	})
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	mw(next).ServeHTTP(w, req)

	var oldValue []byte
	err := pool.QueryRow(context.Background(),
		`SELECT old_value FROM audit_logs WHERE action LIKE 'DELETE %'`).Scan(&oldValue)
	if err != nil {
		t.Fatalf("query audit old_value: %v", err)
	}
	if !strings.Contains(string(oldValue), "Acme Corp") {
		t.Errorf("old_value = %s, want it to contain the tenant identity", oldValue)
	}
}

// contextWithUser injects a fake actor id into the request context.
func contextWithUser(ctx context.Context) context.Context {
	return context.WithValue(ctx, userIDKey{}, uuid.New().String())
}

type userIDKey struct{}
