package quota

import (
	"context"
	"fmt"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresRepository implements Repository.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// NewPostgresRepository creates a new PostgresRepository.
func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

var _ Repository = (*PostgresRepository)(nil)

// FindAllocation retrieves the most specific quota allocation for a user.
// Priority: model-scoped allocation > tenant-level (nil model) allocation.
func (r *PostgresRepository) FindAllocation(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error) {
	// 1. Try model-specific allocation first.
	const modelQuery = `
		SELECT a.id, a.pool_id, a.user_id, a.allocated_amount, a.used_amount, a.created_at, a.updated_at
		FROM quota_allocations a
		JOIN quota_pools p ON a.pool_id = p.id
		WHERE a.user_id = $1 AND p.tenant_id = $2 AND p.model_id = $3
		LIMIT 1
	`
	alloc, err := scanAllocation(r.pool.QueryRow(ctx, modelQuery, userID, tenantID, modelID))
	if err == nil {
		return alloc, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("quota find allocation (model): %w", err)
	}

	// 2. Fallback to global (tenant-level) allocation with NULL model_id.
	const globalQuery = `
		SELECT a.id, a.pool_id, a.user_id, a.allocated_amount, a.used_amount, a.created_at, a.updated_at
		FROM quota_allocations a
		JOIN quota_pools p ON a.pool_id = p.id
		WHERE a.user_id = $1 AND p.tenant_id = $2 AND p.model_id IS NULL
		LIMIT 1
	`
	alloc, err = scanAllocation(r.pool.QueryRow(ctx, globalQuery, userID, tenantID))
	if err == nil {
		return alloc, nil
	}
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	return nil, fmt.Errorf("quota find allocation (global): %w", err)
}

// Consume atomically deducts quota from an allocation.
func (r *PostgresRepository) Consume(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("quota consume begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Check for existing idempotent ledger entry.
	existing, err := findQuotaLedgerEntry(ctx, tx, idempotencyKey)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("quota consume commit (idempotent): %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("quota consume idempotent check: %w", err)
	}

	// 2. Lock the allocation and pool rows for update.
	var poolID uuid.UUID
	var allocatedAmount, usedAmount int64
	const lockQuery = `
		SELECT a.pool_id, a.allocated_amount, a.used_amount
		FROM quota_allocations a
		WHERE a.id = $1
		FOR UPDATE
	`
	if err := tx.QueryRow(ctx, lockQuery, allocationID).Scan(&poolID, &allocatedAmount, &usedAmount); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("quota consume: allocation not found: %w", err)
		}
		return nil, fmt.Errorf("quota consume lock: %w", err)
	}

	// 3. Check remaining quota.
	remaining := allocatedAmount - usedAmount
	if remaining < amount {
		return nil, fmt.Errorf("quota consume: %w: remaining=%d required=%d", ErrInsufficientQuota, remaining, amount)
	}

	// 4. Update allocation used_amount.
	newUsed := usedAmount + amount
	const updateAlloc = `
		UPDATE quota_allocations SET used_amount = $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updateAlloc, newUsed, allocationID); err != nil {
		return nil, fmt.Errorf("quota consume update allocation: %w", err)
	}

	// 5. Update pool used_amount.
	const updatePool = `
		UPDATE quota_pools SET used_amount = used_amount + $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updatePool, amount, poolID); err != nil {
		return nil, fmt.Errorf("quota consume update pool: %w", err)
	}

	// 6. Insert ledger entry.
	entryID := uuid.New()
	const insertLedger = `
		INSERT INTO quota_ledger (id, allocation_id, idempotency_key, action, amount, balance_after, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	if _, err := tx.Exec(ctx, insertLedger, entryID, allocationID, idempotencyKey, string(domain.QuotaActionConsume), amount, newUsed); err != nil {
		return nil, fmt.Errorf("quota consume insert ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("quota consume commit: %w", err)
	}

	return &domain.QuotaLedgerEntry{
		ID:             entryID,
		AllocationID:   allocationID,
		IdempotencyKey: idempotencyKey,
		Action:         domain.QuotaActionConsume,
		Amount:         amount,
		BalanceAfter:   newUsed,
	}, nil
}

// Restore returns consumed quota back to the allocation.
func (r *PostgresRepository) Restore(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("quota restore begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Check for existing idempotent ledger entry.
	existing, err := findQuotaLedgerEntry(ctx, tx, idempotencyKey)
	if err == nil {
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("quota restore commit (idempotent): %w", cerr)
		}
		return existing, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("quota restore idempotent check: %w", err)
	}

	// 2. Lock the allocation and pool rows for update.
	var poolID uuid.UUID
	var usedAmount int64
	const lockQuery = `
		SELECT a.pool_id, a.used_amount
		FROM quota_allocations a
		WHERE a.id = $1
		FOR UPDATE
	`
	if err := tx.QueryRow(ctx, lockQuery, allocationID).Scan(&poolID, &usedAmount); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("quota restore: allocation not found: %w", err)
		}
		return nil, fmt.Errorf("quota restore lock: %w", err)
	}

	// 3. Do not restore more than was used.
	restoreAmount := amount
	if restoreAmount > usedAmount {
		restoreAmount = usedAmount
	}

	newUsed := usedAmount - restoreAmount

	// 4. Update allocation used_amount.
	const updateAlloc = `
		UPDATE quota_allocations SET used_amount = $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updateAlloc, newUsed, allocationID); err != nil {
		return nil, fmt.Errorf("quota restore update allocation: %w", err)
	}

	// 5. Update pool used_amount.
	const updatePool = `
		UPDATE quota_pools SET used_amount = used_amount - $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updatePool, restoreAmount, poolID); err != nil {
		return nil, fmt.Errorf("quota restore update pool: %w", err)
	}

	// 6. Insert ledger entry.
	entryID := uuid.New()
	const insertLedger = `
		INSERT INTO quota_ledger (id, allocation_id, idempotency_key, action, amount, balance_after, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	if _, err := tx.Exec(ctx, insertLedger, entryID, allocationID, idempotencyKey, string(domain.QuotaActionRestore), restoreAmount, newUsed); err != nil {
		return nil, fmt.Errorf("quota restore insert ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("quota restore commit: %w", err)
	}

	return &domain.QuotaLedgerEntry{
		ID:             entryID,
		AllocationID:   allocationID,
		IdempotencyKey: idempotencyKey,
		Action:         domain.QuotaActionRestore,
		Amount:         restoreAmount,
		BalanceAfter:   newUsed,
	}, nil
}

// ---------------------------------------------------------------------------
// enterprise admin (team) management
// ---------------------------------------------------------------------------

// FindPool retrieves a single quota pool by ID.
func (r *PostgresRepository) FindPool(ctx context.Context, poolID uuid.UUID) (*domain.QuotaPool, error) {
	const query = `
		SELECT id, tenant_id, model_id, dimension, total_amount,
		       allocated_amount, used_amount, unit_name, created_at, updated_at
		FROM quota_pools
		WHERE id = $1
	`
	var p domain.QuotaPool
	err := r.pool.QueryRow(ctx, query, poolID).Scan(
		&p.ID, &p.TenantID, &p.ModelID, &p.Dimension, &p.TotalAmount,
		&p.AllocatedAmount, &p.UsedAmount, &p.UnitName, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("quota find pool: %w", err)
	}
	return &p, nil
}

// FindPoolsByTenant lists all quota pools owned by a tenant.
func (r *PostgresRepository) FindPoolsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaPool, error) {
	const query = `
		SELECT id, tenant_id, model_id, dimension, total_amount,
		       allocated_amount, used_amount, unit_name, created_at, updated_at
		FROM quota_pools
		WHERE tenant_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quota list pools by tenant: %w", err)
	}
	defer rows.Close()

	var pools []domain.QuotaPool
	for rows.Next() {
		var p domain.QuotaPool
		if err := rows.Scan(
			&p.ID, &p.TenantID, &p.ModelID, &p.Dimension, &p.TotalAmount,
			&p.AllocatedAmount, &p.UsedAmount, &p.UnitName, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("quota scan pool: %w", err)
		}
		pools = append(pools, p)
	}
	return pools, rows.Err()
}

// FindAllocationsByTenant lists all allocations across a tenant's pools.
func (r *PostgresRepository) FindAllocationsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaAllocation, error) {
	const query = `
		SELECT a.id, a.pool_id, a.user_id, a.allocated_amount, a.used_amount,
		       a.created_at, a.updated_at
		FROM quota_allocations a
		JOIN quota_pools p ON a.pool_id = p.id
		WHERE p.tenant_id = $1
		ORDER BY a.created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, fmt.Errorf("quota list allocations by tenant: %w", err)
	}
	defer rows.Close()

	var allocations []domain.QuotaAllocation
	for rows.Next() {
		var a domain.QuotaAllocation
		if err := rows.Scan(&a.ID, &a.PoolID, &a.UserID, &a.AllocatedAmount, &a.UsedAmount, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("quota scan allocation: %w", err)
		}
		allocations = append(allocations, a)
	}
	return allocations, rows.Err()
}

