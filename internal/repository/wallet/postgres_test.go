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

func TestWalletTransfer(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	fromUserID := seedWalletUser(t, ctx, repo)
	toUserID := seedWalletUser(t, ctx, repo)

	fromWalletID := uuid.New()
	toWalletID := uuid.New()
	seedWallet := func(id, userID uuid.UUID, balance string) {
		t.Helper()
		_, err := repo.pool.Exec(ctx, `
			INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
			VALUES ($1, $2, $3, '0.000000', 'CNY', 0)
		`, id, userID, balance)
		if err != nil {
			t.Fatalf("seed wallet: %v", err)
		}
	}
	seedWallet(fromWalletID, fromUserID, "100.000000")
	seedWallet(toWalletID, toUserID, "0.000000")

	t.Run("moves balance and records both sides", func(t *testing.T) {
		tx, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.NewFromInt(40), "transfer-1")
		if err != nil {
			t.Fatalf("Transfer: %v", err)
		}
		if tx.TxType != domain.WalletTxTransferOut {
			t.Errorf("TxType = %s, want %s", tx.TxType, domain.WalletTxTransferOut)
		}

		from, _ := repo.FindByID(ctx, fromWalletID)
		to, _ := repo.FindByID(ctx, toWalletID)
		if !from.Balance.Equal(decimal.NewFromInt(60)) {
			t.Errorf("from Balance = %s, want 60", from.Balance)
		}
		if !to.Balance.Equal(decimal.NewFromInt(40)) {
			t.Errorf("to Balance = %s, want 40", to.Balance)
		}
		if from.Version != 1 || to.Version != 1 {
			t.Errorf("versions = %d/%d, want 1/1", from.Version, to.Version)
		}

		// Two ledger rows share the idempotency key, with opposite signs.
		var n int
		if err := repo.pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM wallet_transactions WHERE idempotency_key = 'transfer-1'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 2 {
			t.Errorf("transactions count = %d, want 2", n)
		}
		var outType, inType string
		var outAmt, inAmt string
		if err := repo.pool.QueryRow(ctx,
			`SELECT tx_type, amount::text FROM wallet_transactions WHERE wallet_id = $1 AND idempotency_key = 'transfer-1'`,
			fromWalletID).Scan(&outType, &outAmt); err != nil {
			t.Fatalf("read out row: %v", err)
		}
		if err := repo.pool.QueryRow(ctx,
			`SELECT tx_type, amount::text FROM wallet_transactions WHERE wallet_id = $1 AND idempotency_key = 'transfer-1'`,
			toWalletID).Scan(&inType, &inAmt); err != nil {
			t.Fatalf("read in row: %v", err)
		}
		if outType != "transfer_out" || outAmt != "-40.000000" {
			t.Errorf("out row = %s/%s, want transfer_out/-40.000000", outType, outAmt)
		}
		if inType != "transfer_in" || inAmt != "40.000000" {
			t.Errorf("in row = %s/%s, want transfer_in/40.000000", inType, inAmt)
		}
	})

	t.Run("fails when source balance insufficient", func(t *testing.T) {
		_, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.NewFromInt(999), "transfer-insuff")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("err = %v, want ErrInsufficientBalance", err)
		}
		from, _ := repo.FindByID(ctx, fromWalletID)
		if !from.Balance.Equal(decimal.NewFromInt(60)) {
			t.Errorf("Balance = %s, want 60 (unchanged)", from.Balance)
		}
	})

	t.Run("is idempotent on same key", func(t *testing.T) {
		_, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.NewFromInt(40), "transfer-1")
		if err != nil {
			t.Fatalf("Transfer (idempotent): %v", err)
		}
		from, _ := repo.FindByID(ctx, fromWalletID)
		to, _ := repo.FindByID(ctx, toWalletID)
		if !from.Balance.Equal(decimal.NewFromInt(60)) || !to.Balance.Equal(decimal.NewFromInt(40)) {
			t.Errorf("balances changed on idempotent replay: from=%s to=%s", from.Balance, to.Balance)
		}
	})

	t.Run("rejects a key reused with a different amount", func(t *testing.T) {
		// Consume a fresh key for 40, then reuse it for 10. The second call
		// must be rejected, not silently replayed (which would report success
		// while moving no money). Expectations are derived from the current
		// balances so the sub-test is order-independent.
		beforeFrom, _ := repo.FindByID(ctx, fromWalletID)
		beforeTo, _ := repo.FindByID(ctx, toWalletID)
		if _, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.NewFromInt(40), "transfer-collide"); err != nil {
			t.Fatalf("Transfer (consume key): %v", err)
		}
		if _, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.NewFromInt(10), "transfer-collide"); err == nil {
			t.Fatal("expected ErrIdempotencyMismatch on key reuse with different amount")
		} else if !errors.Is(err, ErrIdempotencyMismatch) {
			t.Errorf("err = %v, want ErrIdempotencyMismatch", err)
		}
		afterFrom, _ := repo.FindByID(ctx, fromWalletID)
		afterTo, _ := repo.FindByID(ctx, toWalletID)
		if !afterFrom.Balance.Equal(beforeFrom.Balance.Sub(decimal.NewFromInt(40))) ||
			!afterTo.Balance.Equal(beforeTo.Balance.Add(decimal.NewFromInt(40))) {
			t.Errorf("rejected transfer moved money: from=%s to=%s, want delta of 40 only",
				afterFrom.Balance, afterTo.Balance)
		}
	})

	t.Run("fails when destination wallet missing", func(t *testing.T) {
		_, err := repo.Transfer(ctx, fromWalletID, uuid.New(), decimal.NewFromInt(1), "transfer-missing-to")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("fails when source wallet missing", func(t *testing.T) {
		_, err := repo.Transfer(ctx, uuid.New(), toWalletID, decimal.NewFromInt(1), "transfer-missing-from")
		if err == nil {
			t.Fatal("expected error")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("rejects non-positive amount", func(t *testing.T) {
		_, err := repo.Transfer(ctx, fromWalletID, toWalletID, decimal.Zero, "transfer-zero")
		if err == nil {
			t.Fatal("expected error for zero amount")
		}
	})
}

// TH-P05-02 (B5 Settle Fallback Visibility Correction).
//
// Pins the undercharge fallback path against a real database:
//   - Settle rejects a final cost the wallet cannot cover
//     (ErrInsufficientBalance) and leaves the wallet untouched;
//   - the caller's Commit fallback then charges exactly the reserved hold
//     and leaves balance/frozen/ledger mathematically consistent;
//   - any later replay (Settle/Commit/Release on the finalized tx) returns
//     ErrTxNotReserved and moves no money (AC-02 at the repository level).
func TestWalletSettleInsufficientFallbackCommitAndReplay(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants")

	userID := seedWalletUser(t, ctx, repo)
	walletID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO wallets (id, user_id, balance, frozen, currency, version)
		VALUES ($1, $2, '0.000000', '0.000000', 'CNY', 0)
	`, walletID, userID)
	if err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	// Fund via TopUp so the ledger fully backs the balance (W2 invariant).
	if _, err := repo.TopUp(ctx, walletID, decimal.NewFromInt(100), "p0502-topup"); err != nil {
		t.Fatalf("TopUp: %v", err)
	}

	// Reserve a hold of 20.
	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(20), "p0502-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	// Settle at a final cost the wallet cannot cover: reserved 20, final 150
	// needs +130 from available (100-20=80) → ErrInsufficientBalance.
	err = repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(150))
	if err == nil {
		t.Fatal("expected ErrInsufficientBalance for unaffordable final cost")
	}
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	found, _ := repo.FindByID(ctx, walletID)
	if !found.Balance.Equal(decimal.NewFromInt(100)) || !found.Frozen.Equal(decimal.NewFromInt(20)) {
		t.Fatalf("failed settle mutated wallet: balance=%s frozen=%s, want 100/20", found.Balance, found.Frozen)
	}

	// Fallback: commit the reserved hold. Balance nets exactly -20.
	if err := repo.Commit(ctx, reserveTx.ID); err != nil {
		t.Fatalf("Commit fallback: %v", err)
	}
	found, _ = repo.FindByID(ctx, walletID)
	if !found.Balance.Equal(decimal.NewFromInt(80)) {
		t.Errorf("Balance = %s, want 80 after committing the 20 hold", found.Balance)
	}
	if !found.Frozen.IsZero() {
		t.Errorf("Frozen = %s, want 0 after commit", found.Frozen)
	}

	// Ledger consistency (W1/W2): the committed row is the single charge.
	var chargeCount int
	var chargeAmt string
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MAX(amount::text), '') FROM wallet_transactions
		 WHERE wallet_id = $1 AND tx_type = 'charge'`, walletID).Scan(&chargeCount, &chargeAmt); err != nil {
		t.Fatalf("count charges: %v", err)
	}
	if chargeCount != 1 || chargeAmt != "20.000000" {
		t.Errorf("charge rows = %d/%s, want exactly one charge of 20.000000", chargeCount, chargeAmt)
	}
	var openReserves int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1 AND tx_type = 'reserve'`,
		walletID).Scan(&openReserves); err != nil {
		t.Fatalf("count open reserves: %v", err)
	}
	if openReserves != 0 || !found.Frozen.IsZero() {
		t.Errorf("W1 violated after commit: open reserves=%d frozen=%s, want 0/0", openReserves, found.Frozen)
	}
	// W2: balance == ledger net (topup +100, charge -20).
	var ledgerNet string
	if err := repo.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN tx_type IN ('reserve','release') THEN 0
		                           WHEN tx_type = 'charge' THEN -amount
		                           ELSE amount END), 0)::text
		 FROM wallet_transactions WHERE wallet_id = $1`, walletID).Scan(&ledgerNet); err != nil {
		t.Fatalf("ledger net: %v", err)
	}
	net := parseDecimalStr(ledgerNet)
	if !net.Equal(found.Balance) {
		t.Errorf("W2 violated: ledger net %s != balance %s", net, found.Balance)
	}

	// Replay: the transaction is finalized. Settle/Commit/Release must all
	// report ErrTxNotReserved and move no money (no double debit).
	if err := repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(150)); err == nil {
		t.Fatal("expected error re-settling a finalized transaction")
	} else if !errors.Is(err, ErrTxNotReserved) {
		t.Errorf("replay Settle err = %v, want ErrTxNotReserved", err)
	}
	if err := repo.Commit(ctx, reserveTx.ID); err == nil {
		t.Fatal("expected error re-committing a finalized transaction")
	} else if !errors.Is(err, ErrTxNotReserved) {
		t.Errorf("replay Commit err = %v, want ErrTxNotReserved", err)
	}
	if err := repo.Release(ctx, reserveTx.ID); err == nil {
		t.Fatal("expected error releasing a finalized transaction")
	} else if !errors.Is(err, ErrTxNotReserved) {
		t.Errorf("replay Release err = %v, want ErrTxNotReserved", err)
	}
	after, _ := repo.FindByID(ctx, walletID)
	if !after.Balance.Equal(decimal.NewFromInt(80)) || !after.Frozen.IsZero() {
		t.Errorf("replay moved money: balance=%s frozen=%s, want 80/0", after.Balance, after.Frozen)
	}
	var txRows int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1`, walletID).Scan(&txRows); err != nil {
		t.Fatalf("count tx rows: %v", err)
	}
	if txRows != 2 {
		t.Errorf("ledger rows = %d, want 2 (topup + charge); replay must not append rows", txRows)
	}
}
