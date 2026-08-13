package wallet

import (
	"context"
	"errors"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ErrNotFound is returned when a wallet lookup finds no matching row.
var ErrNotFound = errors.New("wallet: not found")

// ErrInsufficientBalance is returned when a settle requires more available
// balance than the wallet holds.
var ErrInsufficientBalance = errors.New("wallet: insufficient available balance")

type Repository interface {
	FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error)
	FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error)
	Create(ctx context.Context, wallet *domain.Wallet) error
	Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
	Commit(ctx context.Context, txID uuid.UUID) error
	// Settle finalizes a reserve with the ACTUAL final cost, charging the
	// difference (or refunding the excess) against the wallet in one
	// transaction. Returns ErrInsufficientBalance when the wallet lacks the
	// available balance to cover a larger-than-reserved final cost.
	Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error
	Release(ctx context.Context, txID uuid.UUID) error
	TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
	// Transfer atomically moves `amount` from the source wallet to the
	// destination wallet in one transaction: a negative transfer_out on the
	// source and a positive transfer_in on the destination. Returns the
	// transfer_out transaction (or the existing record on idempotent replay).
	// Returns ErrInsufficientBalance when the source lacks available balance
	// and ErrNotFound when either wallet is missing.
	Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
	ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]domain.WalletTransaction, error)
}
