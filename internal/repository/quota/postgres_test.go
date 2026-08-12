package quota

import (
	"context"
	"errors"
	"testing"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func seedQuotaUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@test.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

func seedQuotaTenant(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	code := "quota-" + uuid.New().String()[:8]
	_, err := repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, code, code+" name")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenantID
}

func seedQuotaModel(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	modelID := uuid.New()
	code := "quota-model-" + uuid.New().String()[:8]
	_, err := repo.pool.Exec(ctx, `INSERT INTO models (id, code, provider, category) VALUES ($1, $2, 'openai', 'chat')`,
		modelID, code)
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}
	return modelID
}

// seedAllocation creates a quota pool and allocation, returns the allocation ID.
func seedAllocation(t *testing.T, ctx context.Context, repo *PostgresRepository, userID, tenantID uuid.UUID, modelID *uuid.UUID, allocatedAmount int64) (poolID, allocID uuid.UUID) {
	t.Helper()
	poolID = uuid.New()
	allocID = uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO quota_pools (id, tenant_id, model_id, dimension, total_amount, allocated_amount, unit_name)
		VALUES ($1, $2, $3, 'token', $4, $5, 'token')
	`, poolID, tenantID, modelID, allocatedAmount, allocatedAmount)
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	_, err = repo.pool.Exec(ctx, `
		INSERT INTO quota_allocations (id, pool_id, user_id, allocated_amount, used_amount)
		VALUES ($1, $2, $3, $4, 0)
	`, allocID, poolID, userID, allocatedAmount)
	if err != nil {
		t.Fatalf("seed allocation: %v", err)
	}
	return poolID, allocID
}

// ============================================================================
// Tests
// ============================================================================

func TestFindAllocation_ModelSpecific(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	modelID := seedQuotaModel(t, ctx, repo)

	// Create a model-specific allocation
	poolID, allocID := seedAllocation(t, ctx, repo, userID, tenantID, &modelID, 100000)

	found, err := repo.FindAllocation(ctx, userID, tenantID, modelID)
	if err != nil {
		t.Fatalf("FindAllocation: %v", err)
	}
	if found.ID != allocID {
		t.Errorf("Allocation ID = %s, want %s", found.ID, allocID)
	}
	if found.PoolID != poolID {
		t.Errorf("PoolID = %s, want %s", found.PoolID, poolID)
	}
	if found.AllocatedAmount != 100000 {
		t.Errorf("AllocatedAmount = %d, want %d", found.AllocatedAmount, 100000)
	}
	if found.UsedAmount != 0 {
		t.Errorf("UsedAmount = %d, want 0", found.UsedAmount)
	}
	if found.Remaining() != 100000 {
		t.Errorf("Remaining = %d, want %d", found.Remaining(), 100000)
	}
}

func TestFindAllocation_FallbackToGlobal(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	specificModelID := seedQuotaModel(t, ctx, repo)

	// Create a global (tenant-level) allocation with nil model_id
	poolID, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 50000)

	// Look up with a specific model ID that has no allocation — should fallback to global
	found, err := repo.FindAllocation(ctx, userID, tenantID, specificModelID)
	if err != nil {
		t.Fatalf("FindAllocation fallback: %v", err)
	}
	if found.ID != allocID {
		t.Errorf("Allocation ID = %s, want %s (global fallback)", found.ID, allocID)
	}
	if found.PoolID != poolID {
		t.Errorf("PoolID = %s, want %s", found.PoolID, poolID)
	}
}

func TestFindAllocation_ModelSpecificWinsOverGlobal(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	modelID := seedQuotaModel(t, ctx, repo)

	// Create global allocation
	_, globalAllocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 50000)

	// Create model-specific allocation
	_, modelAllocID := seedAllocation(t, ctx, repo, userID, tenantID, &modelID, 100000)

	// Model-specific should win
	found, err := repo.FindAllocation(ctx, userID, tenantID, modelID)
	if err != nil {
		t.Fatalf("FindAllocation: %v", err)
	}
	if found.ID != modelAllocID {
		t.Errorf("Allocation ID = %s, want %s (model-specific wins)", found.ID, modelAllocID)
	}
	_ = globalAllocID
}

func TestFindAllocation_ErrNotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	modelID := seedQuotaModel(t, ctx, repo)

	_, err := repo.FindAllocation(ctx, userID, tenantID, modelID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestConsume_Success(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 100000)

	// Consume 5000 tokens
	entry, err := repo.Consume(ctx, allocID, 5000, "idem-consume-test-1")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}
	if entry.Action != domain.QuotaActionConsume {
		t.Errorf("Action = %s, want %s", entry.Action, domain.QuotaActionConsume)
	}
	if entry.Amount != 5000 {
		t.Errorf("Amount = %d, want 5000", entry.Amount)
	}

	// Verify allocation state
	var usedAmount int64
	err = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	if err != nil {
		t.Fatalf("query allocation: %v", err)
	}
	if usedAmount != 5000 {
		t.Errorf("used_amount = %d, want 5000", usedAmount)
	}

	// Verify pool state
	var poolID uuid.UUID
	var poolUsedAmount int64
	err = repo.pool.QueryRow(ctx,
		`SELECT p.id, p.used_amount FROM quota_pools p
		 JOIN quota_allocations a ON a.pool_id = p.id
		 WHERE a.id = $1`, allocID).Scan(&poolID, &poolUsedAmount)
	if err != nil {
		t.Fatalf("query pool: %v", err)
	}
	if poolUsedAmount != 5000 {
		t.Errorf("pool used_amount = %d, want 5000", poolUsedAmount)
	}
}

func TestConsume_InsufficientQuota(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 100)

	_, err := repo.Consume(ctx, allocID, 200, "idem-consume-exceed")
	if !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("expected ErrInsufficientQuota, got: %v", err)
	}
}

func TestConsume_Idempotent(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 100000)

	// First consume
	entry1, err := repo.Consume(ctx, allocID, 100, "idem-dup")
	if err != nil {
		t.Fatalf("first Consume: %v", err)
	}

	// Second consume with same idempotency key (different amount!) should return existing entry
	entry2, err := repo.Consume(ctx, allocID, 99999, "idem-dup")
	if err != nil {
		t.Fatalf("second Consume (idempotent): %v", err)
	}
	if entry1.ID != entry2.ID {
		t.Errorf("expected same ledger entry for idempotent call, got %s vs %s", entry1.ID, entry2.ID)
	}
	if entry2.Amount != 100 {
		t.Errorf("Amount = %d, want 100 (original amount)", entry2.Amount)
	}

	// used_amount should still be 100 (not deducted twice)
	var usedAmount int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	if usedAmount != 100 {
		t.Errorf("used_amount = %d, want 100 (idempotent)", usedAmount)
	}
}

func TestRestore_Success(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 100000)

	// Consume first
	_, err := repo.Consume(ctx, allocID, 5000, "idem-restore-consume")
	if err != nil {
		t.Fatalf("Consume before restore: %v", err)
	}

	// Restore
	entry, err := repo.Restore(ctx, allocID, 5000, "idem-restore-1")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if entry.Action != domain.QuotaActionRestore {
		t.Errorf("Action = %s, want %s", entry.Action, domain.QuotaActionRestore)
	}

	// Verify allocation state: used_amount back to 0
	var usedAmount int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	if usedAmount != 0 {
		t.Errorf("used_amount = %d, want 0 after restore", usedAmount)
	}
}

func TestRestore_Idempotent(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 100000)

	_, _ = repo.Consume(ctx, allocID, 100, "idem-restore-consume-2")

	// First restore
	entry1, err := repo.Restore(ctx, allocID, 100, "idem-restore-dup")
	if err != nil {
		t.Fatalf("first Restore: %v", err)
	}

	// Second restore with same key
	entry2, err := repo.Restore(ctx, allocID, 100, "idem-restore-dup")
	if err != nil {
		t.Fatalf("second Restore (idempotent): %v", err)
	}
	if entry1.ID != entry2.ID {
		t.Errorf("expected same ledger entry for idempotent restore, got %s vs %s", entry1.ID, entry2.ID)
	}

	// used_amount should be back to 0 only once
	var usedAmount int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	if usedAmount != 0 {
		t.Errorf("used_amount = %d, want 0", usedAmount)
	}
}

func TestConsume_Concurrent(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 10000)

	// Run 5 concurrent consumes of 10 each (5*10=50, well within 10000)
	concurrency := 5
	errs := make(chan error, concurrency)
	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			_, err := repo.Consume(ctx, allocID, 10, "idem-concurrent-"+uuid.New().String()[:8])
			errs <- err
		}(i)
	}

	failures := 0
	for i := 0; i < concurrency; i++ {
		if err := <-errs; err != nil {
			t.Logf("concurrent consume %d failed: %v", i, err)
			failures++
		}
	}

	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}

	var usedAmount int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	expectedAmount := int64(concurrency * 10)
	if usedAmount != expectedAmount {
		t.Errorf("used_amount = %d, want %d (%d concurrent * 10)", usedAmount, expectedAmount, concurrency)
	}
}

func TestFindAllocation_NoAllocationForUserButExistsForAnother(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userA := seedQuotaUser(t, ctx, repo.pool)
	userB := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)

	// Allocate to user A but not user B
	seedAllocation(t, ctx, repo, userA, tenantID, nil, 100000)

	// user B should get ErrNotFound
	_, err := repo.FindAllocation(ctx, userB, tenantID, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for user B, got: %v", err)
	}
}

func TestRestore_CannotGoBelowZero(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	_, allocID := seedAllocation(t, ctx, repo, userID, tenantID, nil, 1000)

	// Consume 100
	_, _ = repo.Consume(ctx, allocID, 100, "idem-restore-clamp-consume")

	// Try to restore more than was consumed (should clamp to 100)
	_, err := repo.Restore(ctx, allocID, 999, "idem-restore-clamp-1")
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	// used_amount should be 0 (clamped, not negative)
	var usedAmount int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_allocations WHERE id = $1`, allocID).Scan(&usedAmount)
	if usedAmount != 0 {
		t.Errorf("used_amount = %d, want 0 (clamped)", usedAmount)
	}
}

