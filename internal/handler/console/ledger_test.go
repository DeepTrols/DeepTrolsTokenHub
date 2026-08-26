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
	"github.com/shopspring/decimal"
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
	tn2 := seedTenantForTest(t, a, "ledger-tenant-2", "Ledger Tenant 2", domain.TenantStatusActive)

	// Enterprise owner with an active owner membership — must be shown.
	enterpriseUser := seedUserForLedgerTest(t, a, "ent@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, enterpriseUser.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	// Enterprise employee sub-accounts (member and admin) — balance rolls into the
	// enterprise account, so both must be excluded from the ledger.
	employeeUser := seedUserForLedgerTest(t, a, "emp@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, employeeUser.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	adminEmployee := seedUserForLedgerTest(t, a, "empadmin@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminEmployee.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	// 员工钱包并入企业账号：enterpriseUser 行应等于这两个员工钱包之和（各只统计一次）。
	seedWalletForLedgerTest(t, a, employeeUser.ID, "100")
	seedWalletForLedgerTest(t, a, adminEmployee.ID, "50")
	// Hybrid: owner of tn2 yet employee of tn — still an enterprise account, must be shown,
	// and its amounts must NOT roll into tn's owner (would double count).
	hybridUser := seedUserForLedgerTest(t, a, "hybrid@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn2.ID, hybridUser.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, hybridUser.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
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
	// admin + enterprise owner + hybrid + personal; both employee sub-accounts excluded.
	if resp.Total != 4 {
		t.Fatalf("total = %d, want 4 (admin + enterprise owner + hybrid + personal, employees excluded), body: %s", resp.Total, w.Body.String())
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

	// 企业员工子账号的余额归并到企业账号，账务列表不应展示（member 与 admin 角色均排除）。
	for _, emp := range []*domain.User{employeeUser, adminEmployee} {
		if _, ok := byID[emp.ID.String()]; ok {
			t.Errorf("enterprise employee sub-account %s must be excluded from ledger", emp.Email)
		}
	}

	// 兼任其他企业员工的 owner 仍是企业账号，必须保留。
	if _, ok := byID[hybridUser.ID.String()]; !ok {
		t.Error("enterprise owner who is also an employee elsewhere must be shown in ledger")
	}

	// 员工余额并入企业账号且只并入一次：enterpriseUser 行 = 100 + 50。
	if got := mustDecimalForLedgerTest(t, entRow.Balance); !got.Equal(decimal.NewFromInt(150)) {
		t.Errorf("enterprise user balance = %s, want 150 (employee wallets rolled in exactly once)", entRow.Balance)
	}
	// 兼任 tn2 owner 的 hybrid 不接收 tn 的员工归并，其自身余额为 0。
	hyRow, ok := byID[hybridUser.ID.String()]
	if !ok {
		t.Fatal("hybrid owner missing from ledger")
	}
	if got := mustDecimalForLedgerTest(t, hyRow.Balance); !got.Equal(decimal.Zero) {
		t.Errorf("hybrid balance = %s, want 0 (no employee roll-up into non-owner role)", hyRow.Balance)
	}

	// 无调用记录的账号必须返回空数组（而非 null），前端才能安全读 .length。
	for _, r := range resp.Data {
		if r.ModelUsage == nil {
			t.Errorf("user %s model_usage is null, want empty array", r.ID)
		}
	}
}

// seedUsageLogForLedgerTest inserts a usage_log (and a backing api_key) for the
// given user and model, with a fixed per-call footprint: 15 tokens and a final
// cost of 1, so the model_usage aggregation yields predictable numbers.
func seedUsageLogForLedgerTest(t *testing.T, a *app.App, userID uuid.UUID, model string) {
	t.Helper()
	keyID := uuid.New()
	now := time.Now().UTC()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, created_at, updated_at)
		 VALUES ($1, $2, 'sk-', $3, $4, 'ledger key', $5, $5)`,
		keyID, userID, "hash-ledger-"+uuid.New().String()[:8], "sk-****ledger", now); err != nil {
		t.Fatalf("seedUsageLogForLedgerTest: api key: %v", err)
	}
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO usage_logs (id, user_id, api_key_id, request_id, request_type,
		                         public_model_code, usage_source, usage_normalized, usage_raw,
		                         list_cost, final_cost, status, created_at)
		 VALUES ($1, $2, $3, $4, 'chat', $5, 'upstream', '{"input_tokens": 10, "output_tokens": 5}'::jsonb,
		         '{"total_tokens": 15}'::jsonb,
		         0, 1, 'completed', $6)`,
		uuid.New(), userID, keyID, "req-ledger-"+uuid.New().String()[:8], model, now); err != nil {
		t.Fatalf("seedUsageLogForLedgerTest: usage log: %v", err)
	}
}

// TestHandleUserLedger_ModelUsage verifies every called model is listed with its
// own aggregated call count, tokens and cost — not just a top-3 slice.
func TestHandleUserLedger_ModelUsage(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-modelusage@test.com", "pass", "Admin")
	u := seedUserForLedgerTest(t, a, "modelusage@test.com", domain.UserTypePersonal)

	// gpt-4o x2, claude-sonnet x1, deepseek-v3 x1 → all three must appear.
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

	if len(row.ModelUsage) != 3 {
		t.Fatalf("model_usage has %d entries, want 3 (full: %v)", len(row.ModelUsage), row.ModelUsage)
	}
	byModel := map[string]modelUsageRow{}
	for _, mu := range row.ModelUsage {
		byModel[mu.Model] = mu
	}

	wantCalls := map[string]int64{"gpt-4o": 2, "claude-sonnet": 1, "deepseek-v3": 1}
	for model, calls := range wantCalls {
		mu, ok := byModel[model]
		if !ok {
			t.Errorf("model %q missing from model_usage", model)
			continue
		}
		if mu.Calls != calls {
			t.Errorf("model %q calls = %d, want %d", model, mu.Calls, calls)
		}
		// 15 tokens and cost 1 per call (see seedUsageLogForLedgerTest).
		if mu.Tokens != calls*15 {
			t.Errorf("model %q tokens = %d, want %d", model, mu.Tokens, calls*15)
		}
		if mu.Cost == "" {
			t.Errorf("model %q cost is empty", model)
		}
	}
}

// seedWalletForLedgerTest creates a personal wallet (tenant_id IS NULL) with the
// given balance, mirroring the wallet provisioned at registration.
func seedWalletForLedgerTest(t *testing.T, a *app.App, userID uuid.UUID, balance string) uuid.UUID {
	t.Helper()
	now := time.Now().UTC()
	wid := uuid.New()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, '0', 'CNY', 0, $4, $4)`,
		wid, userID, balance, now); err != nil {
		t.Fatalf("seedWalletForLedgerTest: %v", err)
	}
	return wid
}

// seedTopupForLedgerTest records a topup against the given wallet, so the ledger's
// cumulative topup aggregation has something to sum.
func seedTopupForLedgerTest(t *testing.T, a *app.App, walletID uuid.UUID, amount string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, created_at)
		 VALUES ($1, $2, $3, 'topup', $4, 0, $4, $5)`,
		uuid.New(), walletID, "topup-"+uuid.New().String(), amount, now); err != nil {
		t.Fatalf("seedTopupForLedgerTest: %v", err)
	}
}

