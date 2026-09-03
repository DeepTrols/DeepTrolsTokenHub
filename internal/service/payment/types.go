package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Gateway abstracts a payment channel (epay first, official adapters later).
type Gateway interface {
	Name() string
	CreateOrder(ctx context.Context, req CreateOrderRequest) (*CreateOrderResult, error)
	VerifyNotify(ctx context.Context, params map[string]string) (*NotifyResult, error)
	// QueryOrder actively queries the provider for the current state of a
	// local order (compensation for lost callbacks). Implementations return
	// a provider-neutral QueryOrderResult; gateways without a query path
	// return ErrQueryUnsupported.
	QueryOrder(ctx context.Context, orderNo string) (*QueryOrderResult, error)
}

// QueryOrderState is the closed, provider-neutral state of an actively
// queried order. The enum is deliberately closed and exact: ambiguous state
// names can cause false settlement, so unrecognized provider states map to
// QueryStateUnknown and imply NO local transition.
type QueryOrderState string

const (
	// QueryStatePaid: the provider reports the order paid.
	QueryStatePaid QueryOrderState = "paid"
	// QueryStateNotPaid: the provider reports the order created, unpaid.
	QueryStateNotPaid QueryOrderState = "not_paid"
	// QueryStateClosed: the provider reports the order closed/cancelled.
	QueryStateClosed QueryOrderState = "closed"
	// QueryStateUnknown: no definitive answer (unrecognized provider state
	// or a failed query). Never implies a local settlement transition.
	QueryStateUnknown QueryOrderState = "unknown"
)

// Valid reports whether the state is a member of the closed enum.
func (s QueryOrderState) Valid() bool {
	switch s {
	case QueryStatePaid, QueryStateNotPaid, QueryStateClosed, QueryStateUnknown:
		return true
	}
	return false
}

// NormalizeQueryState maps a raw provider state string onto the closed enum
// (exact, case-sensitive). Anything unrecognized maps to QueryStateUnknown —
// never to paid — so no local transition intent is implied.
func NormalizeQueryState(raw string) QueryOrderState {
	s := QueryOrderState(raw)
	switch s {
	case QueryStatePaid, QueryStateNotPaid, QueryStateClosed, QueryStateUnknown:
		return s
	}
	return QueryStateUnknown
}

// QueryOrderResult is the provider-neutral result of an active order query
// (usable by Alipay, WeChat, and temporary epay checks).
//
// Retryable distinguishes "the query itself failed transiently" (timeout;
// safe to retry, carries no paid fields) from a definitive provider answer.
type QueryOrderResult struct {
	// OrderNo is the local order number that was queried.
	OrderNo string
	// State is the normalized provider state (closed enum).
	State QueryOrderState
	// GatewayTradeNo is the provider trade number; set only when paid.
	GatewayTradeNo string
	// Amount is the provider-reported amount; zero when unavailable.
	Amount decimal.Decimal
	// PayMethod is the provider-reported method; may be empty.
	PayMethod string
	// PaidAt is the provider-reported payment time; zero unless paid.
	PaidAt time.Time
	// Retryable is true when the query attempt produced no definitive
	// answer (e.g. provider timeout) and a retry is expected.
	Retryable bool
}

// Validate enforces the contract invariants that protect against false
// settlement:
//   - order number present; state is a closed-enum member;
//   - paid requires provider trade number, positive amount and paid time,
//     and must not be retryable;
//   - retryable means no paid fields at all (state unknown);
//   - not_paid/closed carry no paid fields.
func (r *QueryOrderResult) Validate() error {
	if r == nil {
		return errors.New("payment: nil query result")
	}
	if r.OrderNo == "" {
		return errors.New("payment: query result missing order number")
	}
	if !r.State.Valid() {
		return fmt.Errorf("payment: query state %q is not a closed-enum member", string(r.State))
	}
	hasPaidFields := r.GatewayTradeNo != "" || !r.PaidAt.IsZero() || !r.Amount.IsZero()
	if r.Retryable {
		if r.State != QueryStateUnknown || hasPaidFields {
			return errors.New("payment: retryable query result must carry state unknown and no paid fields")
		}
		return nil
	}
	switch r.State {
	case QueryStatePaid:
		if r.GatewayTradeNo == "" {
			return errors.New("payment: paid query result missing provider trade number")
		}
		if r.PaidAt.IsZero() {
			return errors.New("payment: paid query result missing paid time")
		}
		if !r.Amount.IsPositive() {
			return fmt.Errorf("payment: paid query result has malformed amount %s", r.Amount.String())
		}
	case QueryStateNotPaid, QueryStateClosed, QueryStateUnknown:
		if hasPaidFields {
			return fmt.Errorf("payment: state %s must not carry paid fields", string(r.State))
		}
	}
	return nil
}

// CreateOrderRequest is the channel-agnostic order creation request.
type CreateOrderRequest struct {
	OrderNo   string
	Amount    decimal.Decimal
	PayMethod string // alipay / wxpay
	Subject   string
	NotifyURL string
	ReturnURL string
}

// CreateOrderResult carries the payment URL the client should open/scan.
type CreateOrderResult struct {
	PayURL string
}

// NotifyResult is the verified, normalized gateway callback payload.
type NotifyResult struct {
	OrderNo        string
	GatewayTradeNo string
	Amount         decimal.Decimal
	PayMethod      string
	Success        bool
}

var (
	ErrPaymentDisabled = errors.New("payment: online topup disabled")
	ErrNotConfigured   = errors.New("payment: gateway not configured")
	ErrInvalidMethod   = errors.New("payment: unsupported pay method")
	ErrAmountRange     = errors.New("payment: amount out of range")
	ErrOrderNotFound   = errors.New("payment: order not found")
	ErrAmountMismatch  = errors.New("payment: notify amount mismatch")
	ErrOrderExpired    = errors.New("payment: order expired")
	// ErrQueryUnsupported marks gateways with no active query path yet.
	ErrQueryUnsupported = errors.New("payment: gateway does not support order query")
)
