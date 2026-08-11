package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// seedPoolForTeamTest inserts a quota pool (full headroom) for a tenant.
func seedPoolForTeamTest(t *testing.T, a *app.App, tenantID uuid.UUID, total int64) uuid.UUID {
	t.Helper()
	poolID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO quota_pools (id, tenant_id, dimension, total_amount, allocated_amount, used_amount, unit_name)
		 VALUES ($1, $2, 'token', $3, 0, 0, 'token')`,
		poolID, tenantID, total,
	)
	if err != nil {
		t.Fatalf("seed quota pool: %v", err)
	}
	return poolID
}

// =============================================================================
// POST /api/console/team/members
// =============================================================================

func TestHandleCreateSubAccount_Success(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "sub-ok", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "sub-owner-ok@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"email":"alice@acme.com","display_name":"Alice","password":"secret1234","role":"member"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/members", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleCreateSubAccount(a).ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		ID    string `json:"id"`
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	subID, err := uuid.Parse(resp.ID)
	if err != nil {
		t.Fatalf("invalid sub-account id: %v", err)
	}
	if resp.Email != "alice@acme.com" || resp.Role != "member" {
		t.Errorf("resp = %+v", resp)
	}

	// User, active membership, and zero-balance wallet must all exist.
	u, err := a.Users.FindByID(context.Background(), subID)
	if err != nil {
		t.Fatalf("sub-account user missing: %v", err)
	}
	if u.UserType != domain.UserTypeEnterprise {
		t.Errorf("user_type = %q, want enterprise", u.UserType)
	}
	m, err := a.Memberships.FindByUserAndTenant(context.Background(), subID, tn.ID)
	if err != nil {
		t.Fatalf("sub-account membership missing: %v", err)
	}
	if m.Status != domain.MembershipStatusActive || m.Role != domain.MembershipRoleMember {
		t.Errorf("membership = role %s status %s", m.Role, m.Status)
	}
	var walletCount int
	_ = a.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM wallets WHERE user_id = $1`, subID).Scan(&walletCount)
	if walletCount != 1 {
		t.Errorf("wallet count = %d, want 1", walletCount)
	}
}

func TestHandleCreateSubAccount_DuplicateEmail(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "sub-dup", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "sub-owner-dup@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	existing := seedTeamUser(t, a, "already@acme.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, existing.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"email":"already@acme.com","display_name":"Dup","password":"secret1234","role":"member"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/members", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleCreateSubAccount(a).ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
}

