package wallet

// TH-P05-03 (Billing Invariant And Concurrency Tests).
//
// Real-Postgres proof that the wallet money semantics hold under
// repetition, concurrency and exceptions. These tests PIN the established
// semantics; they never change them. Invariants covered:
//
//	W1  balance never negative
//	W2  frozen never negative
//	W3  available (balance-frozen) never negative; concurrent reserves
//	    cannot over-consume
//	W4  every successful mutation leaves ledger evidence
//	W5  one idempotency key -> one money effect
//	W7  settle only on a reserved tx; repeated settle charges once
//	W8  release only on a reserved tx; repeated release never credits
//	W9  settle/release race -> exactly one legal final state
//	W10 commit fallback never charges more than the reserved hold
//	W11 wallet state is fully explainable by the ledger

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// p0503Tables is the truncation set shared by the invariant tests.
var p0503Tables = []string{"wallet_transactions", "wallets", "api_key_spend", "api_keys", "users", "tenants"}

// seedP0503Wallet creates a user and a wallet; when initial is positive the
// wallet is funded through a ledgered TopUp so W11 recomputation holds from
// the very first assertion.
func seedP0503Wallet(t *testing.T, ctx context.Context, repo *PostgresRepository, initial decimal.Decimal) uuid.UUID {
	t.Helper()
	userID := seedWalletUser(t, ctx, repo)
	w := domain.NewWallet(userID, nil, "CNY")
	if err := repo.Create(ctx, &w); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	if initial.IsPositive() {
		if _, err := repo.TopUp(ctx, w.ID, initial, "p0503-topup-"+w.ID.String()); err != nil {
			t.Fatalf("seed topup: %v", err)
		}
	}
	return w.ID
}

// p0503LedgerNet recomputes the wallet balance from wallet_transactions:
// reserve/release rows never move the balance; charge deducts; every other
// row adds its stored amount (transfer_out and subscription are stored
// negative). This is the same recompute used by TH-P05-02.
func p0503LedgerNet(t *testing.T, ctx context.Context, repo *PostgresRepository, walletID uuid.UUID) decimal.Decimal {
	t.Helper()
	var netStr string
	if err := repo.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(CASE WHEN tx_type IN ('reserve','release') THEN 0
		                          WHEN tx_type = 'charge' THEN -amount
		                          ELSE amount END), 0)::text
		 FROM wallet_transactions WHERE wallet_id = $1`, walletID).Scan(&netStr); err != nil {
		t.Fatalf("ledger net query: %v", err)
	}
	return parseDecimalStr(netStr)
}

// p0503OpenReserveSum returns the sum of amounts of still-open reserve rows
// (what wallets.frozen must equal).
func p0503OpenReserveSum(t *testing.T, ctx context.Context, repo *PostgresRepository, walletID uuid.UUID) decimal.Decimal {
	t.Helper()
	var sumStr string
	if err := repo.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0)::text FROM wallet_transactions
		 WHERE wallet_id = $1 AND tx_type = 'reserve'`, walletID).Scan(&sumStr); err != nil {
		t.Fatalf("open reserve sum query: %v", err)
	}
	return parseDecimalStr(sumStr)
}

// p0503AssertInvariants asserts W1/W2/W3 (non-negativity) and W11
// (balance and frozen are exactly explainable by the ledger).
func p0503AssertInvariants(t *testing.T, ctx context.Context, repo *PostgresRepository, walletID uuid.UUID) {
	t.Helper()
	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found.Balance.IsNegative() {
		t.Errorf("W1 violated: negative balance %s", found.Balance)
	}
	if found.Frozen.IsNegative() {
		t.Errorf("W2 violated: negative frozen %s", found.Frozen)
	}
	if available := found.Balance.Sub(found.Frozen); available.IsNegative() {
		t.Errorf("W3 violated: negative available balance %s (balance=%s frozen=%s)",
			available, found.Balance, found.Frozen)
	}
	if net := p0503LedgerNet(t, ctx, repo, walletID); !net.Equal(found.Balance) {
		t.Errorf("W11 violated: ledger net %s != wallet balance %s", net, found.Balance)
	}
	if open := p0503OpenReserveSum(t, ctx, repo, walletID); !open.Equal(found.Frozen) {
		t.Errorf("W11 violated: open reserve sum %s != frozen %s", open, found.Frozen)
	}
}

