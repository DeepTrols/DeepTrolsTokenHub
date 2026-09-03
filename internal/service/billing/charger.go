package billing

import (
	"context"
	"errors"
	"fmt"

	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Charger handles budget reservation with the commit/compensate pattern.
// All money operations go through the wallet repository which enforces
// optimistic locking and idempotency.
type Charger struct {
	wallets wallet.Repository
}

// NewCharger creates a new Charger.
func NewCharger(wallets wallet.Repository) *Charger {
	return &Charger{wallets: wallets}
}

// ReserveResult holds the outcome of a budget reservation.
type ReserveResult struct {
	TransactionID uuid.UUID
	Amount        decimal.Decimal
}

// Reserve freezes the given amount from the wallet.
// idempotencyKey prevents double-reservation on retry.
// Returns an error if available balance is insufficient.
func (c *Charger) Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*ReserveResult, error) {
	if amount.LessThanOrEqual(decimal.Zero) {
		return nil, fmt.Errorf("charger: reserve amount must be positive: %s", amount)
	}

	tx, err := c.wallets.Reserve(ctx, walletID, amount, idempotencyKey)
	if err != nil {
		// TH-P05-04: money-path observability (never affects the outcome).
		if errors.Is(err, wallet.ErrInsufficientBalance) {
			metrics.IncReserveFailed(metrics.ReasonInsufficientBalance)
		} else {
			metrics.IncReserveFailed(metrics.ReasonReserveFailed)
		}
		return nil, fmt.Errorf("charger reserve: %w", err)
	}
	metrics.IncReserve()

	return &ReserveResult{
		TransactionID: tx.ID,
		Amount:        tx.Amount,
	}, nil
}

// Commit finalizes a previously reserved transaction, deducting both balance and frozen.
func (c *Charger) Commit(ctx context.Context, txID uuid.UUID) error {
	if err := c.wallets.Commit(ctx, txID); err != nil {
		return fmt.Errorf("charger commit: %w", err)
	}
	return nil
}

// Settle finalizes a previously reserved transaction against the ACTUAL final
// cost (multi-refund settled in one transaction). Returns an error wrapping
// wallet.ErrInsufficientBalance when the wallet cannot cover a final cost
// larger than the reserved amount.
func (c *Charger) Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error {
	if err := c.wallets.Settle(ctx, txID, finalAmount); err != nil {
		// TH-P05-04: money-path observability (never affects the outcome).
		switch {
		case errors.Is(err, wallet.ErrInsufficientBalance):
			metrics.IncSettleFailed(metrics.ReasonInsufficientBalance)
		case errors.Is(err, wallet.ErrTxNotReserved):
			metrics.IncSettleFailed(metrics.ReasonTxNotReserved)
		default:
			metrics.IncSettleFailed(metrics.ReasonSettleError)
		}
		return fmt.Errorf("charger settle: %w", err)
	}
	metrics.IncSettle()
	return nil
}

// Release cancels a previously reserved transaction, returning the frozen amount
// to the available balance. Used for compensation on upstream failure.
func (c *Charger) Release(ctx context.Context, txID uuid.UUID) error {
	if err := c.wallets.Release(ctx, txID); err != nil {
		// TH-P05-04: money-path observability (never affects the outcome).
		if errors.Is(err, wallet.ErrTxNotReserved) {
			metrics.IncReleaseFailed(metrics.ReasonTxNotReserved)
		} else {
			metrics.IncReleaseFailed("other")
		}
		return fmt.Errorf("charger release: %w", err)
	}
	metrics.IncRelease()
	return nil
}
