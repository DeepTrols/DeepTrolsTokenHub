package wallet

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func seedWalletUser(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@test.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

func seedWalletTenant(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	code := "wallet-" + uuid.New().String()[:8]
	_, err := repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, code, code+" name")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenantID
}

func TestWalletCreateAndFind(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	userID := seedWalletUser(t, ctx, repo)
	tenantID := seedWalletTenant(t, ctx, repo)

	wallet := domain.NewWallet(userID, &tenantID, "CNY")

	t.Run("creates wallet", func(t *testing.T) {
		if err := repo.Create(ctx, &wallet); err != nil {
			t.Fatalf("Create: %v", err)
		}
	})

	t.Run("finds wallet by user", func(t *testing.T) {
		found, err := repo.FindByUser(ctx, userID, &tenantID)
		if err != nil {
			t.Fatalf("FindByUser: %v", err)
		}
		if found.ID != wallet.ID {
			t.Errorf("ID = %s, want %s", found.ID, wallet.ID)
		}
		if !found.Balance.Equal(decimal.Zero) {
			t.Errorf("Balance = %s, want 0", found.Balance)
		}
	})

	t.Run("finds wallet by id", func(t *testing.T) {
		found, err := repo.FindByID(ctx, wallet.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.UserID != userID {
			t.Errorf("UserID = %s, want %s", found.UserID, userID)
		}
	})

	t.Run("returns error for unknown user", func(t *testing.T) {
		_, err := repo.FindByUser(ctx, uuid.New(), nil)
		if err == nil {
			t.Error("expected error")
		}
	})

	t.Run("duplicate wallet returns error", func(t *testing.T) {
		dup := domain.NewWallet(userID, &tenantID, "CNY")
		err := repo.Create(ctx, &dup)
		if err == nil {
			t.Error("expected error for duplicate wallet")
		}
	})
}

func TestWalletReserveCommitRelease(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	userID := seedWalletUser(t, ctx, repo)

	// Create wallet with initial balance (top up manually)
	walletID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
		VALUES ($1, $2, '100.000000', '0.000000', 'CNY', 0)
	`, walletID, userID)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}

	t.Run("reserve creates transaction and freezes amount", func(t *testing.T) {
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(20), "idem-reserve-1")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		if tx.TxType != domain.WalletTxReserve {
			t.Errorf("TxType = %s, want reserve", tx.TxType)
		}
		if !tx.Amount.Equal(decimal.NewFromFloat(20)) {
			t.Errorf("Amount = %s, want 20", tx.Amount)
		}

		// Verify wallet state: frozen = 20
		found, _ := repo.FindByID(ctx, walletID)
		if !found.Frozen.Equal(decimal.NewFromFloat(20)) {
			t.Errorf("Frozen = %s, want 20", found.Frozen)
		}
		if !found.Balance.Equal(decimal.NewFromFloat(100)) {
			t.Errorf("Balance = %s, want 100 (unchanged)", found.Balance)
		}
		if found.Version != 1 {
			t.Errorf("Version = %d, want 1", found.Version)
		}
	})

	t.Run("reserve fails when insufficient balance", func(t *testing.T) {
		_, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(999), "idem-insuff")
		if err == nil {
			t.Error("expected error for insufficient balance")
		}
	})

	t.Run("commit reduces balance and frozen", func(t *testing.T) {
		tx, _ := repo.Reserve(ctx, walletID, decimal.NewFromFloat(10), "idem-commit-1")
		if err := repo.Commit(ctx, tx.ID); err != nil {
			t.Fatalf("Commit: %v", err)
		}

		found, _ := repo.FindByID(ctx, walletID)
		// After commit: balance 100 -> 90, frozen 20+10 -> 20 (minus committed amount)
		if !found.Balance.Equal(decimal.NewFromFloat(90)) {
			t.Errorf("Balance = %s, want 90", found.Balance)
		}
		if !found.Frozen.Equal(decimal.NewFromFloat(20)) {
			t.Errorf("Frozen = %s, want 20", found.Frozen)
		}
	})

	t.Run("commit non-existent tx returns error", func(t *testing.T) {
		err := repo.Commit(ctx, uuid.New())
		if err == nil {
			t.Error("expected error for unknown tx")
		}
	})

	t.Run("release unfreezes amount", func(t *testing.T) {
		tx, _ := repo.Reserve(ctx, walletID, decimal.NewFromFloat(5), "idem-release-1")
		if err := repo.Release(ctx, tx.ID); err != nil {
			t.Fatalf("Release: %v", err)
		}

		found, _ := repo.FindByID(ctx, walletID)
		// frozen was 20, reserve 5 → 25, release 5 → 20
		if !found.Frozen.Equal(decimal.NewFromFloat(20)) {
			t.Errorf("Frozen = %s, want 20", found.Frozen)
		}
	})

	t.Run("idempotency returns same transaction", func(t *testing.T) {
		tx1, _ := repo.Reserve(ctx, walletID, decimal.NewFromFloat(1), "idem-dup-1")
		tx2, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(99999), "idem-dup-1")
		if err != nil {
			t.Fatalf("idempotent reserve: %v", err)
		}
		if tx1.ID != tx2.ID {
			t.Errorf("expected same tx ID for duplicate idempotency key")
		}
		if !tx2.Amount.Equal(decimal.NewFromFloat(1)) {
			t.Errorf("Amount = %s, want 1 (original amount)", tx2.Amount)
		}
	})
}

func TestListTransactions(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users")

	userID := seedWalletUser(t, ctx, repo)

	walletID := uuid.New()
	repo.pool.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
		VALUES ($1, $2, '50.000000', '0.000000', 'CNY', 0)
	`, walletID, userID)

	// Create several transactions
	repo.Reserve(ctx, walletID, decimal.NewFromFloat(1), "list-1")
	repo.Reserve(ctx, walletID, decimal.NewFromFloat(2), "list-2")
	repo.Reserve(ctx, walletID, decimal.NewFromFloat(3), "list-3")

	t.Run("lists transactions with limit", func(t *testing.T) {
		txs, err := repo.ListTransactions(ctx, walletID, 2, 0)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(txs) != 2 {
			t.Errorf("len = %d, want 2", len(txs))
		}
	})

	t.Run("lists with offset", func(t *testing.T) {
		txs, err := repo.ListTransactions(ctx, walletID, 10, 2)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(txs) != 1 {
			t.Errorf("len = %d, want 1", len(txs))
		}
	})

	t.Run("empty for unknown wallet", func(t *testing.T) {
		txs, err := repo.ListTransactions(ctx, uuid.New(), 10, 0)
		if err != nil {
			t.Fatalf("ListTransactions: %v", err)
		}
		if len(txs) != 0 {
			t.Errorf("len = %d, want 0", len(txs))
		}
	})
}

