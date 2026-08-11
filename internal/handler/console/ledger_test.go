package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
)

// appForLedgerTest wires the repos needed by HandleUserLedger.
func appForLedgerTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	return &app.App{
		Pool: pool,
		Config: &config.Config{
			JWT: config.JWTConfig{
				Secret:      "test-jwt-secret-for-ledger-32-bytes",
				ExpiryHours: 24,
			},
		},
		Users:       user.NewPostgresRepository(pool),
		Tenants:     tenant.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// seedUserForLedgerTest creates a user of the given type and returns it.
func seedUserForLedgerTest(t *testing.T, a *app.App, email string, userType domain.UserType) *domain.User {
	t.Helper()
	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: "hashed",
		DisplayName:  "User " + email,
		Role:         "user",
		UserType:     userType,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForLedgerTest: %v", err)
	}
	return u
}

func TestHandleUserLedger_NoAuth(t *testing.T) {
	a := appForLedgerTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/ledger", nil)
	w := httptest.NewRecorder()

	HandleUserLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserLedger_NotAdmin(t *testing.T) {
	a := appForLedgerTest(t)
	nonAdmin := seedUserForTenantsTest(t, a, "nonadmin-ledger@test.com", "pass", "Non Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/ledger", nil)
	req = setNonAdminCtxForTenants(req, nonAdmin.ID.String())
	w := httptest.NewRecorder()

	HandleUserLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUserLedger_ReturnsUserTypeAndTenant(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-ledger@test.com", "pass", "Admin Ledger")
	tn := seedTenantForTest(t, a, "ledger-tenant", "Ledger Tenant", domain.TenantStatusActive)

	// Enterprise member with an active membership.
	enterpriseUser := seedUserForLedgerTest(t, a, "ent@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, enterpriseUser.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	// Personal user with no membership.
	personalUser := seedUserForLedgerTest(t, a, "personal@test.com", domain.UserTypePersonal)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users/ledger", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()

	HandleUserLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []userLedgerRow `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Total != 3 {
		t.Fatalf("total = %d, want 3 (admin + enterprise + personal), body: %s", resp.Total, w.Body.String())
	}

	byID := map[string]userLedgerRow{}
	for _, r := range resp.Data {
		byID[r.ID] = r
	}

	entRow, ok := byID[enterpriseUser.ID.String()]
	if !ok {
		t.Fatal("enterprise user missing from ledger")
	}
	if entRow.UserType != string(domain.UserTypeEnterprise) {
		t.Errorf("enterprise user_type = %q, want enterprise", entRow.UserType)
	}
	if entRow.TenantID != tn.ID.String() {
		t.Errorf("enterprise tenant_id = %q, want %s", entRow.TenantID, tn.ID.String())
	}
	if entRow.TenantName != "Ledger Tenant" {
		t.Errorf("enterprise tenant_name = %q, want Ledger Tenant", entRow.TenantName)
	}

	personalRow, ok := byID[personalUser.ID.String()]
	if !ok {
		t.Fatal("personal user missing from ledger")
	}
	if personalRow.UserType != string(domain.UserTypePersonal) {
		t.Errorf("personal user_type = %q, want personal", personalRow.UserType)
	}
	if personalRow.TenantID != "" {
		t.Errorf("personal tenant_id = %q, want empty", personalRow.TenantID)
	}
	if personalRow.TenantName != "" {
		t.Errorf("personal tenant_name = %q, want empty", personalRow.TenantName)
	}
}

// seedUsageLogForLedgerTest inserts a usage_log (and a backing api_key) for the
// given user and model, enough for the top_models aggregation to see it.
func seedUsageLogForLedgerTest(t *testing.T, a *app.App, userID uuid.UUID, model string) {
	t.Helper()
	keyID := uuid.New()
	now := time.Now().UTC()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, created_at, updated_at)
		 VALUES ($1, $2, 'dt-sk-', $3, $4, 'ledger key', $5, $5)`,
		keyID, userID, "hash-ledger-"+uuid.New().String()[:8], "dt-sk-****ledger", now); err != nil {
		t.Fatalf("seedUsageLogForLedgerTest: api key: %v", err)
	}
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO usage_logs (id, user_id, api_key_id, request_id, request_type,
		                         public_model_code, usage_source, list_cost, final_cost, status, created_at)
		 VALUES ($1, $2, $3, $4, 'chat', $5, 'upstream', 0, 0, 'completed', $6)`,
		uuid.New(), userID, keyID, "req-ledger-"+uuid.New().String()[:8], model, now); err != nil {
		t.Fatalf("seedUsageLogForLedgerTest: usage log: %v", err)
	}
}

func TestHandleUserLedger_TopModels(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-topmodels@test.com", "pass", "Admin")
	u := seedUserForLedgerTest(t, a, "topmodels@test.com", domain.UserTypePersonal)

	// gpt-4o x2, claude-sonnet x1, deepseek-v3 x1 → top 3 in that order.
	seedUsageLogForLedgerTest(t, a, u.ID, "gpt-4o")
	seedUsageLogForLedgerTest(t, a, u.ID, "gpt-4o")
	seedUsageLogForLedgerTest(t, a, u.ID, "claude-sonnet")
	seedUsageLogForLedgerTest(t, a, u.ID, "deepseek-v3")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/ledger", nil)
	req = setAdminCtxForTenants(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleUserLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []userLedgerRow `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	var row userLedgerRow
	for _, r := range resp.Data {
		if r.ID == u.ID.String() {
			row = r
		}
	}
	if row.ID == "" {
		t.Fatal("seeded user missing from ledger")
	}
	if row.RequestCount != 4 {
		t.Errorf("request_count = %d, want 4", row.RequestCount)
	}
	want := []string{"gpt-4o", "claude-sonnet", "deepseek-v3"}
	if len(row.TopModels) != len(want) {
		t.Fatalf("top_models = %v, want %v", row.TopModels, want)
	}
	for i := range want {
		if row.TopModels[i] != want[i] {
			t.Errorf("top_models[%d] = %q, want %q (full: %v)", i, row.TopModels[i], want[i], row.TopModels)
		}
	}
}