func TestConsume_OnNonExistentAllocation(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	_, err := repo.Consume(ctx, uuid.New(), 100, "idem-nonexistent")
	if err == nil {
		t.Error("expected error when consuming on non-existent allocation")
	}
}

// seedQuotaPool inserts a quota pool with full headroom (allocated = 0).
func seedQuotaPool(t *testing.T, ctx context.Context, repo *PostgresRepository, tenantID uuid.UUID, total int64) uuid.UUID {
	t.Helper()
	poolID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO quota_pools (id, tenant_id, dimension, total_amount, allocated_amount, used_amount, unit_name)
		VALUES ($1, $2, 'token', $3, 0, 0, 'token')
	`, poolID, tenantID, total)
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	return poolID
}

var truncateQuota = []string{
	"quota_ledger", "quota_allocations", "quota_pools",
	"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
}

func TestFindPool_NotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	_, err := repo.FindPool(ctx, uuid.New())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindPool_Success(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	pool, err := repo.FindPool(ctx, poolID)
	if err != nil {
		t.Fatalf("FindPool: %v", err)
	}
	if pool.TenantID != tenantID {
		t.Errorf("TenantID = %s, want %s", pool.TenantID, tenantID)
	}
	if pool.TotalAmount != 100000 {
		t.Errorf("TotalAmount = %d, want 100000", pool.TotalAmount)
	}
}

func TestFindPoolsByTenant_Scoped(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	tenantA := seedQuotaTenant(t, ctx, repo)
	tenantB := seedQuotaTenant(t, ctx, repo)
	seedQuotaPool(t, ctx, repo, tenantA, 100000)
	seedQuotaPool(t, ctx, repo, tenantA, 200000)
	seedQuotaPool(t, ctx, repo, tenantB, 300000)

	pools, err := repo.FindPoolsByTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("FindPoolsByTenant: %v", err)
	}
	if len(pools) != 2 {
		t.Fatalf("len = %d, want 2 (tenant A only)", len(pools))
	}
	for _, p := range pools {
		if p.TenantID != tenantA {
			t.Errorf("pool %s belongs to tenant %s, want tenant A", p.ID, p.TenantID)
		}
	}
}

func TestAllocate_NewAllocation(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	alloc, err := repo.Allocate(ctx, poolID, userID, 30000, "idem-alloc-new")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if alloc.AllocatedAmount != 30000 {
		t.Errorf("AllocatedAmount = %d, want 30000", alloc.AllocatedAmount)
	}
	if alloc.UsedAmount != 0 {
		t.Errorf("UsedAmount = %d, want 0", alloc.UsedAmount)
	}
	if alloc.Remaining() != 30000 {
		t.Errorf("Remaining = %d, want 30000", alloc.Remaining())
	}

	// Pool allocated counter advanced by the full amount.
	var poolAllocated int64
	_ = repo.pool.QueryRow(ctx, `SELECT allocated_amount FROM quota_pools WHERE id = $1`, poolID).Scan(&poolAllocated)
	if poolAllocated != 30000 {
		t.Errorf("pool allocated_amount = %d, want 30000", poolAllocated)
	}

	// Audit trail has one allocate entry.
	entries, err := repo.FindLedgerByAllocation(ctx, alloc.ID, 100, 0)
	if err != nil {
		t.Fatalf("FindLedgerByAllocation: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ledger len = %d, want 1", len(entries))
	}
	if entries[0].Action != domain.QuotaActionAllocate || entries[0].Amount != 30000 {
		t.Errorf("entry = %+v, want action=allocate amount=30000", entries[0])
	}
}

func TestAllocate_IncreaseExisting(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	if _, err := repo.Allocate(ctx, poolID, userID, 30000, "idem-alloc-inc-1"); err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	alloc, err := repo.Allocate(ctx, poolID, userID, 20000, "idem-alloc-inc-2")
	if err != nil {
		t.Fatalf("second Allocate: %v", err)
	}
	if alloc.AllocatedAmount != 50000 {
		t.Errorf("AllocatedAmount = %d, want 50000 (cumulative)", alloc.AllocatedAmount)
	}

	// A second allocate writes a second ledger row but only one allocation row.
	var allocCount int
	_ = repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM quota_allocations WHERE pool_id = $1 AND user_id = $2`, poolID, userID).Scan(&allocCount)
	if allocCount != 1 {
		t.Errorf("allocation rows = %d, want 1 (upsert not insert)", allocCount)
	}
}

