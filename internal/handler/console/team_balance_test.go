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
	"github.com/shopspring/decimal"
)

// seedWalletBalanceForTeamTest inserts a personal wallet (tenant_id NULL) with
// the given balance, matching what auth/sub-account provisioning creates.
func seedWalletBalanceForTeamTest(t *testing.T, a *app.App, userID uuid.UUID, balance string) {
	t.Helper()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), userID, balance,
	)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
}

// walletBalanceForTeamTest reads a user's personal wallet balance.
func walletBalanceForTeamTest(t *testing.T, a *app.App, userID uuid.UUID) decimal.Decimal {
	t.Helper()
	w, err := a.Wallets.FindByUser(context.Background(), userID, nil)
	if err != nil {
		t.Fatalf("wallet lookup: %v", err)
	}
	return w.Balance
}

// =============================================================================
// POST /api/console/team/balance/allocate
// =============================================================================

func TestHandleAllocateBalance_Success(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-ok", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-ok@test.com")
	sub := seedTeamUser(t, a, "bal-sub-ok@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")
	seedWalletBalanceForTeamTest(t, a, sub.ID, "0")

	body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		TransactionID string `json:"transaction_id"`
		FromBalance   string `json:"from_balance"`
		ToBalance     string `json:"to_balance"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TransactionID == "" {
		t.Error("transaction_id missing")
	}
	if !decimal.RequireFromString(resp.FromBalance).Equal(decimal.RequireFromString("90")) {
		t.Errorf("from_balance = %s, want 90", resp.FromBalance)
	}
	if !decimal.RequireFromString(resp.ToBalance).Equal(decimal.RequireFromString("10")) {
		t.Errorf("to_balance = %s, want 10", resp.ToBalance)
	}

	// Both wallets moved atomically.
	if !walletBalanceForTeamTest(t, a, owner.ID).Equal(decimal.RequireFromString("90")) {
		t.Error("owner balance != 90 after transfer")
	}
	if !walletBalanceForTeamTest(t, a, sub.ID).Equal(decimal.RequireFromString("10")) {
		t.Error("member balance != 10 after transfer")
	}
}

// Regression for the security-reviewer HIGH: the idempotency key must include
// the amount, otherwise a second allocation to the same member with a
// *different* amount silently replays the first and moves no money.
func TestHandleAllocateBalance_DifferentAmountsToSameMember(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-multi", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-multi@test.com")
	sub := seedTeamUser(t, a, "bal-sub-multi@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")
	seedWalletBalanceForTeamTest(t, a, sub.ID, "0")

	allocate := func(amount string) string {
		t.Helper()
		body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","amount":"` + amount + `"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
		req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
		w := httptest.NewRecorder()
		HandleAllocateBalance(a).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("allocate %s: status = %d, want %d, body: %s", amount, w.Code, http.StatusOK, w.Body.String())
		}
		var resp struct {
			TransactionID string `json:"transaction_id"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		return resp.TransactionID
	}

	first := allocate("10.00")
	second := allocate("20.00")
	if first == "" || second == "" {
		t.Fatal("expected transaction ids")
	}
	// Different amounts → distinct transfers, never a replay.
	if first == second {
		t.Errorf("two different amounts resolved to the same transaction %q — H1 regression", first)
	}
	if !walletBalanceForTeamTest(t, a, owner.ID).Equal(decimal.RequireFromString("70")) {
		t.Errorf("owner balance = %s, want 70 after 10 + 20", walletBalanceForTeamTest(t, a, owner.ID))
	}
	if !walletBalanceForTeamTest(t, a, sub.ID).Equal(decimal.RequireFromString("30")) {
		t.Errorf("member balance = %s, want 30 after 10 + 20", walletBalanceForTeamTest(t, a, sub.ID))
	}

	// A retried identical amount is a genuine replay: no double debit.
	replay := allocate("10.00")
	if replay != first {
		t.Errorf("same-amount replay produced a new transaction %q, want %q (must dedup)", replay, first)
	}
	if !walletBalanceForTeamTest(t, a, owner.ID).Equal(decimal.RequireFromString("70")) {
		t.Error("owner balance changed on idempotent replay")
	}
	if !walletBalanceForTeamTest(t, a, sub.ID).Equal(decimal.RequireFromString("30")) {
		t.Error("member balance changed on idempotent replay")
	}
}

