package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// assertLedgerMatchesBalance verifies the reconciliation invariant behind the
// B2 fix: a wallet's balance must equal the sum of its wallet_transactions
// amounts, so no balance change can exist off-ledger.
func assertLedgerMatchesBalance(t *testing.T, a *app.App, wal *domain.Wallet) {
	t.Helper()
	txs, err := a.Wallets.ListTransactions(context.Background(), wal.ID, 100, 0)
	if err != nil {
		t.Fatalf("ListTransactions(wallet=%s): %v", wal.ID, err)
	}
	sum := decimal.Zero
	for _, tx := range txs {
		sum = sum.Add(tx.Amount)
	}
	if !sum.Equal(wal.Balance) {
		t.Errorf("ledger sum = %s, wallet balance = %s (wallet %s): every balance change must have a ledger row",
			sum, wal.Balance, wal.ID)
	}
}

func TestProvisionUserWallet_BonusIsLedgered(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	user := seedUserForConsoleTest(t, a, "provision-bonus@example.com", "password123", "Provision Bonus")
	bonus := decimal.NewFromInt(1000)

	// Act
	wal, err := ProvisionUserWallet(context.Background(), a.Wallets, user.ID, bonus)
	if err != nil {
		t.Fatalf("ProvisionUserWallet: %v", err)
	}

	// Assert: balance reflects the bonus.
	found, err := a.Wallets.FindByUser(context.Background(), user.ID, nil)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if !found.Balance.Equal(bonus) {
		t.Errorf("balance = %s, want %s", found.Balance, bonus)
	}

	// Assert: exactly one ledger row, a topup of the bonus from 0 to bonus.
	txs, err := a.Wallets.ListTransactions(context.Background(), wal.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(txs) != 1 {
		t.Fatalf("ledger rows = %d, want 1 (bonus must be ledgered)", len(txs))
	}
	tx := txs[0]
	if tx.TxType != domain.WalletTxTopup {
		t.Errorf("tx_type = %s, want %s", tx.TxType, domain.WalletTxTopup)
	}
	if !tx.Amount.Equal(bonus) {
		t.Errorf("amount = %s, want %s", tx.Amount, bonus)
	}
	if !tx.BalanceBefore.IsZero() {
		t.Errorf("balance_before = %s, want 0", tx.BalanceBefore)
	}
	if !tx.BalanceAfter.Equal(bonus) {
		t.Errorf("balance_after = %s, want %s", tx.BalanceAfter, bonus)
	}
	if tx.IdempotencyKey != SignupBonusKey(user.ID) {
		t.Errorf("idempotency_key = %s, want %s", tx.IdempotencyKey, SignupBonusKey(user.ID))
	}
}

func TestProvisionUserWallet_ZeroBonusWritesNoLedgerRow(t *testing.T) {
	// Arrange: production mode grants no signup bonus.
	a := appForConsoleTest(t)
	user := seedUserForConsoleTest(t, a, "provision-zero@example.com", "password123", "Provision Zero")

	// Act
	wal, err := ProvisionUserWallet(context.Background(), a.Wallets, user.ID, decimal.Zero)
	if err != nil {
		t.Fatalf("ProvisionUserWallet: %v", err)
	}

	// Assert: zero balance, zero ledger rows (no money moved).
	found, err := a.Wallets.FindByUser(context.Background(), user.ID, nil)
	if err != nil {
		t.Fatalf("FindByUser: %v", err)
	}
	if !found.Balance.IsZero() {
		t.Errorf("balance = %s, want 0", found.Balance)
	}
	txs, err := a.Wallets.ListTransactions(context.Background(), wal.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListTransactions: %v", err)
	}
	if len(txs) != 0 {
		t.Errorf("ledger rows = %d, want 0 for a zero-balance initialization", len(txs))
	}
}

// TestRegister_AllGrantsReconcileWithLedger is the B2 regression: after a
// signup with an invite code, every coin in both wallets must be traceable to
// a wallet_transactions row (signup bonus + both invite rewards).
func TestRegister_AllGrantsReconcileWithLedger(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	inviter := seedInviter(t, a, "recon-inviter@example.com")

	// Act: FakePayment=true → signup bonus 1000; default invite reward 10.
	w := registerWithInvite(t, a, "recon-invitee@example.com", "DTPTEST01")
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Assert: invitee ledger holds exactly the signup bonus and the invite
	// reward, and the ledger sums to the balance.
	invitee, err := a.Users.FindByEmail(context.Background(), "recon-invitee@example.com")
	if err != nil {
		t.Fatalf("FindByEmail(invitee): %v", err)
	}
	inviteeWal, err := a.Wallets.FindByUser(context.Background(), invitee.ID, nil)
	if err != nil {
		t.Fatalf("FindByUser(invitee): %v", err)
	}
	if !inviteeWal.Balance.Equal(decimal.NewFromInt(1010)) {
		t.Errorf("invitee balance = %s, want 1010 (1000 bonus + 10 reward)", inviteeWal.Balance)
	}
	assertLedgerMatchesBalance(t, a, inviteeWal)
	assertLedgerKeys(t, a, inviteeWal.ID, []string{
		SignupBonusKey(invitee.ID),
		"invite:" + invitee.ID.String(),
	})

	// Assert: inviter ledger holds exactly the reward and sums to the balance.
	inviterWal, err := a.Wallets.FindByUser(context.Background(), inviter.ID, nil)
	if err != nil {
		t.Fatalf("FindByUser(inviter): %v", err)
	}
	if !inviterWal.Balance.Equal(decimal.NewFromInt(10)) {
		t.Errorf("inviter balance = %s, want 10", inviterWal.Balance)
	}
	assertLedgerMatchesBalance(t, a, inviterWal)
	assertLedgerKeys(t, a, inviterWal.ID, []string{
		"invite:" + invitee.ID.String() + ":inviter",
	})
}

// assertLedgerKeys verifies a wallet has exactly the given ledger rows,
// identified by idempotency key.
func assertLedgerKeys(t *testing.T, a *app.App, walletID uuid.UUID, wantKeys []string) {
	t.Helper()
	txs, err := a.Wallets.ListTransactions(context.Background(), walletID, 100, 0)
	if err != nil {
		t.Fatalf("ListTransactions(wallet=%s): %v", walletID, err)
	}
	if len(txs) != len(wantKeys) {
		t.Fatalf("wallet %s has %d ledger rows, want %d", walletID, len(txs), len(wantKeys))
	}
	got := make(map[string]bool, len(txs))
	for _, tx := range txs {
		got[tx.IdempotencyKey] = true
	}
	for _, key := range wantKeys {
		if !got[key] {
			t.Errorf("wallet %s missing ledger row with idempotency key %q", walletID, key)
		}
	}
}
