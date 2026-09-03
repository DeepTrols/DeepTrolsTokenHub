package payment

// TH-P1-01: QueryOrder result contract tests.
//
// The contract is provider-neutral (Alipay / WeChat / temporary epay checks)
// and its state enum is CLOSED: an ambiguous or unknown provider state must
// never imply a local settlement transition.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

// Compile-time interface checks: the fake and the epay gateway must both
// satisfy Gateway once QueryOrder is part of the contract.
var (
	_ Gateway = (*fakeGateway)(nil)
	_ Gateway = (*EpayGateway)(nil)
)

func TestQueryOrderStateEnumClosed(t *testing.T) {
	valid := []QueryOrderState{QueryStatePaid, QueryStateNotPaid, QueryStateClosed, QueryStateUnknown}
	for _, s := range valid {
		if !s.Valid() {
			t.Errorf("state %q must be valid", s)
		}
	}
	for _, s := range []QueryOrderState{"", "settled", "SUCCESS", "PAID", "refunded"} {
		if s.Valid() {
			t.Errorf("state %q must be invalid (enum is closed, exact members only)", s)
		}
	}
}

// AC-01: paid carries local order number, provider trade number, amount,
// method, paid time, and retryable=false.
func TestQueryResultPaidCarriesAllFields(t *testing.T) {
	paidAt := time.Unix(1700000000, 0)
	r := &QueryOrderResult{
		OrderNo:        "DTP202609030001",
		State:          QueryStatePaid,
		GatewayTradeNo: "2026090322001400001",
		Amount:         decimal.NewFromInt(50),
		PayMethod:      "alipay",
		PaidAt:         paidAt,
		Retryable:      false,
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.OrderNo != "DTP202609030001" || r.GatewayTradeNo != "2026090322001400001" ||
		!r.Amount.Equal(decimal.NewFromInt(50)) || r.PayMethod != "alipay" ||
		!r.PaidAt.Equal(paidAt) || r.Retryable {
		t.Fatalf("paid fields not carried verbatim: %+v", r)
	}
}

// AC-02: timeout carries retryable=true WITHOUT any paid fields.
func TestQueryResultTimeoutRetryableWithoutPaidFields(t *testing.T) {
	r := &QueryOrderResult{
		OrderNo:   "DTP202609030002",
		State:     QueryStateUnknown,
		Retryable: true,
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.GatewayTradeNo != "" {
		t.Fatalf("timeout result must carry no provider trade number: %+v", r)
	}
	if !r.PaidAt.IsZero() || !r.Amount.IsZero() {
		t.Fatalf("timeout result must carry no paid time/amount: %+v", r)
	}
}

// A retryable result must never carry a definitive paid answer: mixing the
// two is the exact shape that could cause a false settlement.
func TestQueryResultRetryableRejectsPaidFields(t *testing.T) {
	r := &QueryOrderResult{
		OrderNo:        "DTP202609030003",
		State:          QueryStatePaid,
		GatewayTradeNo: "G1",
		Amount:         decimal.NewFromInt(10),
		PaidAt:         time.Unix(1700000000, 0),
		Retryable:      true,
	}
	if err := r.Validate(); err == nil {
		t.Fatal("retryable=true with paid fields must be invalid")
	}
}

// AC-03: an unknown provider state maps to `unknown` and implies no local
// transition intent (no paid fields, not marked retryable by the mapping).
func TestNormalizeUnknownProviderState(t *testing.T) {
	for _, raw := range []string{"WEIRD_STATE", "TRADE_FINISHED", "", "paid ", "CLOSED"} {
		if got := NormalizeQueryState(raw); got != QueryStateUnknown {
			t.Errorf("NormalizeQueryState(%q) = %q, want unknown", raw, got)
		}
	}
	// Recognized members map to themselves (exact, case-sensitive).
	cases := map[string]QueryOrderState{
		"paid":     QueryStatePaid,
		"not_paid": QueryStateNotPaid,
		"closed":   QueryStateClosed,
		"unknown":  QueryStateUnknown,
	}
	for raw, want := range cases {
		if got := NormalizeQueryState(raw); got != want {
			t.Errorf("NormalizeQueryState(%q) = %q, want %q", raw, got, want)
		}
	}

	r := &QueryOrderResult{OrderNo: "DTP202609030004", State: NormalizeQueryState("WEIRD_STATE")}
	if err := r.Validate(); err != nil {
		t.Fatalf("unknown-state result must validate (no transition implied): %v", err)
	}
	if r.Retryable || !r.PaidAt.IsZero() || r.GatewayTradeNo != "" {
		t.Fatalf("unknown state must imply no paid fields and no retry intent: %+v", r)
	}
}

// Failure injection: a result whose State is not a closed-enum member must
// be rejected even if it otherwise looks plausible.
func TestQueryResultRejectsUnclosedState(t *testing.T) {
	r := &QueryOrderResult{OrderNo: "DTP202609030005", State: QueryOrderState("settled")}
	if err := r.Validate(); err == nil {
		t.Fatal("non-enum state must be rejected")
	}
}

// Failure injection: malformed amounts. Paid with zero/negative amount must
// be rejected; not-paid/closed must not carry an amount.
func TestQueryResultMalformedAmount(t *testing.T) {
	base := func() *QueryOrderResult {
		return &QueryOrderResult{
			OrderNo:        "DTP202609030006",
			State:          QueryStatePaid,
			GatewayTradeNo: "G2",
			Amount:         decimal.NewFromInt(50),
			PaidAt:         time.Unix(1700000000, 0),
		}
	}

	r := base()
	r.Amount = decimal.Zero
	if err := r.Validate(); err == nil {
		t.Fatal("paid with zero amount must be rejected")
	}
	r = base()
	r.Amount = decimal.NewFromInt(-1)
	if err := r.Validate(); err == nil {
		t.Fatal("paid with negative amount must be rejected")
	}

	r = base()
	r.State = QueryStateNotPaid
	if err := r.Validate(); err == nil {
		t.Fatal("not_paid carrying an amount must be rejected")
	}
}

// Paid without a provider trade number or paid time is an incomplete
// settlement signal and must be rejected.
func TestQueryResultPaidRequiresTradeNoAndPaidAt(t *testing.T) {
	r := &QueryOrderResult{OrderNo: "DTP202609030007", State: QueryStatePaid, Amount: decimal.NewFromInt(5)}
	if err := r.Validate(); err == nil {
		t.Fatal("paid without trade number/paid time must be rejected")
	}
}

func TestQueryResultRequiresOrderNo(t *testing.T) {
	r := &QueryOrderResult{State: QueryStateNotPaid}
	if err := r.Validate(); err == nil {
		t.Fatal("missing order number must be rejected")
	}
}

// Epay placeholder: the bundled epay library exposes no order query, so the
// epay gateway reports unsupported until the compensation-worker task lands
// a real query path. The contract stays usable; behavior is explicit.
func TestEpayQueryOrderUnsupportedPlaceholder(t *testing.T) {
	g := &EpayGateway{}
	_, err := g.QueryOrder(context.Background(), "DTP202609030008")
	if !errors.Is(err, ErrQueryUnsupported) {
		t.Fatalf("QueryOrder error = %v, want ErrQueryUnsupported", err)
	}
}
