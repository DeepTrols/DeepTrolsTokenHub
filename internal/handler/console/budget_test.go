package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/budget"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func TestBudgetApproveFlow(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	admin := &domain.User{
		ID: uuid.New(), Email: "budget-admin@test.local",
		PasswordHash: string(hash), DisplayName: "Budget Admin",
		Role: "admin", Status: domain.UserStatusActive,
	}
	if err := user.NewPostgresRepository(pool).Create(ctx, admin); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	tenantID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenants (id, code, name) VALUES ($1, 'flow-tenant', 'Flow Tenant')`,
		tenantID); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO tenant_memberships (id, tenant_id, user_id, role, status)
		 VALUES ($1, $2, $3, 'owner', 'active')`,
		uuid.New(), tenantID, admin.ID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	a := &app.App{
		Pool: pool, Config: &config.Config{},
		Budgets: budget.NewPostgresRepository(pool), Memberships: membership.NewPostgresRepository(pool),
	}
	router := chi.NewRouter()
	router.Get("/team/budget", HandleGetTeamBudget(a))
	router.Post("/team/budget/requests", HandleCreateBudgetRequest(a))
	router.Post("/admin/budgets/requests/{id}/approve", HandleApproveBudgetRequest(a))

	// Enterprise admin creates an increase request.
	tenantCtx := context.WithValue(ctx, jwtutil.CtxUserIDKey, admin.ID.String())
	tenantCtx = context.WithValue(tenantCtx, jwtutil.CtxTenantIDKey, tenantID.String())
	req := httptest.NewRequest(http.MethodPost, "/team/budget/requests",
		bytes.NewBufferString(`{"amount":"800","reason":"扩容"}`))
	req = req.WithContext(tenantCtx)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("create request status = %d, body: %s", w.Code, w.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("create request returned no id")
	}

	// Admin approves.
	adminCtx := context.WithValue(ctx, jwtutil.CtxUserIDKey, admin.ID.String())
	adminCtx = context.WithValue(adminCtx, jwtutil.CtxRoleKey, "admin")
	req2 := httptest.NewRequest(http.MethodPost, "/admin/budgets/requests/"+created.ID+"/approve", nil)
	req2 = req2.WithContext(adminCtx)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("approve status = %d, body: %s", w2.Code, w2.Body.String())
	}

	// Budget now visible to the tenant admin.
	req3 := httptest.NewRequest(http.MethodGet, "/team/budget", nil)
	req3 = req3.WithContext(tenantCtx)
	w3 := httptest.NewRecorder()
	router.ServeHTTP(w3, req3)
	if w3.Code != http.StatusOK {
		t.Fatalf("get budget status = %d", w3.Code)
	}
	if !bytes.Contains(w3.Body.Bytes(), []byte(`"limit_amount":"800"`)) {
		t.Errorf("budget body = %s, want limit 800", w3.Body.String())
	}
}