// p0503CountRows counts ledger rows of the wallet, optionally by tx_type.
func p0503CountRows(t *testing.T, ctx context.Context, repo *PostgresRepository, walletID uuid.UUID, txType string) int {
	t.Helper()
	query := `SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1`
	args := []any{walletID}
	if txType != "" {
		query += ` AND tx_type = $2`
		args = append(args, txType)
	}
	var n int
	if err := repo.pool.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return n
}

// ---------------------------------------------------------------------------
// Case A: one wallet, N concurrent reserves for the full remaining balance.
// Exactly one may succeed; every other attempt must fail with
// ErrInsufficientBalance (W3: concurrent reserves cannot over-consume).
// ---------------------------------------------------------------------------

func TestP0503_CaseA_ConcurrentReserves_ExactlyOneSucceeds(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(10))

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	txs := make([]*domain.WalletTransaction, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(10),
				fmt.Sprintf("p0503-caseA-%d-%s", i, uuid.New().String()))
			results[i] = err
			txs[i] = tx
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	winner := -1
	for i, err := range results {
		if err == nil {
			successes++
			winner = i
			continue
		}
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Errorf("worker %d: error %v, want ErrInsufficientBalance", i, err)
		}
	}
	if successes != 1 {
		t.Fatalf("W3 violated: %d of %d concurrent full-balance reserves succeeded; want exactly 1",
			successes, workers)
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(10)) || !found.Frozen.Equal(decimal.NewFromInt(10)) {
		t.Errorf("wallet = %s/%s, want balance 10 / frozen 10 after one successful reserve",
			found.Balance, found.Frozen)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "reserve"); rows != 1 {
		t.Errorf("open reserve rows = %d, want 1", rows)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)

	// Finalize the winning hold: settling exactly the frozen amount must
	// leave a zeroed, ledger-consistent wallet.
	if err := repo.Settle(ctx, txs[winner].ID, decimal.NewFromInt(10)); err != nil {
		t.Fatalf("settle winning reserve: %v", err)
	}
	after, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID after settle: %v", err)
	}
	if !after.Balance.IsZero() || !after.Frozen.IsZero() {
		t.Errorf("after settle = %s/%s, want 0/0", after.Balance, after.Frozen)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// Case A (partial capacity): balance 10, three concurrent reserves of 6 —
// after one succeeds only 4 remains available, so again exactly one wins.
func TestP0503_CaseA_PartialCapacity_StillOneWinner(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(10))

	const workers = 3
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(6),
				fmt.Sprintf("p0503-caseA2-%d-%s", i, uuid.New().String()))
			results[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("W3 violated: %d of %d concurrent reserves of 6 against balance 10 succeeded; want exactly 1",
			successes, workers)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case B: N concurrent reserves sharing ONE idempotency key. Exactly one
// money effect: one ledger row, one freeze. Losers either error on the
// unique (wallet_id, idempotency_key) index or, when serialized after the
// winner's commit, observe the existing row — both are legal, but neither
// may move money a second time (W5).
// ---------------------------------------------------------------------------

func TestP0503_CaseB_ConcurrentDuplicateKeyReserve_OneMoneyEffect(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(100))

	const workers = 8
	key := "p0503-caseB-" + uuid.New().String()
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	txs := make([]*domain.WalletTransaction, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(5), key)
			results[i] = err
			txs[i] = tx
		}(i)
	}
	close(start)
	wg.Wait()

	// Every success must reference the SAME transaction; any error is fine
	// (unique-index collision) as long as it produced no ledger row.
	successIDs := map[uuid.UUID]int{}
	for i, err := range results {
		if err == nil {
			successIDs[txs[i].ID]++
		}
	}
	if len(successIDs) != 1 {
		t.Fatalf("W5 violated: concurrent same-key reserves produced %d distinct transactions; want exactly 1",
			len(successIDs))
	}

	if rows := p0503CountRows(t, ctx, repo, walletID, ""); rows != 2 { // topup + exactly one reserve
		t.Errorf("ledger rows = %d, want 2 (seed topup + one reserve); duplicate money effect?", rows)
	}
	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Frozen.Equal(decimal.NewFromInt(5)) {
		t.Errorf("W5 violated: frozen = %s, want exactly one freeze of 5", found.Frozen)
	}
	if !found.Balance.Equal(decimal.NewFromInt(100)) {
		t.Errorf("balance = %s, want 100 (reserve must not touch balance)", found.Balance)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case C: concurrent Settle vs Release on the same reserve (W9). Exactly one
// transition wins; every loser must observe ErrTxNotReserved; the wallet ends
// in exactly one legal final state.
// ---------------------------------------------------------------------------

func TestP0503_CaseC_SettleReleaseRace_OneLegalFinalState(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(50))

	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(10), "p0503-caseC-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	const workers = 16
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			if i%2 == 0 {
				results[i] = repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(8))
			} else {
				results[i] = repo.Release(ctx, reserveTx.ID)
			}
		}(i)
	}
	close(start)
	wg.Wait()

	winner := -1
	for i, err := range results {
		if err == nil {
			if winner != -1 {
				t.Fatalf("W9 violated: settle/release race had multiple winners (%d and %d)", winner, i)
			}
			winner = i
			continue
		}
		if !errors.Is(err, ErrTxNotReserved) {
			t.Errorf("worker %d: error %v, want ErrTxNotReserved", i, err)
		}
	}
	if winner == -1 {
		t.Fatal("W9 violated: no winner in settle/release race")
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	var txType, amountStr string
	if err := repo.pool.QueryRow(ctx,
		`SELECT tx_type, amount::text FROM wallet_transactions WHERE id = $1`, reserveTx.ID).
		Scan(&txType, &amountStr); err != nil {
		t.Fatalf("read tx: %v", err)
	}

	if winner%2 == 0 {
		// Settle won: charged exactly 8, frozen cleared.
		if txType != "charge" || amountStr != "8.000000" {
			t.Errorf("tx = %s/%s, want charge/8.000000 after settle wins the race", txType, amountStr)
		}
		if !found.Balance.Equal(decimal.NewFromInt(42)) || !found.Frozen.IsZero() {
			t.Errorf("wallet = %s/%s, want 42/0 after settle(8) wins", found.Balance, found.Frozen)
		}
	} else {
		// Release won: balance untouched, frozen cleared.
		if txType != "release" {
			t.Errorf("tx = %s, want release after release wins the race", txType)
		}
		if !found.Balance.Equal(decimal.NewFromInt(50)) || !found.Frozen.IsZero() {
			t.Errorf("wallet = %s/%s, want 50/0 after release wins", found.Balance, found.Frozen)
		}
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case D: concurrent duplicate Settle on the same reserve (W7). Exactly one
// charge, no double debit.
// ---------------------------------------------------------------------------

func TestP0503_CaseD_ConcurrentDuplicateSettle_OneCharge(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(50))

	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(10), "p0503-caseD-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(6))
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("W7 violated: %d of %d concurrent settles succeeded; want exactly 1 (double debit risk)",
			successes, workers)
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(44)) {
		t.Errorf("W7 violated: balance = %s, want 44 (charged exactly once)", found.Balance)
	}
	if !found.Frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", found.Frozen)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "charge"); rows != 1 {
		t.Errorf("charge rows = %d, want exactly 1", rows)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case E: concurrent duplicate Release on the same reserve (W8). Exactly one
// release; repeated releases never credit the balance.
// ---------------------------------------------------------------------------

func TestP0503_CaseE_ConcurrentDuplicateRelease_OneRelease(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(50))

	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(10), "p0503-caseE-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make([]error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results[i] = repo.Release(ctx, reserveTx.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("W8 violated: %d of %d concurrent releases succeeded; want exactly 1", successes, workers)
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(50)) {
		t.Errorf("W8 violated: balance = %s, want 50 (release must never credit)", found.Balance)
	}
	if !found.Frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", found.Frozen)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "release"); rows != 1 {
		t.Errorf("release rows = %d, want exactly 1", rows)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case F: parallel independent operations on one wallet (different keys and
// transactions). Whatever the interleaving, the final wallet must equal the
// ledger recomputation exactly (W11) and stay non-negative (W1-W3).
// ---------------------------------------------------------------------------

func TestP0503_CaseF_ParallelIndependentOps_LedgerConsistent(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(100))

	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 12)

	// 4 workers: reserve 7 then settle at 5 (net -5 each).
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(7),
				fmt.Sprintf("p0503-caseF-settle-%d-%s", i, uuid.New().String()))
			if err != nil {
				errs[i] = err
				return
			}
			errs[i] = repo.Settle(ctx, tx.ID, decimal.NewFromInt(5))
		}(i)
	}
	// 2 workers: top up 3 (net +3 each).
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[4+i] = repo.TopUp(ctx, walletID, decimal.NewFromInt(3),
				fmt.Sprintf("p0503-caseF-topup-%d-%s", i, uuid.New().String()))
		}(i)
	}
	// 2 workers: reserve 4 then release (net 0 each).
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			tx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(4),
				fmt.Sprintf("p0503-caseF-release-%d-%s", i, uuid.New().String()))
			if err != nil {
				errs[6+i] = err
				return
			}
			errs[6+i] = repo.Release(ctx, tx.ID)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d failed: %v", i, err)
		}
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	// 100 - 4*5 + 2*3 = 86, with every hold finalized.
	if !found.Balance.Equal(decimal.NewFromInt(86)) {
		t.Errorf("balance = %s, want 86 after parallel mixed operations", found.Balance)
	}
	if !found.Frozen.IsZero() {
		t.Errorf("frozen = %s, want 0 (all holds finalized)", found.Frozen)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "charge"); rows != 4 {
		t.Errorf("charge rows = %d, want 4", rows)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "release"); rows != 2 {
		t.Errorf("release rows = %d, want 2", rows)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "topup"); rows != 3 { // seed + 2
		t.Errorf("topup rows = %d, want 3", rows)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// Case G: settle larger than the reserve (W10). The settle must be rejected
// without moving a cent (no silent over-deduct), and the TH-P05-02 pinned
// fallback — commit the reserved hold — must charge exactly the hold, never
// the unaffordable final cost. Negative final amounts are rejected outright.
// ---------------------------------------------------------------------------

func TestP0503_CaseG_SettleOverReserve_NeverOverDeducts(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(30))

	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(20), "p0503-caseG-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	// Final cost 150 > reserve 20 and > available (30-20=10): rejected, and
	// the failed settle must leave the wallet exactly as it was.
	err = repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(150))
	if err == nil {
		t.Fatal("W10 violated: settle(150) on reserve(20) with available 10 succeeded")
	}
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Fatalf("err = %v, want ErrInsufficientBalance", err)
	}
	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(30)) || !found.Frozen.Equal(decimal.NewFromInt(20)) {
		t.Errorf("failed settle mutated wallet: %s/%s, want 30/20", found.Balance, found.Frozen)
	}

	// Pinned fallback (TH-P05-02): commit charges exactly the reserved hold.
	if err := repo.Commit(ctx, reserveTx.ID); err != nil {
		t.Fatalf("Commit fallback: %v", err)
	}
	found, err = repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID after commit: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(10)) {
		t.Errorf("W10 violated: balance = %s, want 10 (charged the 20 hold, never the 150 final)",
			found.Balance)
	}
	if !found.Frozen.IsZero() {
		t.Errorf("frozen = %s, want 0", found.Frozen)
	}
	var chargeRows int
	var chargeAmt string
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(MAX(amount::text),'') FROM wallet_transactions
		 WHERE wallet_id = $1 AND tx_type = 'charge'`, walletID).Scan(&chargeRows, &chargeAmt); err != nil {
		t.Fatalf("count charges: %v", err)
	}
	if chargeRows != 1 || chargeAmt != "20.000000" {
		t.Errorf("charges = %d/%s, want exactly one charge of 20.000000", chargeRows, chargeAmt)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)

	// Negative final amounts are rejected before any wallet access.
	tx2, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(1), "p0503-caseG-negative")
	if err != nil {
		t.Fatalf("Reserve (negative case): %v", err)
	}
	if err := repo.Settle(ctx, tx2.ID, decimal.NewFromInt(-5)); err == nil {
		t.Fatal("W10 violated: settle with a negative final amount succeeded")
	}
	found, err = repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID after negative settle: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(10)) || !found.Frozen.Equal(decimal.NewFromInt(1)) {
		t.Errorf("negative settle mutated wallet: %s/%s, want 10/1", found.Balance, found.Frozen)
	}
	// Clean up the open hold; invariants must hold at the end.
	if err := repo.Release(ctx, tx2.ID); err != nil {
		t.Fatalf("Release cleanup: %v", err)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// W4: every successful mutation leaves ledger evidence. Reserve/topup append
// a row; settle/commit/release finalize that row in place (state transition
// IS the evidence) — no mutation may happen without a trace.
// ---------------------------------------------------------------------------

func TestP0503_W4_EveryMutationHasLedgerEvidence(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.Zero)

	if rows := p0503CountRows(t, ctx, repo, walletID, ""); rows != 0 {
		t.Fatalf("fresh wallet has %d ledger rows, want 0", rows)
	}

	// TopUp: one evidence row.
	if _, err := repo.TopUp(ctx, walletID, decimal.NewFromInt(25), "p0503-w4-topup"); err != nil {
		t.Fatalf("TopUp: %v", err)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, "topup"); rows != 1 {
		t.Errorf("topup rows = %d, want 1", rows)
	}

	// Reserve: second evidence row.
	reserveTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(10), "p0503-w4-reserve")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, ""); rows != 2 {
		t.Errorf("rows after reserve = %d, want 2", rows)
	}

	// Settle finalizes the reserve row in place: row count unchanged, the row
	// now carries the charge state, final amount and post-settle balance.
	if err := repo.Settle(ctx, reserveTx.ID, decimal.NewFromInt(6)); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, ""); rows != 2 {
		t.Errorf("rows after settle = %d, want 2 (settle finalizes in place)", rows)
	}
	var txType, amountStr, balanceAfterStr string
	if err := repo.pool.QueryRow(ctx,
		`SELECT tx_type, amount::text, balance_after::text FROM wallet_transactions WHERE id = $1`,
		reserveTx.ID).Scan(&txType, &amountStr, &balanceAfterStr); err != nil {
		t.Fatalf("read settled row: %v", err)
	}
	if txType != "charge" || amountStr != "6.000000" || balanceAfterStr != "19.000000" {
		t.Errorf("settled row = %s/%s/%s, want charge/6.000000/19.000000", txType, amountStr, balanceAfterStr)
	}

	// Release finalizes a second reserve row in place as well.
	releaseTx, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(4), "p0503-w4-release")
	if err != nil {
		t.Fatalf("Reserve (release case): %v", err)
	}
	if err := repo.Release(ctx, releaseTx.ID); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if rows := p0503CountRows(t, ctx, repo, walletID, ""); rows != 3 {
		t.Errorf("rows after release = %d, want 3", rows)
	}
	if err := repo.pool.QueryRow(ctx,
		`SELECT tx_type FROM wallet_transactions WHERE id = $1`, releaseTx.ID).Scan(&txType); err != nil {
		t.Fatalf("read released row: %v", err)
	}
	if txType != "release" {
		t.Errorf("released row type = %s, want release", txType)
	}

	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Balance.Equal(decimal.NewFromInt(19)) || !found.Frozen.IsZero() {
		t.Errorf("wallet = %s/%s, want 19/0", found.Balance, found.Frozen)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}

// ---------------------------------------------------------------------------
// W5 (sequential): replaying an idempotency key returns the ORIGINAL
// transaction — never a second freeze, never a rewritten amount.
// ---------------------------------------------------------------------------

func TestP0503_W5_IdempotentReplay_OriginalTransactionOnly(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, p0503Tables...)
	walletID := seedP0503Wallet(t, ctx, repo, decimal.NewFromInt(50))

	key := "p0503-w5-" + uuid.New().String()
	first, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(5), key)
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	// Replay with a DIFFERENT amount: the original transaction must come
	// back unchanged (amount 5), not a new 999 freeze.
	replay, err := repo.Reserve(ctx, walletID, decimal.NewFromInt(999), key)
	if err != nil {
		t.Fatalf("idempotent replay errored: %v", err)
	}
	if replay.ID != first.ID {
		t.Errorf("W5 violated: replay created a new transaction %s, want original %s", replay.ID, first.ID)
	}
	if !replay.Amount.Equal(decimal.NewFromInt(5)) {
		t.Errorf("W5 violated: replay amount = %s, want original 5", replay.Amount)
	}

	var keyRows int
	if err := repo.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM wallet_transactions WHERE wallet_id = $1 AND idempotency_key = $2`,
		walletID, key).Scan(&keyRows); err != nil {
		t.Fatalf("count key rows: %v", err)
	}
	if keyRows != 1 {
		t.Errorf("W5 violated: %d ledger rows for one idempotency key, want 1", keyRows)
	}
	found, err := repo.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if !found.Frozen.Equal(decimal.NewFromInt(5)) {
		t.Errorf("W5 violated: frozen = %s, want exactly one freeze of 5", found.Frozen)
	}
	p0503AssertInvariants(t, ctx, repo, walletID)
}
