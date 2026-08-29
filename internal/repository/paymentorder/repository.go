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
}
