package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Wallet struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TenantID  *uuid.UUID
	Balance   decimal.Decimal
	Frozen    decimal.Decimal
	Currency  string
	Version   int64
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (w Wallet) Available() decimal.Decimal {
	return w.Balance.Sub(w.Frozen)
}

func (w Wallet) CanReserve(amount decimal.Decimal) bool {
	return w.Available().GreaterThanOrEqual(amount)
}

type WalletTransaction struct {
	ID             uuid.UUID
	WalletID       uuid.UUID
	IdempotencyKey string
	TxType         WalletTxType
	Amount         decimal.Decimal
	BalanceBefore  decimal.Decimal
	BalanceAfter   decimal.Decimal
	ReferenceType  string
	ReferenceID    *uuid.UUID
	Metadata       map[string]any
	CreatedAt      time.Time
}

type WalletTxType string

const (
	WalletTxTopup        WalletTxType = "topup"
	WalletTxCharge       WalletTxType = "charge"
	WalletTxRefund       WalletTxType = "refund"
	WalletTxReserve      WalletTxType = "reserve"
	WalletTxRelease      WalletTxType = "release"
	WalletTxTransferIn   WalletTxType = "transfer_in"
	WalletTxTransferOut  WalletTxType = "transfer_out"
	WalletTxCompensate   WalletTxType = "compensate"
	WalletTxSubscription WalletTxType = "subscription"
)

func NewWallet(userID uuid.UUID, tenantID *uuid.UUID, currency string) Wallet {
	now := time.Now().UTC()
	return Wallet{
		ID:        uuid.New(),
		UserID:    userID,
		TenantID:  tenantID,
		Balance:   decimal.Zero,
		Frozen:    decimal.Zero,
		Currency:  currency,
		Version:   0,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
