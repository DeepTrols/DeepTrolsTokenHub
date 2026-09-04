package payment

// TH-P1-02: settlement intent table tests. The intent service converts an
// actively queried provider result into a local settlement intent WITHOUT
// mutating the wallet or the order; the fake wallet call counter must stay
// at zero on every path (the caller executes settlement, never the intent).

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func paidQueryResult(orderNo string, amount decimal.Decimal) *QueryOrderResult {
	return &QueryOrderResult{
		OrderNo:        orderNo,
		State:          QueryStatePaid,
		GatewayTradeNo: "GQ1",
		Amount:         amount,
		PaidAt:         time.Unix(1700000100, 0),
	}
}

func seedPendingOrder(orders *fakeOrders, orderNo string, amount decimal.Decimal) *paymentorder.Order {
	o := &paymentorder.Order{
		ID: uuid.New(), OrderNo: orderNo, UserID: uuid.New(),
		Amount: amount, Currency: "CNY", Channel: "epay", PayMethod: "alipay",
		Status: paymentorder.StatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
	orders.byNo[orderNo] = o
	orders.byID[o.ID] = o
	return o
}

// AC-01: query paid + matching amount -> mark_paid carrying provider fields.
func TestIntentPaidMatchingAmountMarkPaid(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	seedPendingOrder(orders, "DTPQ1", decimal.NewFromInt(50))

	intent, err := s.QuerySettlementIntent(context.Background(), paidQueryResult("DTPQ1", decimal.NewFromInt(50)))
	if err != nil {
		t.Fatalf("QuerySettlementIntent: %v", err)
	}
	if intent.Kind != IntentMarkPaid {
		t.Fatalf("intent = %q, want mark_paid", intent.Kind)
	}
	if intent.GatewayTradeNo != "GQ1" || !intent.Amount.Equal(decimal.NewFromInt(50)) {
		t.Fatalf("mark_paid intent missing provider fields: %+v", intent)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("intent service must never call the wallet, got %d calls", wallets.topupCount)
	}
}

// AC-02: local order already paid -> already_settled, no wallet call.
func TestIntentAlreadyPaidNoWalletCall(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	o := seedPendingOrder(orders, "DTPQ2", decimal.NewFromInt(50))
	o.Status = paymentorder.StatusPaid

	intent, err := s.QuerySettlementIntent(context.Background(), paidQueryResult("DTPQ2", decimal.NewFromInt(50)))
	if err != nil || intent.Kind != IntentAlreadySettled {
		t.Fatalf("intent = %+v err = %v, want already_settled", intent, err)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("already_settled must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// AC-03: query amount differs from the local amount -> amount_mismatch,
// local order unchanged.
func TestIntentAmountMismatchLeavesOrder(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	o := seedPendingOrder(orders, "DTPQ3", decimal.NewFromInt(50))

	intent, err := s.QuerySettlementIntent(context.Background(), paidQueryResult("DTPQ3", decimal.NewFromInt(99)))
	if err != nil || intent.Kind != IntentAmountMismatch {
		t.Fatalf("intent = %+v err = %v, want amount_mismatch", intent, err)
	}
	if o.Status != paymentorder.StatusPending {
		t.Fatalf("order must stay pending after mismatch, got %s", o.Status)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("amount_mismatch must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// AC-04: provider timeout (retryable result) -> retryable, order unchanged.
func TestIntentTimeoutRetryable(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	o := seedPendingOrder(orders, "DTPQ4", decimal.NewFromInt(50))

	res := &QueryOrderResult{OrderNo: "DTPQ4", State: QueryStateUnknown, Retryable: true}
	intent, err := s.QuerySettlementIntent(context.Background(), res)
	if err != nil || intent.Kind != IntentRetryable {
		t.Fatalf("intent = %+v err = %v, want retryable", intent, err)
	}
	if o.Status != paymentorder.StatusPending {
		t.Fatalf("order must stay pending on timeout, got %s", o.Status)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("retryable must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// Intent table for the remaining provider states: not_paid and unknown never
// imply a transition; provider closed maps to a close intent; a paid answer
// for a locally closed order goes to manual review (no auto settlement of
// non-pending orders).
func TestIntentTable(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	seedPendingOrder(orders, "DTPQ5", decimal.NewFromInt(50))
	closed := seedPendingOrder(orders, "DTPQ6", decimal.NewFromInt(50))
	closed.Status = paymentorder.StatusClosed

	cases := []struct {
		name string
		res  *QueryOrderResult
		want SettlementIntentKind
	}{
		{"not_paid", &QueryOrderResult{OrderNo: "DTPQ5", State: QueryStateNotPaid}, IntentNoAction},
		{"provider_closed", &QueryOrderResult{OrderNo: "DTPQ5", State: QueryStateClosed}, IntentMarkClosed},
		{"unknown", &QueryOrderResult{OrderNo: "DTPQ5", State: QueryStateUnknown}, IntentNoAction},
		{"paid_but_locally_closed", paidQueryResult("DTPQ6", decimal.NewFromInt(50)), IntentNoAction},
	}
	for _, tc := range cases {
		intent, err := s.QuerySettlementIntent(context.Background(), tc.res)
		if err != nil || intent.Kind != tc.want {
			t.Fatalf("%s: intent = %+v err = %v, want %s", tc.name, intent, err, tc.want)
		}
	}
	if wallets.topupCount != 0 {
		t.Fatalf("no intent path may call the wallet, got %d calls", wallets.topupCount)
	}
}

// Failure injection: missing order yields the repository not-found error and
// never reaches the wallet.
func TestIntentMissingOrder(t *testing.T) {
	s, _, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	_, err := s.QuerySettlementIntent(context.Background(), paidQueryResult("DTPGONE", decimal.NewFromInt(50)))
	if !errors.Is(err, paymentorder.ErrNotFound) {
		t.Fatalf("error = %v, want paymentorder.ErrNotFound", err)
	}
	if wallets.topupCount != 0 {
		t.Fatalf("missing order must not call the wallet, got %d calls", wallets.topupCount)
	}
}

// Failure injection: a query result violating the TH-P1-01 contract is
// rejected fail-closed (no intent).
func TestIntentInvalidResultRejected(t *testing.T) {
	s, orders, wallets := newTestService(enabledSettings(), &fakeGateway{payURL: "u"})
	seedPendingOrder(orders, "DTPQ7", decimal.NewFromInt(50))
	// paid without provider trade number violates the contract.
	bad := &QueryOrderResult{OrderNo: "DTPQ7", State: QueryStatePaid, Amount: decimal.NewFromInt(50), PaidAt: time.Unix(1700000100, 0)}
	if _, err := s.QuerySettlementIntent(context.Background(), bad); err == nil {
		t.Fatal("expected error for contract-violating query result")
	}
	if wallets.topupCount != 0 {
		t.Fatalf("invalid result must not call the wallet, got %d calls", wallets.topupCount)
	}
}
