package paymentorder

// TH-P1-CW-01 — pending-order scanner candidate query tests (persistence
// layer). The scanner selects pending orders by channel, age, and limit
// with deterministic ordering; expiry/retry eligibility is enforced by the
// worker-level rules (internal/worker/paymentscan).

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// seedScannerOrder inserts an order with an explicit created_at, status and
// channel so age/status/channel filtering is fully under test control.
func seedScannerOrder(t *testing.T, repo *PostgresRepository, orderNo, status, channel string, userID uuid.UUID, createdAt time.Time) {
	t.Helper()
	_, err := repo.pool.Exec(context.Background(),
		`INSERT INTO payment_orders
			(order_no, user_id, amount, currency, purpose, channel, pay_method, status, expires_at, created_at, updated_at)
		 VALUES ($1,$2,$3,'CNY','topup',$4,'alipay',$5, NOW() + interval '1 hour', $6, $6)`,
		orderNo, userID, decimal.NewFromInt(10), channel, status, createdAt)
	if err != nil {
		t.Fatalf("seed scanner order %s: %v", orderNo, err)
	}
}

// TestListPendingCandidatesSelectsOldPending covers AC-01 at the persistence
// layer: pending orders older than the threshold are candidates; too-new,
// paid, closed and refunded rows are excluded (AC-02).
func TestListPendingCandidatesSelectsOldPending(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	now := time.Now()
	old := now.Add(-10 * time.Minute)
	seedScannerOrder(t, repo, "DTPSCAN_OLD1", StatusPending, "epay", userID, old)
	seedScannerOrder(t, repo, "DTPSCAN_NEW1", StatusPending, "epay", userID, now.Add(-2*time.Second))
	seedScannerOrder(t, repo, "DTPSCAN_PAID1", StatusPaid, "epay", userID, old)
	seedScannerOrder(t, repo, "DTPSCAN_CLOSED1", StatusClosed, "epay", userID, old)
	seedScannerOrder(t, repo, "DTPSCAN_REFUND1", StatusRefunded, "epay", userID, old)

	got, err := repo.ListPendingCandidates(ctx, now.Add(-time.Minute), 50, nil)
	if err != nil {
		t.Fatalf("ListPendingCandidates: %v", err)
	}
	if len(got) != 1 || got[0].OrderNo != "DTPSCAN_OLD1" {
		t.Fatalf("expected exactly the old pending order, got %+v", orderNos(got))
	}
}

// TestListPendingCandidatesDeterministicOrderAndLimit: candidates come back
// oldest-first with order_no as the tiebreak, and the batch limit holds.
func TestListPendingCandidatesDeterministicOrderAndLimit(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	base := time.Now().Add(-time.Hour)
	seedScannerOrder(t, repo, "DTPSCAN_B", StatusPending, "epay", userID, base.Add(2*time.Minute))
	seedScannerOrder(t, repo, "DTPSCAN_A", StatusPending, "epay", userID, base)
	seedScannerOrder(t, repo, "DTPSCAN_C", StatusPending, "epay", userID, base.Add(time.Minute))
	seedScannerOrder(t, repo, "DTPSCAN_D", StatusPending, "epay", userID, base.Add(3*time.Minute))

	all, err := repo.ListPendingCandidates(ctx, time.Now(), 50, nil)
	if err != nil {
		t.Fatalf("ListPendingCandidates: %v", err)
	}
	want := []string{"DTPSCAN_A", "DTPSCAN_C", "DTPSCAN_B", "DTPSCAN_D"}
	if len(all) != len(want) {
		t.Fatalf("expected %d candidates, got %+v", len(want), orderNos(all))
	}
	for i, no := range want {
		if all[i].OrderNo != no {
			t.Fatalf("position %d = %s, want %s (full order: %v)", i, all[i].OrderNo, no, orderNos(all))
		}
	}

	batch, err := repo.ListPendingCandidates(ctx, time.Now(), 2, nil)
	if err != nil {
		t.Fatalf("ListPendingCandidates limit: %v", err)
	}
	if len(batch) != 2 || batch[0].OrderNo != "DTPSCAN_A" || batch[1].OrderNo != "DTPSCAN_C" {
		t.Fatalf("small batch must hold the deterministic prefix, got %+v", orderNos(batch))
	}
}

