package payment

import (
	"context"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/shopspring/decimal"
)

// SettlementIntentKind is the closed set of settlement intents produced by
// the active query flow (TH-P1-02). The service converts a provider query
// result into one of these intents WITHOUT mutating the wallet or the
// order; the caller (compensation worker / manual admin path) decides
// whether and how to execute it.
type SettlementIntentKind string

const (
	// IntentMarkPaid: provider reports paid with a matching amount for a
	// pending local order; the caller may settle.
	IntentMarkPaid SettlementIntentKind = "mark_paid"
	// IntentAlreadySettled: the local order is already paid; nothing to do.
	IntentAlreadySettled SettlementIntentKind = "already_settled"
	// IntentAmountMismatch: provider amount differs from the local amount;
	// the local order is left unchanged for review.
	IntentAmountMismatch SettlementIntentKind = "amount_mismatch"
	// IntentRetryable: the query itself failed transiently (e.g. provider
	// timeout); retry later, leave the local order unchanged.
	IntentRetryable SettlementIntentKind = "retryable"
	// IntentNoAction: no transition is implied (provider answered not_paid
	// or unknown, or the order is not in a settleable state).
	IntentNoAction SettlementIntentKind = "no_action"
	// IntentMarkClosed: the provider reports the order closed/cancelled;
	// the caller may close the local order.
	IntentMarkClosed SettlementIntentKind = "mark_closed"
)

// SettlementIntent is the settlement decision for one actively queried
// order. Paid fields are carried only on IntentMarkPaid.
type SettlementIntent struct {
	Kind           SettlementIntentKind
	OrderNo        string
	GatewayTradeNo string          // mark_paid only
	Amount         decimal.Decimal // mark_paid only: provider-reported amount
}

// Intent table (TH-P1-02 documentation requirement):
//
//	paid   + local pending + amount match   -> mark_paid
//	paid   + local pending + amount differs -> amount_mismatch (order unchanged)
//	paid   + local already paid             -> already_settled (no wallet call)
//	paid   + local closed                   -> no_action (manual review; non-pending
//	                                           orders are never auto-settled, mirroring
//	                                           the callback path's pending-only guard)
//	retryable query (timeout)               -> retryable (order unchanged)
//	not_paid                                -> no_action
//	closed                                  -> mark_closed
//	unknown                                 -> no_action (never implies a transition)
//	invalid query result (TH-P1-01 breach)  -> error, fail closed

// QuerySettlementIntent converts an actively queried provider result into a
// local settlement intent. It mirrors the callback path's safety checks —
// the TH-P1-01 contract must hold, the order must exist, only pending
// orders may settle, and the amounts must match — but it never touches the
// wallet or mutates the order: the caller executes the intent.
func (s *Service) QuerySettlementIntent(ctx context.Context, res *QueryOrderResult) (SettlementIntent, error) {
	if err := res.Validate(); err != nil {
		return SettlementIntent{}, err
	}
	if res.Retryable {
		// No definitive answer: no local transition, retry later.
		return SettlementIntent{Kind: IntentRetryable, OrderNo: res.OrderNo}, nil
	}
	order, err := s.orders.FindByOrderNo(ctx, res.OrderNo)
	if err != nil {
		return SettlementIntent{}, err
	}
	if order.Status == paymentorder.StatusPaid {
		return SettlementIntent{Kind: IntentAlreadySettled, OrderNo: order.OrderNo}, nil
	}
	switch res.State {
	case QueryStatePaid:
		if order.Status != paymentorder.StatusPending {
			// Same pending-only guard as the callback path: a paid provider
			// answer for a locally closed order goes to manual review.
			return SettlementIntent{Kind: IntentNoAction, OrderNo: order.OrderNo}, nil
		}
		if !res.Amount.Equal(order.Amount) {
			return SettlementIntent{Kind: IntentAmountMismatch, OrderNo: order.OrderNo}, nil
		}
		return SettlementIntent{
			Kind:           IntentMarkPaid,
			OrderNo:        order.OrderNo,
			GatewayTradeNo: res.GatewayTradeNo,
			Amount:         res.Amount,
		}, nil
	case QueryStateClosed:
		return SettlementIntent{Kind: IntentMarkClosed, OrderNo: order.OrderNo}, nil
	default:
		// not_paid and unknown never imply a local transition.
		return SettlementIntent{Kind: IntentNoAction, OrderNo: order.OrderNo}, nil
	}
}
