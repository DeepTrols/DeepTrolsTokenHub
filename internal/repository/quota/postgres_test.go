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
