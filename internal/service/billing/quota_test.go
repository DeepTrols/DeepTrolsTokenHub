package billing

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/quota"
	"github.com/google/uuid"
)

// ---------------------------------------------------------------------------
// mockQuotaRepo — mocks quota.Repository for QuotaChecker tests
// ---------------------------------------------------------------------------

type mockQuotaRepo struct {
	findAllocationFn func(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error)
	consumeFn        func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)
	restoreFn        func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)

	// call tracking
	consumeCalled int
	restoreCalled int
	lastConsumed  int64
	lastRestored  int64
}

func (m *mockQuotaRepo) FindAllocation(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error) {
	if m.findAllocationFn != nil {
		return m.findAllocationFn(ctx, userID, tenantID, modelID)
	}
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepo) Consume(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	m.consumeCalled++
	m.lastConsumed = amount
	if m.consumeFn != nil {
		return m.consumeFn(ctx, allocationID, amount, idempotencyKey)
	}
	return nil, nil
}

func (m *mockQuotaRepo) Restore(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	m.restoreCalled++
	m.lastRestored = amount
	if m.restoreFn != nil {
		return m.restoreFn(ctx, allocationID, amount, idempotencyKey)
	}
	return nil, nil
}

// The remaining interface methods are not exercised by QuotaChecker tests; they
// exist only to satisfy quota.Repository as the interface grows. Stub values
// keep the assertion compiling without dead behaviour to maintain.

func (m *mockQuotaRepo) FindPool(ctx context.Context, poolID uuid.UUID) (*domain.QuotaPool, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepo) FindPoolsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaPool, error) {
	return nil, nil
}

func (m *mockQuotaRepo) FindAllocationsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaAllocation, error) {
	return nil, nil
}

func (m *mockQuotaRepo) Allocate(ctx context.Context, poolID, userID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaAllocation, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepo) FindLedgerByAllocation(ctx context.Context, allocationID uuid.UUID, limit, offset int) ([]domain.QuotaLedgerEntry, error) {
	return nil, nil
}

func (m *mockQuotaRepo) UpdatePool(ctx context.Context, poolID uuid.UUID, totalAmount int64, unitName, dimension string) (*domain.QuotaPool, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepo) DeletePool(ctx context.Context, poolID uuid.UUID) error {
	return quota.ErrNotFound
}

var _ quota.Repository = (*mockQuotaRepo)(nil)

// ============================================================================
// Tests
// ============================================================================

func TestQuotaChecker_Check_AllocationHasRoom(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()
	allocID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              allocID,
				UserID:          uid,
				AllocatedAmount: 1000,
				UsedAmount:      200,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// Act
	result, err := checker.Check(ctx, userID, tenantID, modelID, 256)

	// Assert
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.AllocationID != allocID {
		t.Errorf("AllocationID = %s, want %s", result.AllocationID, allocID)
	}
	if !result.Allowed {
		t.Error("expected Allowed = true")
	}
}

func TestQuotaChecker_Check_QuotaExceeded(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 100,
				UsedAmount:      90,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// Act - estimated 256 tokens but only 10 remaining
	result, err := checker.Check(ctx, userID, tenantID, modelID, 256)

	// Assert
	if err != nil {
		t.Fatalf("Check returned unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Allowed {
		t.Error("expected Allowed = false (quota exceeded)")
	}
}

func TestQuotaChecker_Check_NoAllocation(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return nil, quota.ErrNotFound
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// Act
	result, err := checker.Check(ctx, userID, tenantID, modelID, 256)

	// Assert - no allocation means allowed (nil result, no error)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result when no allocation, got %+v", result)
	}
}

func TestQuotaChecker_Deduct_CallsConsume(t *testing.T) {
	// Arrange
	allocID := uuid.New()
	var capturedAmount int64
	var capturedAllocID uuid.UUID

	repo := &mockQuotaRepo{
		consumeFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			capturedAmount = amount
			capturedAllocID = allocationID
			return &domain.QuotaLedgerEntry{
				ID:           uuid.New(),
				AllocationID: allocationID,
				Action:       domain.QuotaActionConsume,
				Amount:       amount,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)

	// Act
	checker.Deduct(context.Background(), allocID, 500, "req-123")

	// Assert
	if repo.consumeCalled != 1 {
		t.Errorf("consumeCalled = %d, want 1", repo.consumeCalled)
	}
	if capturedAmount != 500 {
		t.Errorf("deducted amount = %d, want 500", capturedAmount)
	}
	if capturedAllocID != allocID {
		t.Errorf("deducted allocID = %s, want %s", capturedAllocID, allocID)
	}
}

func TestQuotaChecker_Deduct_DoesNotPanicOnError(t *testing.T) {
	// Arrange
	repo := &mockQuotaRepo{
		consumeFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return nil, errors.New("db connection lost")
		},
	}

	checker := NewQuotaChecker(repo)

	// Act - should not panic
	checker.Deduct(context.Background(), uuid.New(), 100, "req-error")
	// If we reach here without panicking, test passes.
}