func TestHandleAllocateBalance_MemberForbidden(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-forbid", "Acme", domain.TenantStatusActive)
	member := seedTeamUser(t, a, "bal-member-forbid@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, member.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)

	body := bytes.NewBufferString(`{"user_id":"` + member.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, member.ID.String(), tn.ID.String(), "member")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleAllocateBalance_CrossTenant(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-cross", "Acme", domain.TenantStatusActive)
	otherTN := seedTenantForTest(t, a, "bal-cross2", "Other", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-cross@test.com")
	outsider := seedTeamUser(t, a, "bal-outsider@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, otherTN.ID, outsider.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")
	seedWalletBalanceForTeamTest(t, a, outsider.ID, "0")

	body := bytes.NewBufferString(`{"user_id":"` + outsider.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	// No balance moved.
	if !walletBalanceForTeamTest(t, a, owner.ID).Equal(decimal.RequireFromString("100")) {
		t.Error("owner balance must be unchanged on cross-tenant attempt")
	}
}

func TestHandleAllocateBalance_NotAMember(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-nomem", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-nomem@test.com")
	outsider := seedTeamUser(t, a, "bal-outsider-nomem@test.com") // no membership at all
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")

	body := bytes.NewBufferString(`{"user_id":"` + outsider.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleAllocateBalance_InvalidRequest(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-invalid", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-invalid@test.com")
	sub := seedTeamUser(t, a, "bal-sub-invalid@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")
	seedWalletBalanceForTeamTest(t, a, sub.ID, "0")

	cases := []struct {
		name string
		body string
		want int
	}{
		{"zero amount", `{"user_id":"` + sub.ID.String() + `","amount":"0"}`, http.StatusBadRequest},
		{"negative amount", `{"user_id":"` + sub.ID.String() + `","amount":"-5"}`, http.StatusBadRequest},
		{"non-numeric amount", `{"user_id":"` + sub.ID.String() + `","amount":"abc"}`, http.StatusBadRequest},
		{"missing user", `{"amount":"10"}`, http.StatusBadRequest},
		{"malformed body", `{`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", bytes.NewBufferString(tc.body))
			req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
			w := httptest.NewRecorder()
			HandleAllocateBalance(a).ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestHandleAllocateBalance_InsufficientBalance(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-insuf", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-insuf@test.com")
	sub := seedTeamUser(t, a, "bal-sub-insuf@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	_ = seedMembershipForAdminTest(t, a, tn.ID, sub.ID, domain.MembershipRoleMember, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "5.00")
	seedWalletBalanceForTeamTest(t, a, sub.ID, "0")

	body := bytes.NewBufferString(`{"user_id":"` + sub.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	if !walletBalanceForTeamTest(t, a, owner.ID).Equal(decimal.RequireFromString("5")) {
		t.Error("owner balance must be unchanged on insufficient balance")
	}
}

func TestHandleAllocateBalance_SelfTransfer(t *testing.T) {
	a := appForTeamTest(t)
	tn := seedTenantForTest(t, a, "bal-self", "Acme", domain.TenantStatusActive)
	owner := seedTeamUser(t, a, "bal-owner-self@test.com")
	_ = seedMembershipForAdminTest(t, a, tn.ID, owner.ID, domain.MembershipRoleOwner, domain.MembershipStatusActive)
	seedWalletBalanceForTeamTest(t, a, owner.ID, "100.00")

	body := bytes.NewBufferString(`{"user_id":"` + owner.ID.String() + `","amount":"10.00"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/console/team/balance/allocate", body)
	req = setTenantCtx(req, owner.ID.String(), tn.ID.String(), "owner")
	w := httptest.NewRecorder()

	HandleAllocateBalance(a).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}