func TestWalletEdgeCases(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users")

	userID := seedWalletUser(t, ctx, repo)

	walletID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
		VALUES ($1, $2, '100.000000', '0.000000', 'CNY', 0)
	`, walletID, userID)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}

	t.Run("reserve on non-existent wallet returns error", func(t *testing.T) {
		_, err := repo.Reserve(ctx, uuid.New(), decimal.NewFromFloat(10), "idem-no-wallet")
		if err == nil {
			t.Error("expected error for missing wallet")
		}
	})

	t.Run("release already released tx returns error", func(t *testing.T) {
		tx, _ := repo.Reserve(ctx, walletID, decimal.NewFromFloat(1), "idem-rel-twice")
		_ = repo.Release(ctx, tx.ID)
		err := repo.Release(ctx, tx.ID)
		if err == nil {
			t.Error("expected error for releasing a released tx")
		}
	})

	t.Run("commit already committed tx returns error", func(t *testing.T) {
		tx, _ := repo.Reserve(ctx, walletID, decimal.NewFromFloat(1), "idem-commit-twice")
		_ = repo.Commit(ctx, tx.ID)
		err := repo.Commit(ctx, tx.ID)
		if err == nil {
			t.Error("expected error for committing a committed tx")
		}
	})

	t.Run("list transactions with default limit", func(t *testing.T) {
		txs, err := repo.ListTransactions(ctx, walletID, 0, 0)
		if err != nil {
			t.Fatalf("ListTransactions default: %v", err)
		}
		if len(txs) < 1 {
			t.Errorf("expected at least 1 transaction, got %d", len(txs))
		}
	})
}

func TestParseDecimalStrHelper(t *testing.T) {
	t.Run("valid string", func(t *testing.T) {
		d := parseDecimalStr("42.5")
		if !d.Equal(decimal.NewFromFloat(42.5)) {
			t.Errorf("expected 42.5, got %s", d)
		}
	})
	t.Run("invalid string returns zero", func(t *testing.T) {
		d := parseDecimalStr("not-a-number")
		if !d.Equal(decimal.Zero) {
			t.Errorf("expected zero, got %s", d)
		}
	})
	t.Run("empty string returns zero", func(t *testing.T) {
		d := parseDecimalStr("")
		if !d.Equal(decimal.Zero) {
			t.Errorf("expected zero, got %s", d)
		}
	})
}

func TestWalletTopUp(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	userID := seedWalletUser(t, ctx, repo)
	wallet := domain.NewWallet(userID, nil, "CNY")
	if err := repo.Create(ctx, &wallet); err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Run("credits balance and records topup transaction", func(t *testing.T) {
		tx, err := repo.TopUp(ctx, wallet.ID, decimal.NewFromInt(500), "topup-key-1")
		if err != nil {
			t.Fatalf("TopUp: %v", err)
		}
		if tx.TxType != domain.WalletTxTopup {
			t.Errorf("TxType = %s, want %s", tx.TxType, domain.WalletTxTopup)
		}
		if !tx.BalanceAfter.Equal(decimal.NewFromInt(500)) {
			t.Errorf("BalanceAfter = %s, want 500", tx.BalanceAfter)
		}

		found, err := repo.FindByID(ctx, wallet.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if !found.Balance.Equal(decimal.NewFromInt(500)) {
			t.Errorf("Balance = %s, want 500", found.Balance)
		}
	})

	t.Run("is idempotent on same key", func(t *testing.T) {
		_, err := repo.TopUp(ctx, wallet.ID, decimal.NewFromInt(200), "topup-key-1")
		if err != nil {
			t.Fatalf("TopUp (idempotent): %v", err)
		}
		// Balance should NOT have increased again.
		found, err := repo.FindByID(ctx, wallet.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if !found.Balance.Equal(decimal.NewFromInt(500)) {
			t.Errorf("Balance = %s, want 500 (idempotent topup should not double-credit)", found.Balance)
		}
	})

	t.Run("rejects non-positive amount", func(t *testing.T) {
		_, err := repo.TopUp(ctx, wallet.ID, decimal.Zero, "topup-key-zero")
		if err == nil {
			t.Fatal("expected error for zero amount")
		}
	})

	t.Run("returns error for unknown wallet", func(t *testing.T) {
		_, err := repo.TopUp(ctx, uuid.New(), decimal.NewFromInt(10), "topup-key-unknown")
		if err == nil {
			t.Fatal("expected error for unknown wallet")
		}
	})
}

func TestWalletSettle(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	userID := seedWalletUser(t, ctx, repo)
	walletID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
		VALUES ($1, $2, '100.000000', '0.000000', 'CNY', 0)
	`, walletID, userID)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}

	t.Run("settle larger than reserve charges the extra", func(t *testing.T) {
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(10), "idem-settle-larger")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		// final cost 25 > reserved 10 → balance 100-25=75, frozen 0
		if err := repo.Settle(ctx, tx.ID, decimal.NewFromFloat(25)); err != nil {
			t.Fatalf("Settle: %v", err)
		}
		found, _ := repo.FindByID(ctx, walletID)
		if !found.Balance.Equal(decimal.NewFromFloat(75)) {
			t.Errorf("Balance = %s, want 75", found.Balance)
		}
		if !found.Frozen.IsZero() {
			t.Errorf("Frozen = %s, want 0", found.Frozen)
		}
		// transaction is marked as charge with final amount
		var txType string
		var amount string
		if err := repo.pool.QueryRow(ctx,
			`SELECT tx_type, amount::text FROM wallet_transactions WHERE id = $1`, tx.ID).Scan(&txType, &amount); err != nil {
			t.Fatalf("read tx: %v", err)
		}
		if txType != "charge" || amount != "25.000000" {
			t.Errorf("tx type/amount = %s/%s, want charge/25.000000", txType, amount)
		}
	})

	t.Run("settle smaller than reserve refunds the excess", func(t *testing.T) {
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(10), "idem-settle-smaller")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		// Balance nets -finalAmount: 75 - 4 = 71, frozen released.
		if err := repo.Settle(ctx, tx.ID, decimal.NewFromFloat(4)); err != nil {
			t.Fatalf("Settle: %v", err)
		}
		found, _ := repo.FindByID(ctx, walletID)
		if !found.Balance.Equal(decimal.NewFromFloat(71)) {
			t.Errorf("Balance = %s, want 71", found.Balance)
		}
		if !found.Frozen.IsZero() {
			t.Errorf("Frozen = %s, want 0", found.Frozen)
		}
	})

	t.Run("settle zero final cost refunds the whole reserve", func(t *testing.T) {
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(10), "idem-settle-zero")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		// Balance unchanged on zero final cost: 71.
		if err := repo.Settle(ctx, tx.ID, decimal.Zero); err != nil {
			t.Fatalf("Settle: %v", err)
		}
		found, _ := repo.FindByID(ctx, walletID)
		if !found.Balance.Equal(decimal.NewFromFloat(71)) {
			t.Errorf("Balance = %s, want 71", found.Balance)
		}
		if !found.Frozen.IsZero() {
			t.Errorf("Frozen = %s, want 0", found.Frozen)
		}
	})

	t.Run("settle larger than available balance fails", func(t *testing.T) {
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(10), "idem-settle-too-big")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		// balance 71, reserve 10 → available 61; final 100 requires +90 → fail
		err = repo.Settle(ctx, tx.ID, decimal.NewFromFloat(100))
		if err == nil {
			t.Fatal("expected ErrInsufficientBalance for oversized final cost")
		}
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("expected ErrInsufficientBalance, got %v", err)
		}
		// wallet unchanged
		found, _ := repo.FindByID(ctx, walletID)
		if !found.Balance.Equal(decimal.NewFromFloat(71)) || !found.Frozen.Equal(decimal.NewFromFloat(10)) {
			t.Errorf("wallet changed on failed settle: balance=%s frozen=%s", found.Balance, found.Frozen)
		}
	})

	t.Run("settle non-reserve transaction fails", func(t *testing.T) {
		// commit a reserve first, then settle it again → not in reserve state
		tx, err := repo.Reserve(ctx, walletID, decimal.NewFromFloat(1), "idem-settle-committed")
		if err != nil {
			t.Fatalf("Reserve: %v", err)
		}
		if err := repo.Commit(ctx, tx.ID); err != nil {
			t.Fatalf("Commit: %v", err)
		}
		if err := repo.Settle(ctx, tx.ID, decimal.NewFromFloat(1)); err == nil {
			t.Fatal("expected error settling a non-reserve transaction")
		}
	})
}