// mustDecimalForLedgerTest parses a ledger money string into a decimal for exact
// assertion, instead of comparing fragile fixed-scale string representations.
func mustDecimalForLedgerTest(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("parse amount %q: %v", s, err)
	}
	return d
}

// TestHandleUserLedger_RollsUpEmployeeActivity verifies that an enterprise
// employee's wallet (balance/frozen/topup) and usage (spend/requests/tokens,
// plus per-model breakdown) are attributed to the tenant owner's row, while the
// employee row itself is hidden. An account that is itself an owner elsewhere is
// never rolled into another enterprise (avoids double counting).
func TestHandleUserLedger_RollsUpEmployeeActivity(t *testing.T) {
	a := appForLedgerTest(t)
	admin := seedUserForTenantsTest(t, a, "admin-rollup@test.com", "pass", "Admin Rollup")
	tn := seedTenantForTest(t, a, "rollup-tenant", "Rollup Tenant", domain.TenantStatusActive)

	owner := seedUserForLedgerTest(t, a, "owner@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	seedWalletForLedgerTest(t, a, owner.ID, "1000")

	emp := seedUserForLedgerTest(t, a, "emp@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, emp.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	empWallet := seedWalletForLedgerTest(t, a, emp.ID, "200")
	seedTopupForLedgerTest(t, a, empWallet, "300")
	seedUsageLogForLedgerTest(t, a, emp.ID, "gpt-4o")
	seedUsageLogForLedgerTest(t, a, emp.ID, "gpt-4o")

	emp2 := seedUserForLedgerTest(t, a, "emp2@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, emp2.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletForLedgerTest(t, a, emp2.ID, "50")

	adminEmp := seedUserForLedgerTest(t, a, "adminemp@test.com", domain.UserTypeEnterprise)
	_ = seedMembershipForAdminTest(t, a, tn.ID, adminEmp.ID, domain.MembershipRoleAdmin, domain.MembershipStatusActive)
	seedWalletForLedgerTest(t, a, adminEmp.ID, "30")

	// 兼任其他企业 owner 的员工不并入本企业（避免重复统计），且自身仍作为企业账号展示。
	hybrid := seedUserForLedgerTest(t, a, "hybrid@test.com", domain.UserTypeEnterprise)
	tn2 := seedTenantForTest(t, a, "rollup-tenant-2", "Rollup Tenant 2", domain.TenantStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn2.ID, hybrid.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, hybrid.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletForLedgerTest(t, a, hybrid.ID, "700")

	personal := seedUserForLedgerTest(t, a, "personal@test.com", domain.UserTypePersonal)
	seedWalletForLedgerTest(t, a, personal.ID, "9")

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
	byID := map[string]userLedgerRow{}
	for _, r := range resp.Data {
		byID[r.ID] = r
	}
	// 员工子账号不单独成行。
	for _, em := range []*domain.User{emp, emp2, adminEmp} {
		if _, ok := byID[em.ID.String()]; ok {
			t.Errorf("employee sub-account %s must be hidden from ledger", em.Email)
		}
	}

	// owner 行 = 自有钱包 + 员工钱包（200 + 50 + 30）+ 员工充值 300。
	or, ok := byID[owner.ID.String()]
	if !ok {
		t.Fatal("enterprise owner missing from ledger")
	}
	if got := mustDecimalForLedgerTest(t, or.Balance); !got.Equal(decimal.NewFromInt(1000 + 200 + 50 + 30)) {
		t.Errorf("owner balance = %s, want 1280 (own 1000 + emp 200 + 50 + 30)", or.Balance)
	}
	if got := mustDecimalForLedgerTest(t, or.TotalTopup); !got.Equal(decimal.NewFromInt(300)) {
		t.Errorf("owner total_topup = %s, want 300 (emp topup)", or.TotalTopup)
	}
	// 2 次调用，每次 final_cost=1 / 15 tokens。
	if got := mustDecimalForLedgerTest(t, or.TotalSpend); !got.Equal(decimal.NewFromInt(2)) {
		t.Errorf("owner total_spend = %s, want 2", or.TotalSpend)
	}
	if or.RequestCount != 2 {
		t.Errorf("owner request_count = %d, want 2", or.RequestCount)
	}
	if or.TotalTokens != 30 {
		t.Errorf("owner total_tokens = %d, want 30", or.TotalTokens)
	}
	// 员工调用模型归并到 owner 行的模型明细。
	var gpt *modelUsageRow
	for i := range or.ModelUsage {
		if or.ModelUsage[i].Model == "gpt-4o" {
			gpt = &or.ModelUsage[i]
		}
	}
	if gpt == nil {
		t.Fatalf("owner model_usage missing gpt-4o (got %v)", or.ModelUsage)
	}
	if gpt.Calls != 2 {
		t.Errorf("owner gpt-4o calls = %d, want 2", gpt.Calls)
	}
	if gpt.Tokens != 30 {
		t.Errorf("owner gpt-4o tokens = %d, want 30", gpt.Tokens)
	}

	// 兼任他企 owner 的员工只统计自己的钱包，不计入本企业。
	hr, ok := byID[hybrid.ID.String()]
	if !ok {
		t.Fatal("hybrid owner missing from ledger")
	}
	if got := mustDecimalForLedgerTest(t, hr.Balance); !got.Equal(decimal.NewFromInt(700)) {
		t.Errorf("hybrid balance = %s, want 700 (own only)", hr.Balance)
	}
	// 个人用户余额不受影响。
	pr, ok := byID[personal.ID.String()]
	if !ok {
		t.Fatal("personal user missing from ledger")
	}
	if got := mustDecimalForLedgerTest(t, pr.Balance); !got.Equal(decimal.NewFromInt(9)) {
		t.Errorf("personal balance = %s, want 9", pr.Balance)
	}
}
