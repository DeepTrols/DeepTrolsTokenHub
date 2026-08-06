package billing

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/deeptrols/api/internal/repository/quota"
	"github.com/google/uuid"
)

// QuotaChecker enforces token quota limits at the gateway level.
// No allocation = allowed (nil result, no error). Only blocks when
// an allocation exists and its remaining quota is insufficient for
// the estimated usage.
type QuotaChecker struct {
	quotas quota.Repository
}

// NewQuotaChecker creates a new QuotaChecker.
func NewQuotaChecker(quotas quota.Repository) *QuotaChecker {
	return &QuotaChecker{quotas: quotas}
}

// QuotaReservation is the outcome of an atomic quota reservation.
// Insufficient=true means the estimated amount could NOT be reserved
// (caller should reject with 429).
type QuotaReservation struct {
	AllocationID uuid.UUID
	Reserved     int64
	Insufficient bool
}

// Reserve atomically reserves estimatedTokens against the user's allocation,
// eliminating the Check→Deduct TOCTOU race: the allocation is consumed (with
// row locks) at reservation time, so concurrent requests cannot overdraw.
// Returns (nil, nil) when no allocation exists (unlimited account).
func (q *QuotaChecker) Reserve(ctx context.Context, userID, tenantID, modelID uuid.UUID, estimatedTokens int64, requestID string) (*QuotaReservation, error) {
	if estimatedTokens <= 0 {
		return nil, nil
	}

	alloc, err := q.quotas.FindAllocation(ctx, userID, tenantID, modelID)
	if err != nil {
		if err == quota.ErrNotFound {
			return nil, nil // no allocation = unlimited
		}
		return nil, fmt.Errorf("quota reserve: %w", err)
	}

	idempotencyKey := fmt.Sprintf("reserve-%s-%s", alloc.ID, requestID)
	if _, err := q.quotas.Consume(ctx, alloc.ID, estimatedTokens, idempotencyKey); err != nil {
		if errors.Is(err, quota.ErrInsufficientQuota) {
			return &QuotaReservation{AllocationID: alloc.ID, Reserved: estimatedTokens, Insufficient: true}, nil
		}
		return nil, fmt.Errorf("quota reserve: %w", err)
	}
	return &QuotaReservation{AllocationID: alloc.ID, Reserved: estimatedTokens}, nil
}

// Settle reconciles a reservation against the ACTUAL tokens consumed:
// consumes the extra when actual > reserved, refunds the difference when
// actual < reserved. Best-effort; failures are logged.
func (q *QuotaChecker) Settle(ctx context.Context, res *QuotaReservation, actualTokens int64, requestID string) {
	if res == nil || actualTokens < 0 {
		return
	}
	switch {
	case actualTokens > res.Reserved:
		extra := actualTokens - res.Reserved
		key := fmt.Sprintf("settle-extra-%s-%s", res.AllocationID, requestID)
		if _, err := q.quotas.Consume(ctx, res.AllocationID, extra, key); err != nil {
			log.Printf("quota: Settle extra failed allocation=%s request=%s amount=%d: %v", res.AllocationID, requestID, extra, err)
		}
	case actualTokens < res.Reserved:
		refund := res.Reserved - actualTokens
		key := fmt.Sprintf("settle-refund-%s-%s", res.AllocationID, requestID)
		if _, err := q.quotas.Restore(ctx, res.AllocationID, refund, key); err != nil {
			log.Printf("quota: Settle refund failed allocation=%s request=%s amount=%d: %v", res.AllocationID, requestID, refund, err)
		}
	}
}

// Release returns the whole reservation on upstream failure.
// Best-effort; failures are logged.
func (q *QuotaChecker) Release(ctx context.Context, res *QuotaReservation, requestID string) {
	if res == nil {
		return
	}
	key := fmt.Sprintf("release-%s-%s", res.AllocationID, requestID)
	if _, err := q.quotas.Restore(ctx, res.AllocationID, res.Reserved, key); err != nil {
		log.Printf("quota: Release failed allocation=%s request=%s amount=%d: %v", res.AllocationID, requestID, res.Reserved, err)
	}
}

// ---------------------------------------------------------------------------
// Legacy read-only check + best-effort adjust. Kept for compatibility/tests;
// the gateway now uses Reserve/Settle/Release.
// ---------------------------------------------------------------------------

// QuotaCheckResult holds the outcome of a quota check.
type QuotaCheckResult struct {
	AllocationID uuid.UUID
	Allowed      bool
	Remaining    int64
}

// Check verifies that the user has sufficient quota remaining for the estimated
// token consumption. Returns (nil, nil) when no allocation exists for the
// user/tenant/model combination (no quota = allowed).
//
// Deprecated: read-only — see Reserve for the race-free reservation flow.
func (q *QuotaChecker) Check(ctx context.Context, userID, tenantID, modelID uuid.UUID, estimatedTokens int64) (*QuotaCheckResult, error) {
	if estimatedTokens <= 0 {
		// Allow zero/negative estimated tokens (no quota consumption risk).
		// Still return a result with Allowed=true and a nil AllocationID
		// so callers know allocation lookup was skipped.
		return &QuotaCheckResult{Allowed: true, Remaining: 0}, nil
	}

	alloc, err := q.quotas.FindAllocation(ctx, userID, tenantID, modelID)
	if err != nil {
		if err == quota.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("quota check: %w", err)
	}

	remaining := alloc.Remaining()
	allowed := remaining >= estimatedTokens

	return &QuotaCheckResult{
		AllocationID: alloc.ID,
		Allowed:      allowed,
		Remaining:    remaining,
	}, nil
}

// Deduct consumes quota after a successful upstream request.
// Best-effort: logs errors but does not propagate them.
// Callers should invoke this in a background goroutine with recover.
//
// Deprecated: use Reserve+Settle instead.
func (q *QuotaChecker) Deduct(ctx context.Context, allocationID uuid.UUID, amount int64, requestID string) {
	if allocationID == uuid.Nil || amount <= 0 {
		return
	}

	idempotencyKey := fmt.Sprintf("deduct-%s-%s", allocationID, requestID)
	if _, err := q.quotas.Consume(ctx, allocationID, amount, idempotencyKey); err != nil {
		log.Printf("quota: Deduct failed allocation=%s request=%s amount=%d: %v", allocationID, requestID, amount, err)
	}
}

// Restore returns quota after an upstream failure.
// Best-effort: logs errors but does not propagate them.
// Callers should invoke this in a background goroutine with recover.
//
// Deprecated: use Release instead.
func (q *QuotaChecker) Restore(ctx context.Context, allocationID uuid.UUID, amount int64, requestID string) {
	if allocationID == uuid.Nil || amount <= 0 {
		return
	}

	idempotencyKey := fmt.Sprintf("restore-%s-%s", allocationID, requestID)
	if _, err := q.quotas.Restore(ctx, allocationID, amount, idempotencyKey); err != nil {
		log.Printf("quota: Restore failed allocation=%s request=%s amount=%d: %v", allocationID, requestID, amount, err)
	}
}
