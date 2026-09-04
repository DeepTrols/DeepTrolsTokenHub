package paymentorder

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Status values for a payment order.
const (
	StatusPending  = "pending"
	StatusPaid     = "paid"
	StatusClosed   = "closed"
	StatusRefunded = "refunded"
)

// Order is a recharge order. Crediting happens via wallet_transactions.
//
// Provider metadata (TH-P1-05, migration 000037) — all nullable so rows
// created before the migration read back as nil, never zero-valued garbage:
//
//	QueryAttempts  provider order-query attempts already made (nil = never).
//	LastQueryAt    time of the most recent provider query.
//	NextRetryAt    when the query/compensation worker may retry (nil = none).
//	ReviewReason   why the order is flagged for manual review (nil = not).
//
// Callback and reconciliation identity is carried by OrderNo, Channel,
// PayMethod, Amount and GatewayTradeNo.
type Order struct {
	ID             uuid.UUID
	OrderNo        string
	UserID         uuid.UUID
	Amount         decimal.Decimal
	Currency       string
	Purpose        string
	PlanID         *uuid.UUID
	Channel        string
	PayMethod      string
	Status         string
	GatewayTradeNo *string
	PayURL         *string
	NotifyRaw      []byte
	PaidAt         *time.Time
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time

	QueryAttempts *int
	LastQueryAt   *time.Time
	NextRetryAt   *time.Time
	ReviewReason  *string
}

// Repository defines payment_orders data access.
type Repository interface {
	Create(ctx context.Context, o *Order) error
	FindByOrderNo(ctx context.Context, orderNo string) (*Order, error)
	FindByID(ctx context.Context, id uuid.UUID) (*Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Order, error)
	List(ctx context.Context, limit, offset int, status *string, userID *uuid.UUID) ([]Order, error)
	// MarkPaid atomically transitions pending→paid. Returns true when the
	// transition was applied; false when the row was already paid or closed.
	MarkPaid(ctx context.Context, id uuid.UUID, gatewayTradeNo string, notifyRaw []byte) (bool, error)
	// MarkClosed atomically transitions pending→closed (expired/cancelled).
	MarkClosed(ctx context.Context, id uuid.UUID) error
	// RecordProviderQuery records one provider query attempt (TH-P1-05):
	// increments query_attempts, stamps last_query_at and schedules the next
	// retry (nil clears the schedule). Never changes status or amount.
	// Returns ErrNotFound when the order does not exist.
	RecordProviderQuery(ctx context.Context, id uuid.UUID, nextRetryAt *time.Time) error
	// SetReviewReason flags an order for manual review (TH-P1-05); an empty
	// reason clears the flag. Never changes status or amount. Returns
	// ErrNotFound when the order does not exist.
	SetReviewReason(ctx context.Context, id uuid.UUID, reason string) error
	// ListPendingCandidates returns pending orders old enough for the
	// scanner (created_at <= olderThan), oldest first with order_no as the
	// deterministic tiebreak, capped at limit (TH-P1-CW-01). A non-nil
	// channel restricts candidates to that channel. The query is strictly
	// read-only: it never mutates status, metadata or timestamps.
	ListPendingCandidates(ctx context.Context, olderThan time.Time, limit int, channel *string) ([]Order, error)
}
