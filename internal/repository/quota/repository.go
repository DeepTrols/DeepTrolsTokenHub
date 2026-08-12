package quota

import (
	"context"
	"errors"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

// ErrNotFound is returned when the requested quota entity (allocation, pool,
// or ledger row) does not exist.
var ErrNotFound = errors.New("quota: not found")

// ErrInsufficientQuota is returned when the remaining quota is insufficient.
var ErrInsufficientQuota = errors.New("quota: insufficient remaining quota")

// ErrConstraintViolation is returned when an update would violate a business
// invariant, e.g. shrinking a pool's total below what is already allocated.
var ErrConstraintViolation = errors.New("quota: update violates business constraint")

// Repository defines the data access interface for quota operations.
type Repository interface {
	// FindAllocation retrieves the most specific quota allocation for a user.
	// Priority: model-scoped allocation > tenant-level (nil model) allocation.
	// Returns ErrNotFound if no allocation exists.
	FindAllocation(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error)

	// Consume atomically deducts quota from an allocation with idempotency support.
	// Uses SELECT ... FOR UPDATE to prevent race conditions.
	// Returns ErrInsufficientQuota when remaining < amount.
	// Returns the ledger entry that was created (or the existing one if idempotent).
	Consume(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)

	// Restore returns consumed quota back to the allocation.
	// Best-effort: logs errors in the caller but does not fail the main flow.
	Restore(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)

	// --- Enterprise admin (team) management ---

	// FindPool retrieves a single quota pool. Returns ErrNotFound when the pool
	// does not exist.
	FindPool(ctx context.Context, poolID uuid.UUID) (*domain.QuotaPool, error)

	// FindPoolsByTenant lists all quota pools owned by a tenant.
	FindPoolsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaPool, error)

	// FindAllocationsByTenant lists all allocations across a tenant's pools.
	FindAllocationsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaAllocation, error)

	// Allocate atomically grants or increases a user's allocation inside a pool.
	// The pool row is locked, capacity is enforced (pool.allocated + amount <=
	// pool.total), the allocation is upserted on (pool_id, user_id), the pool
	// allocated counter is incremented, and an `allocate` ledger entry is
	// written. Returns ErrInsufficientQuota when the pool has no headroom and
	// ErrNotFound when the pool does not exist.
	Allocate(ctx context.Context, poolID, userID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaAllocation, error)

	// FindLedgerByAllocation returns the ledger entries for an allocation, newest
	// first, bounded by limit/offset.
	FindLedgerByAllocation(ctx context.Context, allocationID uuid.UUID, limit, offset int) ([]domain.QuotaLedgerEntry, error)

	// UpdatePool modifies the editable fields of a quota pool. Returns
	// ErrNotFound when the pool does not exist and ErrConstraintViolation when
	// the new total_amount would drop below the currently allocated amount.
	UpdatePool(ctx context.Context, poolID uuid.UUID, totalAmount int64, unitName, dimension string) (*domain.QuotaPool, error)

	// DeletePool permanently removes a quota pool and its allocations and ledger
	// entries in one transaction (leaf-first cascade). Returns ErrNotFound when
	// the pool does not exist.
	DeletePool(ctx context.Context, poolID uuid.UUID) error
}
