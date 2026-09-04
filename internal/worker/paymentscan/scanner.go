// Package paymentscan implements the pending payment order scanner
// (TH-P1-CW-01). Each cycle selects a small, deterministic batch of pending
// orders old enough to be worth a provider query, then applies the
// worker-level eligibility rules (not expired, retry window reached) without
// ever writing: the scanner SELECTs candidates and never mutates orders or
// wallets. Consuming the eligible set (provider queries, retry scheduling,
// closure of expired orders) belongs to the follow-up workers (TH-P1-CW-02
// and TH-P1-CW-04), keeping this worker read-only by construction.
package paymentscan

import (
	"context"
	"fmt"
	"time"

	"github.com/deeptrols/api/internal/pkg/metrics"
	"github.com/deeptrols/api/internal/repository/paymentorder"
)

// Scanner tuning defaults. Batches stay small and the age threshold stays
// positive so a scanner fleet can never burst provider query APIs (the
// documented TH-P1-CW-01 risk).
const (
	DefaultMinAge = 60 * time.Second
	DefaultBatch  = 20
)

// candidateSource is the narrow persistence dependency of the scanner: the
// read-only candidate query. paymentorder.Repository satisfies it, and unit
// tests substitute a small fake.
type candidateSource interface {
	ListPendingCandidates(ctx context.Context, olderThan time.Time, limit int, channel *string) ([]paymentorder.Order, error)
}

// Scanner selects pending orders eligible for a provider query.
type Scanner struct {
	orders  candidateSource
	minAge  time.Duration // minimum order age before it becomes a candidate
	batch   int           // maximum candidates per cycle
	channel *string       // optional channel selector (nil = all channels)
	now     func() time.Time
}

// ScanResult summarizes one scanner cycle.
type ScanResult struct {
	Scanned  int                  // candidates examined by this cycle
	Eligible []paymentorder.Order // candidates that passed the eligibility rules
}

// New builds a Scanner with the small-batch production defaults.
func New(orders candidateSource) *Scanner {
	return &Scanner{
		orders: orders,
		minAge: DefaultMinAge,
		batch:  DefaultBatch,
		now:    time.Now,
	}
}

// eligible reports whether a pending candidate may be queried now: it must
// still be pending, not yet expired (expiry itself is exclusive), and any
// scheduled retry time (TH-P1-05 next_retry_at) must have passed.
func eligible(o paymentorder.Order, now time.Time) bool {
	if o.Status != paymentorder.StatusPending {
		return false
	}
	if !now.Before(o.ExpiresAt) {
		return false
	}
	if o.NextRetryAt != nil && o.NextRetryAt.After(now) {
		return false
	}
	return true
}

// Run executes one scan cycle: one candidate query against the repository,
// then the eligibility filter. It returns an error (and no result) when the
// candidate query fails, so the cycle is recorded failed and nothing is
// handed downstream. Run never writes.
func (s *Scanner) Run(ctx context.Context) (*ScanResult, error) {
	now := s.now()
	candidates, err := s.orders.ListPendingCandidates(ctx, now.Add(-s.minAge), s.batch, s.channel)
	if err != nil {
		return nil, fmt.Errorf("payment_scanner: list pending candidates: %w", err)
	}

	result := &ScanResult{Scanned: len(candidates), Eligible: []paymentorder.Order{}}
	scannedByChannel := map[string]int{}
	eligibleByChannel := map[string]int{}
	for _, o := range candidates {
		scannedByChannel[o.Channel]++
		if eligible(o, now) {
			result.Eligible = append(result.Eligible, o)
			eligibleByChannel[o.Channel]++
		}
	}
	for channel, n := range scannedByChannel {
		metrics.AddPaymentOrderScanned(channel, n)
	}
	for channel, n := range eligibleByChannel {
		metrics.AddPaymentOrderScanEligible(channel, n)
	}
	return result, nil
}