func TestHandleCreateSubAccount_Validation(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "sub-val", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "sub-owner-val@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"short password", `{"email":"x@acme.com","display_name":"X","password":"short","role":"member"}`, http.StatusBadRequest},
		{"invalid role", `{"email":"x@acme.com","display_name":"X","password":"secret1234","role":"boss"}`, http.StatusBadRequest},
		{"bad email", `{"email":"not-an-email","display_name":"X","password":"secret1234","role":"member"}`, http.StatusBadRequest},
		{"empty name", `{"email":"x@acme.com","display_name":"","password":"secret1234","role":"member"}`, http.StatusBadRequest},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/console/team/members", bytes.NewBufferString(tc.body))
			req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
			w := httptest.NewRecorder()
			HandleCreateSubAccount(a).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestHandleCreateSubAccount_MemberForbidden(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "sub-forbid", "Acme", domain.TenantStatusActive)
	member := seedTeamUser(t, a, "sub-member-forbid@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodPost, "/api/console/team/members", bytes.NewBufferString(`{}`))
	req = setTenantCtx(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleCreateSubAccount(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

// =============================================================================
// GET /api/console/team/quotas
// =============================================================================

func TestHandleListTeamQuotas_Empty(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-empty", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-empty@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/quotas", nil)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleListTeamQuotas(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Pools []teamQuotaPool `json:"pools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Pools) != 0 {
		t.Fatalf("pools = %d, want 0", len(resp.Pools))
	}
}

func TestHandleListTeamQuotas_WithAllocation(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-list", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-list@test.com")
	sub := seedTeamUser(t, a, "q-sub-list@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	poolID := seedPoolForTeamTest(t, a, tn.ID, 100000)
	if _, err := a.Quotas.Allocate(context.Background(), poolID, sub.ID, 30000, "handler-alloc-1"); err != nil {
		t.Fatalf("seed allocation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/quotas", nil)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleListTeamQuotas(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Pools []teamQuotaPool `json:"pools"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Pools) != 1 {
		t.Fatalf("pools = %d, want 1", len(resp.Pools))
	}
	p := resp.Pools[0]
	if p.TotalAmount != 100000 || p.Allocated != 30000 || p.Remaining != 70000 {
		t.Errorf("pool = %+v, want total=100000 allocated=30000 remaining=70000", p)
	}
	if len(p.Allocations) != 1 || p.Allocations[0].UserID != sub.ID.String() {
		t.Errorf("allocations = %+v", p.Allocations)
	}
}

// =============================================================================
// POST /api/console/team/quotas/allocate
// =============================================================================

func TestHandleAllocateTeamQuota_Success(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-alloc", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-alloc@test.com")
	sub := seedTeamUser(t, a, "q-sub-alloc@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	poolID := seedPoolForTeamTest(t, a, tn.ID, 100000)

	body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","pool_id":"` + poolID.String() + `","amount":3000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/quotas/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateTeamQuota(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Allocated int64  `json:"allocated"`
		Remaining int64  `json:"remaining"`
		UserID    string `json:"user_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Allocated != 3000 || resp.Remaining != 3000 || resp.UserID != sub.ID.String() {
		t.Errorf("resp = %+v", resp)
	}
}

func TestHandleAllocateTeamQuota_ExceedsPool(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-over", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-over@test.com")
	sub := seedTeamUser(t, a, "q-sub-over@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	poolID := seedPoolForTeamTest(t, a, tn.ID, 10000)

	body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","pool_id":"` + poolID.String() + `","amount":20000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/quotas/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateTeamQuota(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}

	// The rejected allocation must not have created a ledger row.
	var ledgerCount int
	_ = a.Pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM quota_ledger`).Scan(&ledgerCount)
	if ledgerCount != 0 {
		t.Errorf("ledger rows = %d, want 0 after rejected allocation", ledgerCount)
	}
}

func TestHandleAllocateTeamQuota_NotTeamMember(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-notmem", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-notmem@test.com")
	outsider := seedTeamUser(t, a, "q-outsider@test.com") // no membership in tn
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	poolID := seedPoolForTeamTest(t, a, tn.ID, 100000)

	body := bytes.NewBufferString(`{"user_id":"` + outsider.ID.String() + `","pool_id":"` + poolID.String() + `","amount":1000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/quotas/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateTeamQuota(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleAllocateTeamQuota_OtherTenantPool(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-other", "Acme", domain.TenantStatusActive)
	otherTN := seedTenantForTest(t, a, "q-other2", "Other", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-other@test.com")
	sub := seedTeamUser(t, a, "q-sub-other@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	foreignPool := seedPoolForTeamTest(t, a, otherTN.ID, 100000)

	body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","pool_id":"` + foreignPool.String() + `","amount":1000}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/quotas/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateTeamQuota(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

// =============================================================================
// GET /api/console/team/quotas/ledger
// =============================================================================

func TestHandleTeamQuotaLedger_Success(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-ledger", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-ledger@test.com")
	sub := seedTeamUser(t, a, "q-sub-ledger@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	poolID := seedPoolForTeamTest(t, a, tn.ID, 100000)
	alloc, err := a.Quotas.Allocate(context.Background(), poolID, sub.ID, 5000, "handler-ledger-1")
	if err != nil {
		t.Fatalf("seed allocation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/quotas/ledger?allocation_id="+alloc.ID.String(), nil)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTeamQuotaLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Entries []struct {
			Action string `json:"action"`
			Amount int64  `json:"amount"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Entries) != 1 || resp.Entries[0].Action != "allocate" || resp.Entries[0].Amount != 5000 {
		t.Errorf("entries = %+v", resp.Entries)
	}
}

func TestHandleTeamQuotaLedger_ForeignAllocation(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-ledger-f", "Acme", domain.TenantStatusActive)
	otherTN := seedTenantForTest(t, a, "q-ledger-f2", "Other", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-ledgerf@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	// An allocation in another tenant's pool.
	foreignPool := seedPoolForTeamTest(t, a, otherTN.ID, 100000)
	otherUser := seedTeamUser(t, a, "q-other-user@test.com")
	foreignAlloc, err := a.Quotas.Allocate(context.Background(), foreignPool, otherUser.ID, 1000, "handler-ledger-foreign")
	if err != nil {
		t.Fatalf("seed foreign allocation: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/quotas/ledger?allocation_id="+foreignAlloc.ID.String(), nil)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTeamQuotaLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleTeamQuotaLedger_MissingParam(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "q-ledger-m", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "q-owner-ledgerm@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)

	req := httptest.NewRequest(http.MethodGet, "/api/console/team/quotas/ledger", nil)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleTeamQuotaLedger(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}