func TestAllocate_InsufficientCapacity(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 10000)

	if _, err := repo.Allocate(ctx, poolID, userID, 5000, "idem-alloc-ok"); err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	if _, err := repo.Allocate(ctx, poolID, userID, 6000, "idem-alloc-over"); !errors.Is(err, ErrInsufficientQuota) {
		t.Fatalf("expected ErrInsufficientQuota, got: %v", err)
	}

	// The failed allocation must not have moved any counter.
	var poolAllocated int64
	_ = repo.pool.QueryRow(ctx, `SELECT allocated_amount FROM quota_pools WHERE id = $1`, poolID).Scan(&poolAllocated)
	if poolAllocated != 5000 {
		t.Errorf("pool allocated_amount = %d, want 5000 (unchanged after rejection)", poolAllocated)
	}
}

func TestAllocate_Idempotent(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	first, err := repo.Allocate(ctx, poolID, userID, 30000, "idem-alloc-dup")
	if err != nil {
		t.Fatalf("first Allocate: %v", err)
	}
	second, err := repo.Allocate(ctx, poolID, userID, 99999, "idem-alloc-dup")
	if err != nil {
		t.Fatalf("second Allocate (idempotent): %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("expected same allocation for idempotent call, got %s vs %s", first.ID, second.ID)
	}
	if second.AllocatedAmount != 30000 {
		t.Errorf("AllocatedAmount = %d, want 30000 (replayed, not doubled)", second.AllocatedAmount)
	}

	var poolAllocated int64
	_ = repo.pool.QueryRow(ctx, `SELECT allocated_amount FROM quota_pools WHERE id = $1`, poolID).Scan(&poolAllocated)
	if poolAllocated != 30000 {
		t.Errorf("pool allocated_amount = %d, want 30000", poolAllocated)
	}
}

func TestAllocate_UnknownPool(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	_, err := repo.Allocate(ctx, uuid.New(), uuid.New(), 100, "idem-alloc-missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestFindAllocationsByTenant_Scoped(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	userA := seedQuotaUser(t, ctx, repo.pool)
	userB := seedQuotaUser(t, ctx, repo.pool)
	tenantA := seedQuotaTenant(t, ctx, repo)
	tenantB := seedQuotaTenant(t, ctx, repo)
	poolA := seedQuotaPool(t, ctx, repo, tenantA, 100000)
	poolB := seedQuotaPool(t, ctx, repo, tenantB, 100000)

	if _, err := repo.Allocate(ctx, poolA, userA, 10000, "idem-alloc-tenant-a"); err != nil {
		t.Fatalf("Allocate tenant A: %v", err)
	}
	if _, err := repo.Allocate(ctx, poolB, userB, 20000, "idem-alloc-tenant-b"); err != nil {
		t.Fatalf("Allocate tenant B: %v", err)
	}

	allocs, err := repo.FindAllocationsByTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("FindAllocationsByTenant: %v", err)
	}
	if len(allocs) != 1 {
		t.Fatalf("len = %d, want 1 (tenant A only)", len(allocs))
	}
	if allocs[0].UserID != userA || allocs[0].AllocatedAmount != 10000 {
		t.Errorf("unexpected allocation for tenant A: %+v", allocs[0])
	}
}

func TestConsume_UpdatesPoolUsedAmount(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool,
		"quota_ledger", "quota_allocations", "quota_pools",
		"tenant_models", "model_pricing", "models", "tenants", "api_key_spend", "api_keys", "users",
	)

	userID := seedQuotaUser(t, ctx, repo.pool)
	tenantID := seedQuotaTenant(t, ctx, repo)
	modelID := seedQuotaModel(t, ctx, repo)
	poolID, allocID := seedAllocation(t, ctx, repo, userID, tenantID, &modelID, 10000)

	_, err := repo.Consume(ctx, allocID, 500, "idem-pool-update")
	if err != nil {
		t.Fatalf("Consume: %v", err)
	}

	var poolUsed int64
	_ = repo.pool.QueryRow(ctx, `SELECT used_amount FROM quota_pools WHERE id = $1`, poolID).Scan(&poolUsed)
	if poolUsed != 500 {
		t.Errorf("pool used_amount = %d, want 500", poolUsed)
	}
}

