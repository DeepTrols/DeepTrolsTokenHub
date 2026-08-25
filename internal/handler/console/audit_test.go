package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestHandleListAuditLogs(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	admin := &domain.User{ID: uuid.New(), Email: "audit-admin@test.local",
		PasswordHash: string(hash), DisplayName: "Audit Admin", Role: "admin", Status: domain.UserStatusActive}
	if err := user.NewPostgresRepository(pool).Create(ctx, admin); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO audit_logs (actor_id, actor_type, action, resource_type, resource_id, new_value, created_at)
		 VALUES ($1, 'user', 'guardrail_blocked', 'guardrail', $2, '{"reason_code":"x"}', NOW())`,
		admin.ID, uuid.New()); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	a := &app.App{Pool: pool, Config: &config.Config{}}
	req := httptest.NewRequest(http.MethodGet, "/api/admin/audit", nil)
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxUserIDKey, admin.ID.String()))
	req = req.WithContext(context.WithValue(req.Context(), jwtutil.CtxRoleKey, "admin"))
	w := httptest.NewRecorder()

	HandleListAuditLogs(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Data  []auditLogResponse `json:"data"`
		Total int                `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 1 || resp.Data[0].Action != "guardrail_blocked" || resp.Data[0].ActorEmail != "audit-admin@test.local" {
		t.Fatalf("resp = %+v", resp)
	}
}