// TestListPendingCandidatesChannelFilter: the optional channel selector
// restricts candidates without touching other channels' rows.
func TestListPendingCandidatesChannelFilter(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)

	old := time.Now().Add(-10 * time.Minute)
	seedScannerOrder(t, repo, "DTPSCAN_EPAY", StatusPending, "epay", userID, old)
	seedScannerOrder(t, repo, "DTPSCAN_ALI", StatusPending, "alipay", userID, old)

	alipay := "alipay"
	got, err := repo.ListPendingCandidates(ctx, time.Now(), 50, &alipay)
	if err != nil {
		t.Fatalf("ListPendingCandidates(channel=alipay): %v", err)
	}
	if len(got) != 1 || got[0].OrderNo != "DTPSCAN_ALI" {
		t.Fatalf("channel filter leaked rows: %+v", orderNos(got))
	}
}

// TestListPendingCandidatesNullRetryMetadata covers the failure-injection
// path: rows whose TH-P1-05 metadata is entirely NULL remain selectable
// candidates without scan errors.
func TestListPendingCandidatesNullRetryMetadata(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)
	seedScannerOrder(t, repo, "DTPSCAN_NULLMETA", StatusPending, "epay", userID, time.Now().Add(-time.Hour))

	got, err := repo.ListPendingCandidates(ctx, time.Now(), 50, nil)
	if err != nil {
		t.Fatalf("ListPendingCandidates: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 candidate, got %+v", orderNos(got))
	}
	o := got[0]
	if o.QueryAttempts != nil || o.LastQueryAt != nil || o.NextRetryAt != nil || o.ReviewReason != nil {
		t.Fatalf("null metadata must scan as nil, got %+v", o)
	}
}

// TestListPendingCandidatesDoesNotMutate is the regression guard: running
// the candidate query leaves every row byte-identical (status, metadata,
// timestamps) — the scanner SELECTs and never writes.
func TestListPendingCandidatesDoesNotMutate(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	if repo.pool == nil {
		return // SetupPool skipped
	}
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "payment_orders", "users")
	userID := seedMetadataUser(t, repo.pool)
	seedScannerOrder(t, repo, "DTPSCAN_MUT1", StatusPending, "epay", userID, time.Now().Add(-time.Hour))

	before, err := repo.List(ctx, 50, 0, nil, nil)
	if err != nil {
		t.Fatalf("List before: %v", err)
	}
	if _, err := repo.ListPendingCandidates(ctx, time.Now(), 50, nil); err != nil {
		t.Fatalf("ListPendingCandidates: %v", err)
	}
	after, err := repo.List(ctx, 50, 0, nil, nil)
	if err != nil {
		t.Fatalf("List after: %v", err)
	}
	if len(before) != len(after) {
		t.Fatalf("row count changed: %d -> %d", len(before), len(after))
	}
	for i := range before {
		b, a := before[i], after[i]
		if b.OrderNo != a.OrderNo || b.Status != a.Status || !b.UpdatedAt.Equal(a.UpdatedAt) {
			t.Fatalf("row %s mutated by scan: before=%+v after=%+v", b.OrderNo, b, a)
		}
		if !equalMetadata(b, a) {
			t.Fatalf("row %s metadata mutated by scan", b.OrderNo)
		}
	}
}

func equalMetadata(b, a Order) bool {
	if (b.QueryAttempts == nil) != (a.QueryAttempts == nil) ||
		(b.LastQueryAt == nil) != (a.LastQueryAt == nil) ||
		(b.NextRetryAt == nil) != (a.NextRetryAt == nil) ||
		(b.ReviewReason == nil) != (a.ReviewReason == nil) {
		return false
	}
	if b.QueryAttempts != nil && *b.QueryAttempts != *a.QueryAttempts {
		return false
	}
	if b.ReviewReason != nil && *b.ReviewReason != *a.ReviewReason {
		return false
	}
	return true
}

func orderNos(orders []Order) []string {
	out := make([]string, 0, len(orders))
	for _, o := range orders {
		out = append(out, o.OrderNo)
	}
	return out
}