func TestUpdatePool_Success(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	pool, err := repo.UpdatePool(ctx, poolID, 250000, "chars", "token")
	if err != nil {
		t.Fatalf("UpdatePool: %v", err)
	}
	if pool.TotalAmount != 250000 {
		t.Errorf("TotalAmount = %d, want 250000", pool.TotalAmount)
	}
	if pool.UnitName != "chars" {
		t.Errorf("UnitName = %s, want chars", pool.UnitName)
	}
	if pool.Dimension != "token" {
		t.Errorf("Dimension = %s, want token", pool.Dimension)
	}

	// Persisted, not just returned.
	got, err := repo.FindPool(ctx, poolID)
	if err != nil {
		t.Fatalf("FindPool after update: %v", err)
	}
	if got.TotalAmount != 250000 || got.UnitName != "chars" {
		t.Errorf("persisted pool = %+v, want total=250000 unit=chars", got)
	}
}

func TestUpdatePool_NotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	_, err := repo.UpdatePool(ctx, uuid.New(), 250000, "token", "token")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestUpdatePool_BelowAllocatedRejected(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	tenantID := seedQuotaTenant(t, ctx, repo)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	// Allocate 60k so shrinking the pool below 60k must be rejected.
	userID := seedQuotaUser(t, ctx, repo.pool)
	if _, err := repo.Allocate(ctx, poolID, userID, 60000, "idem-update-below"); err != nil {
		t.Fatalf("Allocate: %v", err)
	}

	if _, err := repo.UpdatePool(ctx, poolID, 50000, "token", "token"); !errors.Is(err, ErrConstraintViolation) {
		t.Fatalf("expected ErrConstraintViolation, got: %v", err)
	}

	// Pool unchanged after the rejected shrink.
	got, _ := repo.FindPool(ctx, poolID)
	if got.TotalAmount != 100000 {
		t.Errorf("TotalAmount = %d, want 100000 (unchanged)", got.TotalAmount)
	}
}

