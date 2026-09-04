package paymentscan

// TH-P1-CW-01 — pending-order scanner unit tests: the worker-level
// eligibility rules and the Run filtering contract. Persistence-layer
// candidate selection (status / age / channel / limit / ordering) is covered
// by internal/repository/paymentorder/scanner_query_test.go; the scanner
// itself SELECTs candidates and never writes orders or wallets.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/paymentorder"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// fakeCandidates is the structural candidateSource double: it records the
// query parameters and hands back canned rows.
type fakeCandidates struct {
	orders []paymentorder.Order
	err    error

	calls        int
	gotOlderThan time.Time
	gotLimit     int
	gotChannel   *string
}

func (f *fakeCandidates) ListPendingCandidates(ctx context.Context, olderThan time.Time, limit int, channel *string) ([]paymentorder.Order, error) {
	f.calls++
	f.gotOlderThan = olderThan
	f.gotLimit = limit
	f.gotChannel = channel
	return f.orders, f.err
}

func scanOrder(orderNo, status string, expiresAt time.Time, nextRetryAt *time.Time) paymentorder.Order {
	return paymentorder.Order{
		ID:          uuid.New(),
		OrderNo:     orderNo,
		UserID:      uuid.New(),
		Amount:      decimal.NewFromInt(10),
		Currency:    "CNY",
		Purpose:     "topup",
		Channel:     "epay",
		PayMethod:   "alipay",
		Status:      status,
		ExpiresAt:   expiresAt,
		NextRetryAt: nextRetryAt,
	}
}

func timePtr(tm time.Time) *time.Time { return &tm }