// Allocate atomically grants or increases a user's allocation inside a pool.
func (r *PostgresRepository) Allocate(ctx context.Context, poolID, userID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaAllocation, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("quota allocate begin: %w", err)
	}
	defer tx.Rollback(ctx)

	// 1. Idempotent replay: a retried allocation returns the already-recorded
	// result instead of granting the quota a second time.
	existing, err := findQuotaLedgerEntry(ctx, tx, idempotencyKey)
	if err == nil {
		alloc, aerr := findAllocationByIDTx(ctx, tx, existing.AllocationID)
		if aerr != nil {
			return nil, aerr
		}
		if cerr := tx.Commit(ctx); cerr != nil {
			return nil, fmt.Errorf("quota allocate commit (idempotent): %w", cerr)
		}
		return alloc, nil
	}
	if err != pgx.ErrNoRows {
		return nil, fmt.Errorf("quota allocate idempotent check: %w", err)
	}

	// 2. Lock the pool row and enforce the "allocated + amount <= total" bound.
	var totalAmount, allocatedAmount int64
	const lockPool = `
		SELECT total_amount, allocated_amount
		FROM quota_pools
		WHERE id = $1
		FOR UPDATE
	`
	if err := tx.QueryRow(ctx, lockPool, poolID).Scan(&totalAmount, &allocatedAmount); err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("quota allocate lock pool: %w", err)
	}
	if allocatedAmount+amount > totalAmount {
		return nil, fmt.Errorf("quota allocate: %w: remaining=%d required=%d", ErrInsufficientQuota, totalAmount-allocatedAmount, amount)
	}

	// 3. Upsert the allocation on (pool_id, user_id).
	allocID := uuid.New()
	const upsertAllocation = `
		INSERT INTO quota_allocations (id, pool_id, user_id, allocated_amount, used_amount, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 0, NOW(), NOW())
		ON CONFLICT (pool_id, user_id) DO UPDATE
			SET allocated_amount = quota_allocations.allocated_amount + EXCLUDED.allocated_amount,
			    updated_at = NOW()
		RETURNING id, pool_id, user_id, allocated_amount, used_amount, created_at, updated_at
	`
	var alloc domain.QuotaAllocation
	if err := tx.QueryRow(ctx, upsertAllocation, allocID, poolID, userID, amount).Scan(
		&alloc.ID, &alloc.PoolID, &alloc.UserID, &alloc.AllocatedAmount, &alloc.UsedAmount, &alloc.CreatedAt, &alloc.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("quota allocate upsert: %w", err)
	}

	// 4. Increment the pool allocated counter.
	const updatePool = `
		UPDATE quota_pools SET allocated_amount = allocated_amount + $1, updated_at = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, updatePool, amount, poolID); err != nil {
		return nil, fmt.Errorf("quota allocate update pool: %w", err)
	}

	// 5. Record the audit trail.
	const insertLedger = `
		INSERT INTO quota_ledger (id, allocation_id, idempotency_key, action, amount, balance_after, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	if _, err := tx.Exec(ctx, insertLedger, uuid.New(), alloc.ID, idempotencyKey, string(domain.QuotaActionAllocate), amount, alloc.AllocatedAmount); err != nil {
		return nil, fmt.Errorf("quota allocate insert ledger: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("quota allocate commit: %w", err)
	}
	return &alloc, nil
}

// FindLedgerByAllocation returns the ledger entries for an allocation, newest
// first, bounded by limit/offset.
func (r *PostgresRepository) FindLedgerByAllocation(ctx context.Context, allocationID uuid.UUID, limit, offset int) ([]domain.QuotaLedgerEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	const query = `
		SELECT id, allocation_id, idempotency_key, action, amount, balance_after, reference_id, created_at
		FROM quota_ledger
		WHERE allocation_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, allocationID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("quota list ledger: %w", err)
	}
	defer rows.Close()

	var entries []domain.QuotaLedgerEntry
	for rows.Next() {
		var entry domain.QuotaLedgerEntry
		var action string
		var refID *uuid.UUID
		if err := rows.Scan(&entry.ID, &entry.AllocationID, &entry.IdempotencyKey, &action, &entry.Amount, &entry.BalanceAfter, &refID, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("quota scan ledger: %w", err)
		}
		entry.Action = domain.QuotaLedgerAction(action)
		entry.ReferenceID = refID
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// findAllocationByIDTx loads one allocation inside an existing transaction.
func findAllocationByIDTx(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*domain.QuotaAllocation, error) {
	const query = `
		SELECT id, pool_id, user_id, allocated_amount, used_amount, created_at, updated_at
		FROM quota_allocations
		WHERE id = $1
	`
	var a domain.QuotaAllocation
	if err := tx.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.PoolID, &a.UserID, &a.AllocatedAmount, &a.UsedAmount, &a.CreatedAt, &a.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("quota find allocation by id: %w", err)
	}
	return &a, nil
}

func scanAllocation(row pgx.Row) (*domain.QuotaAllocation, error) {
	var a domain.QuotaAllocation
	err := row.Scan(&a.ID, &a.PoolID, &a.UserID, &a.AllocatedAmount, &a.UsedAmount, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func findQuotaLedgerEntry(ctx context.Context, tx pgx.Tx, key string) (*domain.QuotaLedgerEntry, error) {
	const query = `
		SELECT id, allocation_id, idempotency_key, action, amount, balance_after, reference_id, created_at
		FROM quota_ledger
		WHERE idempotency_key = $1
	`
	row := tx.QueryRow(ctx, query, key)
	var entry domain.QuotaLedgerEntry
	var action string
	var refID *uuid.UUID
	err := row.Scan(&entry.ID, &entry.AllocationID, &entry.IdempotencyKey, &action, &entry.Amount, &entry.BalanceAfter, &refID, &entry.CreatedAt)
	if err != nil {
		return nil, err
	}
	entry.Action = domain.QuotaLedgerAction(action)
	entry.ReferenceID = refID
	return &entry, nil
}