func TestQuotaChecker_Restore_CallsRepoRestore(t *testing.T) {
	// Arrange
	allocID := uuid.New()
	repo := &mockQuotaRepo{
		restoreFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return &domain.QuotaLedgerEntry{
				ID:           uuid.New(),
				AllocationID: allocationID,
				Action:       domain.QuotaActionRestore,
				Amount:       amount,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)

	// Act
	checker.Restore(context.Background(), allocID, 300, "req-restore")

	// Assert
	if repo.restoreCalled != 1 {
		t.Errorf("restoreCalled = %d, want 1", repo.restoreCalled)
	}
}

func TestQuotaChecker_Restore_DoesNotPanicOnError(t *testing.T) {
	// Arrange
	repo := &mockQuotaRepo{
		restoreFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return nil, errors.New("db error")
		},
	}

	checker := NewQuotaChecker(repo)

	// Act - should not panic
	checker.Restore(context.Background(), uuid.New(), 100, "req-err")
}

func TestQuotaChecker_Check_ExactlyAtLimit(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 100,
				UsedAmount:      100,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	result, err := checker.Check(ctx, userID, tenantID, modelID, 1)

	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result.Allowed {
		t.Error("expected Allowed = false when remaining is 0")
	}
}

func TestQuotaChecker_Check_ZeroEstimatedTokens(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 100,
				UsedAmount:      0,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// Zero estimated tokens should still be allowed (at least 1 should be needed)
	result, err := checker.Check(ctx, userID, tenantID, modelID, 0)

	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.Allowed {
		t.Error("expected Allowed = true when estimatedTokens = 0")
	}
}

func TestQuotaChecker_Check_NegativeEstimatedTokens(t *testing.T) {
	// Arrange
	userID := uuid.New()
	tenantID := uuid.New()
	modelID := uuid.New()

	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 100,
				UsedAmount:      0,
			}, nil
		},
	}

	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// Negative estimated tokens should be allowed
	result, err := checker.Check(ctx, userID, tenantID, modelID, -1)

	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.Allowed {
		t.Error("expected Allowed = true when estimatedTokens is negative")
	}
}

func TestQuotaChecker_Reserve_SettlesAndReleases(t *testing.T) {
	allocID := uuid.New()
	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{ID: allocID, UserID: uid, AllocatedAmount: 1000, UsedAmount: 0}, nil
		},
	}
	checker := NewQuotaChecker(repo)
	ctx := context.Background()

	// 1. Reserve succeeds and atomically consumes the estimate.
	res, err := checker.Reserve(ctx, uuid.New(), uuid.New(), uuid.New(), 256, "req-1")
	if err != nil || res == nil {
		t.Fatalf("Reserve: res=%v err=%v", res, err)
	}
	if res.Insufficient || res.Reserved != 256 {
		t.Fatalf("unexpected reservation: %+v", res)
	}
	if repo.consumeCalled != 1 || repo.lastConsumed != 256 {
		t.Errorf("expected consume(256), got calls=%d amount=%d", repo.consumeCalled, repo.lastConsumed)
	}

	// 2. Settle with a smaller actual refunds the difference.
	checker.Settle(ctx, res, 200, "req-1")
	if repo.restoreCalled != 1 || repo.lastRestored != 56 {
		t.Errorf("expected restore(56), got calls=%d amount=%d", repo.restoreCalled, repo.lastRestored)
	}

	// 3. Settle with a larger actual consumes the extra.
	res2, _ := checker.Reserve(ctx, uuid.New(), uuid.New(), uuid.New(), 100, "req-2")
	checker.Settle(ctx, res2, 150, "req-2")
	if repo.lastConsumed != 50 {
		t.Errorf("expected extra consume(50), got %d", repo.lastConsumed)
	}

	// 4. Release returns the whole reservation on failure.
	res3, _ := checker.Reserve(ctx, uuid.New(), uuid.New(), uuid.New(), 100, "req-3")
	checker.Release(ctx, res3, "req-3")
	if repo.lastRestored != 100 {
		t.Errorf("expected release restore(100), got %d", repo.lastRestored)
	}
}

func TestQuotaChecker_Reserve_Insufficient(t *testing.T) {
	allocID := uuid.New()
	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{ID: allocID, UserID: uid, AllocatedAmount: 10, UsedAmount: 0}, nil
		},
		consumeFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return nil, quota.ErrInsufficientQuota
		},
	}
	checker := NewQuotaChecker(repo)
	res, err := checker.Reserve(context.Background(), uuid.New(), uuid.New(), uuid.New(), 256, "req-x")
	if err != nil {
		t.Fatalf("Reserve should not error on insufficient, got %v", err)
	}
	if res == nil || !res.Insufficient {
		t.Fatalf("expected Insufficient reservation, got %+v", res)
	}
}

func TestQuotaChecker_Reserve_NoAllocation_Unlimited(t *testing.T) {
	repo := &mockQuotaRepo{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return nil, quota.ErrNotFound
		},
	}
	checker := NewQuotaChecker(repo)
	res, err := checker.Reserve(context.Background(), uuid.New(), uuid.New(), uuid.New(), 256, "req-u")
	if err != nil || res != nil {
		t.Fatalf("expected (nil, nil) for unlimited account, got res=%v err=%v", res, err)
	}
}