func TestDeletePool_CascadesLedgerAndAllocations(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	tenantID := seedQuotaTenant(t, ctx, repo)
	userID := seedQuotaUser(t, ctx, repo.pool)
	poolID := seedQuotaPool(t, ctx, repo, tenantID, 100000)

	alloc, err := repo.Allocate(ctx, poolID, userID, 50000, "idem-delete-alloc")
	if err != nil {
		t.Fatalf("Allocate: %v", err)
	}
	if _, err := repo.Consume(ctx, alloc.ID, 10000, "idem-delete-consume"); err != nil {
		t.Fatalf("Consume: %v", err)
	}

	if err := repo.DeletePool(ctx, poolID); err != nil {
		t.Fatalf("DeletePool: %v", err)
	}

	// Pool gone.
	if _, err := repo.FindPool(ctx, poolID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got: %v", err)
	}

	// Allocation gone.
	var allocCount int
	_ = repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM quota_allocations WHERE id = $1`, alloc.ID).Scan(&allocCount)
	if allocCount != 0 {
		t.Errorf("quota_allocations rows = %d, want 0 (cascaded)", allocCount)
	}

	// Ledger gone.
	var ledgerCount int
	_ = repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM quota_ledger WHERE allocation_id = $1`, alloc.ID).Scan(&ledgerCount)
	if ledgerCount != 0 {
		t.Errorf("quota_ledger rows = %d, want 0 (cascaded)", ledgerCount)
	}
}

func TestDeletePool_NotFound(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, truncateQuota...)

	if err := repo.DeletePool(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}