// TestEligibleRules pins the worker-level eligibility rules (AC-01 pending
// due for a query, AC-02 terminal states excluded, AC-03 retry time not yet
// reached excluded).
func TestEligibleRules(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)
	cases := []struct {
		name  string
		order paymentorder.Order
		want  bool
	}{
		{"due pending without retry metadata is eligible", scanOrder("A", paymentorder.StatusPending, future, nil), true},
		{"pending with future retry time is not yet due (AC-03)", scanOrder("B", paymentorder.StatusPending, future, timePtr(future)), false},
		{"pending with retry time exactly now is due", scanOrder("C", paymentorder.StatusPending, future, timePtr(now)), true},
		{"pending with past retry time is due", scanOrder("D", paymentorder.StatusPending, future, timePtr(past)), true},
		{"expired pending is not eligible (AC-02)", scanOrder("E", paymentorder.StatusPending, past, nil), false},
		{"pending expiring exactly now is not eligible", scanOrder("F", paymentorder.StatusPending, now, nil), false},
		{"paid is never eligible (AC-02)", scanOrder("G", paymentorder.StatusPaid, future, nil), false},
		{"closed is never eligible (AC-02)", scanOrder("H", paymentorder.StatusClosed, future, nil), false},
		{"refunded is never eligible (AC-02)", scanOrder("I", paymentorder.StatusRefunded, future, nil), false},
	}
	for _, tc := range cases {
		if got := eligible(tc.order, now); got != tc.want {
			t.Errorf("%s: eligible = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestScannerRunFiltersAndPreservesOrder: Run returns only eligible
// candidates, preserves the repository's deterministic ordering, and reports
// the total number of candidates examined.
func TestScannerRunFiltersAndPreservesOrder(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	src := &fakeCandidates{orders: []paymentorder.Order{
		scanOrder("DTPSCAN_1", paymentorder.StatusPending, now.Add(time.Hour), nil),
		scanOrder("DTPSCAN_2", paymentorder.StatusPending, now.Add(time.Hour), timePtr(now.Add(time.Hour))), // retry in the future
		scanOrder("DTPSCAN_3", paymentorder.StatusPending, now.Add(-time.Second), nil),                      // expired
		scanOrder("DTPSCAN_4", paymentorder.StatusPaid, now.Add(time.Hour), nil),
		scanOrder("DTPSCAN_5", paymentorder.StatusPending, now.Add(time.Hour), timePtr(now.Add(-time.Second))), // retry window passed
	}}
	s := New(src)
	s.now = func() time.Time { return now }

	res, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Scanned != 5 {
		t.Fatalf("Scanned = %d, want 5", res.Scanned)
	}
	got := make([]string, 0, len(res.Eligible))
	for _, o := range res.Eligible {
		got = append(got, o.OrderNo)
	}
	want := []string{"DTPSCAN_1", "DTPSCAN_5"}
	if len(got) != len(want) {
		t.Fatalf("eligible = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("eligible[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

// TestScannerRunQueryParameters: the candidate query receives the age
// threshold (now − minAge), the batch limit and the channel selector
// unchanged, and runs exactly once per cycle.
func TestScannerRunQueryParameters(t *testing.T) {
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	src := &fakeCandidates{}
	channel := "alipay"
	s := &Scanner{orders: src, minAge: 90 * time.Second, batch: 7, channel: &channel, now: func() time.Time { return now }}

	if _, err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if src.calls != 1 {
		t.Fatalf("candidate query ran %d times, want exactly 1", src.calls)
	}
	wantOlder := now.Add(-90 * time.Second)
	if !src.gotOlderThan.Equal(wantOlder) {
		t.Errorf("olderThan = %v, want %v", src.gotOlderThan, wantOlder)
	}
	if src.gotLimit != 7 {
		t.Errorf("limit = %d, want 7", src.gotLimit)
	}
	if src.gotChannel == nil || *src.gotChannel != "alipay" {
		t.Errorf("channel selector not passed through: %v", src.gotChannel)
	}
}

// TestNewAppliesSmallBatchDefaults: the production constructor keeps batches
// small and the age threshold positive so the scanner can never overload
// provider query APIs (the documented TH-P1-CW-01 risk).
func TestNewAppliesSmallBatchDefaults(t *testing.T) {
	s := New(&fakeCandidates{})
	if s.batch <= 0 || s.batch > 100 {
		t.Errorf("default batch = %d, want a small positive batch size", s.batch)
	}
	if s.minAge <= 0 {
		t.Errorf("default minAge = %v, want a positive age threshold", s.minAge)
	}
	if s.channel != nil {
		t.Errorf("default channel selector = %v, want nil (all channels)", s.channel)
	}
	if s.now == nil {
		t.Error("default clock must be set")
	}
}

// TestScannerRunSourceErrorFailsClosed: a repository error surfaces as a Run
// error with no partial result — the cycle is recorded failed and no order
// is ever handed to a provider query stage.
func TestScannerRunSourceErrorFailsClosed(t *testing.T) {
	src := &fakeCandidates{err: errors.New("database unavailable")}
	s := New(src)

	res, err := s.Run(context.Background())
	if err == nil {
		t.Fatal("expected Run error, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result on error, got %+v", res)
	}
}

// TestScannerRunEmptyAndReadOnly: with no candidates Run succeeds with an
// empty result, and a scan never mutates the candidate rows it examines
// (worker-level complement of the repository does-not-mutate regression).
func TestScannerRunEmptyAndReadOnly(t *testing.T) {
	src := &fakeCandidates{}
	s := New(src)
	res, err := s.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Scanned != 0 || len(res.Eligible) != 0 {
		t.Fatalf("expected empty result, got %+v", res)
	}

	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	original := scanOrder("DTPSCAN_RO", paymentorder.StatusPending, now.Add(time.Hour), nil)
	src.orders = []paymentorder.Order{original}
	s.now = func() time.Time { return now }
	if _, err := s.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := src.orders[0]
	if got.Status != original.Status || !got.ExpiresAt.Equal(original.ExpiresAt) ||
		got.NextRetryAt != nil || got.QueryAttempts != nil || got.LastQueryAt != nil {
		t.Fatalf("scanner mutated the candidate row: before=%+v after=%+v", original, got)
	}
}
