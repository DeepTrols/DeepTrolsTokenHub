package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
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

// contextWithUser injects a fake actor id into the request context.
func contextWithUser(ctx context.Context) context.Context {
	return context.WithValue(ctx, userIDKey{}, uuid.New().String())
}

type userIDKey struct{}
