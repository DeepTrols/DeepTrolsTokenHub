package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestNewCharger(t *testing.T) {
	repo := &mockWalletRepo{}
	c := NewCharger(repo)
	if c == nil {
		t.Fatal("NewCharger returned nil")
	}
	if c.wallets != repo {
		t.Error("Charger did not store the injected wallet repository")
	}
}

func TestCharger_Reserve_Success(t *testing.T) {
	walletID := uuid.New()
	amount := decimal.NewFromFloat(10.0)
	idemKey := "idem-001"
	expectedTx := &domain.WalletTransaction{
		ID:             uuid.New(),
		WalletID:       walletID,
		IdempotencyKey: idemKey,
		TxType:         domain.WalletTxReserve,
		Amount:         amount,
	}

	repo := &mockWalletRepo{
		reserveFn: func(ctx context.Context, wID uuid.UUID, amt decimal.Decimal, key string) (*domain.WalletTransaction, error) {
			if wID != walletID {
				t.Errorf("Reserve walletID = %s, want %s", wID, walletID)
			}
			if !amt.Equal(amount) {
				t.Errorf("Reserve amount = %s, want %s", amt, amount)
			}
			if key != idemKey {
				t.Errorf("Reserve idempotencyKey = %s, want %s", key, idemKey)
			}
			return expectedTx, nil
		},
	}

	c := NewCharger(repo)
	result, err := c.Reserve(context.Background(), walletID, amount, idemKey)

	if err != nil {
		t.Fatalf("Reserve unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("Reserve returned nil result")
	}
	if result.TransactionID != expectedTx.ID {
		t.Errorf("TransactionID = %s, want %s", result.TransactionID, expectedTx.ID)
	}
	if !result.Amount.Equal(amount) {
		t.Errorf("Amount = %s, want %s", result.Amount, amount)
	}
}

func TestCharger_Reserve_ZeroAmount(t *testing.T) {
	c := NewCharger(&mockWalletRepo{})
	_, err := c.Reserve(context.Background(), uuid.New(), decimal.Zero, "key")
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
	if err.Error() != "charger: reserve amount must be positive: 0" {
		t.Errorf("error message = %q, want %q", err.Error(), "charger: reserve amount must be positive: 0")
	}
}

func TestCharger_Reserve_NegativeAmount(t *testing.T) {
	c := NewCharger(&mockWalletRepo{})
	amount := decimal.NewFromFloat(-5.0)
	_, err := c.Reserve(context.Background(), uuid.New(), amount, "key")
	if err == nil {
		t.Fatal("expected error for negative amount")
	}
}

func TestCharger_Reserve_RepoError(t *testing.T) {
	repo := &mockWalletRepo{
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return nil, errors.New("insufficient balance")
		},
	}
	c := NewCharger(repo)
	_, err := c.Reserve(context.Background(), uuid.New(), decimal.NewFromFloat(10.0), "key")
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestCharger_Reserve_Idempotent(t *testing.T) {
	walletID := uuid.New()
	amount := decimal.NewFromFloat(10.0)
	idemKey := "idem-dup"
	expectedTx := &domain.WalletTransaction{
		ID:             uuid.New(),
		WalletID:       walletID,
		IdempotencyKey: idemKey,
		TxType:         domain.WalletTxReserve,
		Amount:         amount,
	}

	callCount := 0
	repo := &mockWalletRepo{
		reserveFn: func(ctx context.Context, wID uuid.UUID, amt decimal.Decimal, key string) (*domain.WalletTransaction, error) {
			callCount++
			return expectedTx, nil
		},
	}

	c := NewCharger(repo)

	// First call
	r1, err1 := c.Reserve(context.Background(), walletID, amount, idemKey)
	if err1 != nil {
		t.Fatalf("first Reserve: %v", err1)
	}

	// Second call with same idempotency key
	r2, err2 := c.Reserve(context.Background(), walletID, amount, idemKey)
	if err2 != nil {
		t.Fatalf("second Reserve: %v", err2)
	}

	if r1.TransactionID != r2.TransactionID {
		t.Error("idempotent retry returned different transaction ID")
	}
	if callCount != 2 {
		t.Logf("repo Reserve called %d times (idempotency handled at repo level)", callCount)
	}
}

func TestCharger_Commit_Success(t *testing.T) {
	txID := uuid.New()
	repo := &mockWalletRepo{
		commitFn: func(ctx context.Context, id uuid.UUID) error {
			if id != txID {
				t.Errorf("Commit txID = %s, want %s", id, txID)
			}
			return nil
		},
	}
	c := NewCharger(repo)
	if err := c.Commit(context.Background(), txID); err != nil {
		t.Fatalf("Commit unexpected error: %v", err)
	}
}

func TestCharger_Commit_RepoError(t *testing.T) {
	repo := &mockWalletRepo{
		commitFn: func(ctx context.Context, txID uuid.UUID) error {
			return errors.New("transaction not found")
		},
	}
	c := NewCharger(repo)
	err := c.Commit(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestCharger_Release_Success(t *testing.T) {
	txID := uuid.New()
	repo := &mockWalletRepo{
		releaseFn: func(ctx context.Context, id uuid.UUID) error {
			if id != txID {
				t.Errorf("Release txID = %s, want %s", id, txID)
			}
			return nil
		},
	}
	c := NewCharger(repo)
	if err := c.Release(context.Background(), txID); err != nil {
		t.Fatalf("Release unexpected error: %v", err)
	}
}

func TestCharger_Release_RepoError(t *testing.T) {
	repo := &mockWalletRepo{
		releaseFn: func(ctx context.Context, txID uuid.UUID) error {
			return errors.New("transaction not in reserve state")
		},
	}
	c := NewCharger(repo)
	err := c.Release(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

// TestCharger_ReserveCommitFlow verifies the full happy path: reserve → commit
func TestCharger_ReserveCommitFlow(t *testing.T) {
	walletID := uuid.New()
	amount := decimal.NewFromFloat(25.0)
	txID := uuid.New()
	idemKey := "flow-001"

	repo := &mockWalletRepo{
		reserveFn: func(ctx context.Context, wID uuid.UUID, amt decimal.Decimal, key string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{
				ID:             txID,
				WalletID:       wID,
				IdempotencyKey: key,
				TxType:         domain.WalletTxReserve,
				Amount:         amt,
			}, nil
		},
		commitFn: func(ctx context.Context, id uuid.UUID) error {
			if id != txID {
				t.Errorf("commit txID = %s, want %s", id, txID)
			}
			return nil
		},
	}

	c := NewCharger(repo)

	// Step 1: Reserve
	result, err := c.Reserve(context.Background(), walletID, amount, idemKey)
	if err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}
	if result.TransactionID != txID {
		t.Errorf("TransactionID = %s, want %s", result.TransactionID, txID)
	}

	// Step 2: Commit
	if err := c.Commit(context.Background(), result.TransactionID); err != nil {
		t.Fatalf("Commit failed: %v", err)
	}
}

// TestCharger_ReserveReleaseFlow verifies the compensation path: reserve → release
func TestCharger_ReserveReleaseFlow(t *testing.T) {
	walletID := uuid.New()
	amount := decimal.NewFromFloat(25.0)
	txID := uuid.New()
	idemKey := "flow-002"

	repo := &mockWalletRepo{
		reserveFn: func(ctx context.Context, wID uuid.UUID, amt decimal.Decimal, key string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{
				ID:             txID,
				WalletID:       wID,
				IdempotencyKey: key,
				TxType:         domain.WalletTxReserve,
				Amount:         amt,
			}, nil
		},
		releaseFn: func(ctx context.Context, id uuid.UUID) error {
			if id != txID {
				t.Errorf("release txID = %s, want %s", id, txID)
			}
			return nil
		},
	}

	c := NewCharger(repo)

	// Step 1: Reserve
	result, err := c.Reserve(context.Background(), walletID, amount, idemKey)
	if err != nil {
		t.Fatalf("Reserve failed: %v", err)
	}

	// Step 2: Release (compensate on upstream failure)
	if err := c.Release(context.Background(), result.TransactionID); err != nil {
		t.Fatalf("Release failed: %v", err)
	}
}
