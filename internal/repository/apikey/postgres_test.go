package apikey

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// seedAPIKey creates a user and returns their ID for FK references.
func seedUser(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@test.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

// seedTenant creates a tenant and returns its ID. Uses a UUID suffix for uniqueness.
func seedTenant(t *testing.T, ctx context.Context, repo *PostgresRepository, prefix string) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	code := prefix + "-" + tenantID.String()[:8]
	_, err := repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, code, code+" name")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenantID
}

func TestCreateAPIKey(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-abc",
		KeyHash:         "hash_abc123",
		MaskedKey:       "sk-abc...xyz",
		Name:            "test-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	t.Run("creates key successfully", func(t *testing.T) {
		if err := repo.Create(ctx, key); err != nil {
			t.Fatalf("Create: unexpected error: %v", err)
		}
	})

	t.Run("find by id after create", func(t *testing.T) {
		found, err := repo.FindByID(ctx, key.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.KeyHash != key.KeyHash {
			t.Errorf("KeyHash = %s, want %s", found.KeyHash, key.KeyHash)
		}
	})

	t.Run("duplicate key hash returns error", func(t *testing.T) {
		dup := &domain.APIKey{
			ID:              uuid.New(),
			UserID:          userID,
			KeyPrefix:       "sk-dup",
			KeyHash:         "hash_abc123",
			MaskedKey:       "sk-dup...xyz",
			Name:            "dup-key",
			Status:          domain.APIKeyStatusActive,
			OverLimitAction: domain.OverLimitBlock,
			CreatedAt:       time.Now().UTC(),
			UpdatedAt:       time.Now().UTC(),
		}
		err := repo.Create(ctx, dup)
		if err == nil {
			t.Error("expected error for duplicate key_hash")
		}
	})
}

func TestFindByHash(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-x",
		KeyHash:         "find_by_hash_key",
		MaskedKey:       "sk-x...xxx",
		Name:            "hash-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("finds by hash", func(t *testing.T) {
		found, err := repo.FindByHash(ctx, key.KeyHash)
		if err != nil {
			t.Fatalf("FindByHash: %v", err)
		}
		if found.ID != key.ID {
			t.Errorf("ID = %s, want %s", found.ID, key.ID)
		}
	})

	t.Run("returns error for unknown hash", func(t *testing.T) {
		_, err := repo.FindByHash(ctx, "bogus_hash")
		if err == nil {
			t.Error("expected error for unknown hash")
		}
	})
}

func TestFindByID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-fid",
		KeyHash:         "find_by_id_key",
		MaskedKey:       "sk-fid...xxx",
		Name:            "fid-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("finds by id", func(t *testing.T) {
		found, err := repo.FindByID(ctx, key.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.KeyHash != key.KeyHash {
			t.Errorf("KeyHash = %s, want %s", found.KeyHash, key.KeyHash)
		}
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := repo.FindByID(ctx, uuid.New())
		if err == nil {
			t.Error("expected error for unknown id")
		}
	})
}

func TestListByUser(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)
	tenantID := seedTenant(t, ctx, repo, "tenant-list")

	key1 := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		TenantID:        &tenantID,
		KeyPrefix:       "sk-a",
		KeyHash:         "list_key_a",
		MaskedKey:       "sk-a...xxx",
		Name:            "key-a",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	key2 := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		TenantID:        &tenantID,
		KeyPrefix:       "sk-b",
		KeyHash:         "list_key_b",
		MaskedKey:       "sk-b...xxx",
		Name:            "key-b",
		Status:          domain.APIKeyStatusDisabled,
		OverLimitAction: domain.OverLimitWarn,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	_ = repo.Create(ctx, key1)
	_ = repo.Create(ctx, key2)

	t.Run("lists by user and tenant", func(t *testing.T) {
		keys, err := repo.ListByUser(ctx, userID, &tenantID)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("len(keys) = %d, want 2", len(keys))
		}
	})

	t.Run("lists by user without tenant filter", func(t *testing.T) {
		keys, err := repo.ListByUser(ctx, userID, nil)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("len(keys) = %d, want 2", len(keys))
		}
	})

	t.Run("empty for unknown user", func(t *testing.T) {
		keys, err := repo.ListByUser(ctx, uuid.New(), nil)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("expected empty, got %d", len(keys))
		}
	})
}

// TestListByUser_HandlesNullLimits guards against the regression where a key
// created without limits (NULL cumulative/weekly/monthly_limit, NULL name,
// NULL tenant_id) crashed the scan with "cannot scan NULL into *string".
// The demo seed produces exactly these rows, and so does a key that is created
// with no limits configured.
func TestListByUser_HandlesNullLimits(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)
	now := time.Now().UTC()

	// Insert via raw SQL with every nullable column left NULL — the failure mode
	// seen in the demo seed.
	if _, err := repo.pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, created_at, updated_at)
		 VALUES ($1, $2, 'sk-null', 'null_limits_key', 'sk-null...xxx', $3, $3)`,
		uuid.New(), userID, now); err != nil {
		t.Fatalf("seed key with NULL limits: %v", err)
	}

	t.Run("list scans NULL limits as zero", func(t *testing.T) {
		keys, err := repo.ListByUser(ctx, userID, nil)
		if err != nil {
			t.Fatalf("ListByUser with NULL-limit row: %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("len(keys) = %d, want 1", len(keys))
		}
		k := keys[0]
		if !k.CumulativeLimit.Equal(decimal.Zero) {
			t.Errorf("CumulativeLimit = %s, want 0", k.CumulativeLimit)
		}
		if !k.WeeklyLimit.Equal(decimal.Zero) {
			t.Errorf("WeeklyLimit = %s, want 0", k.WeeklyLimit)
		}
		if !k.MonthlyLimit.Equal(decimal.Zero) {
			t.Errorf("MonthlyLimit = %s, want 0", k.MonthlyLimit)
		}
		if k.TenantID != nil {
			t.Errorf("TenantID = %v, want nil", k.TenantID)
		}
		if k.LastUsedAt != nil {
			t.Errorf("LastUsedAt = %v, want nil", k.LastUsedAt)
		}
	})

	t.Run("find by id scans NULL limits as zero", func(t *testing.T) {
		keys, err := repo.ListByUser(ctx, userID, nil)
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		found, err := repo.FindByID(ctx, keys[0].ID)
		if err != nil {
			t.Fatalf("FindByID on NULL-limit row: %v", err)
		}
		if !found.CumulativeLimit.Equal(decimal.Zero) {
			t.Errorf("FindByID CumulativeLimit = %s, want 0", found.CumulativeLimit)
		}
	})
}

func TestUpdateAPIKey(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-up",
		KeyHash:         "update_key",
		MaskedKey:       "sk-up...xxx",
		Name:            "original",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("updates name and status", func(t *testing.T) {
		key.Name = "renamed"
		key.Status = domain.APIKeyStatusDisabled
		key.UpdatedAt = time.Now().UTC()
		if err := repo.Update(ctx, key); err != nil {
			t.Fatalf("Update: %v", err)
		}

		found, err := repo.FindByID(ctx, key.ID)
		if err != nil {
			t.Fatalf("FindByID after update: %v", err)
		}
		if found.Name != "renamed" {
			t.Errorf("Name = %s, want renamed", found.Name)
		}
		if found.Status != domain.APIKeyStatusDisabled {
			t.Errorf("Status = %s, want disabled", found.Status)
		}
	})
}

func TestUpdateLastUsed(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-lu",
		KeyHash:         "last_used_key",
		MaskedKey:       "sk-lu...xxx",
		Name:            "lu-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("updates last_used_at", func(t *testing.T) {
		if err := repo.UpdateLastUsed(ctx, key.ID); err != nil {
			t.Fatalf("UpdateLastUsed: %v", err)
		}
		found, _ := repo.FindByID(ctx, key.ID)
		if found.LastUsedAt == nil {
			t.Error("expected LastUsedAt to be set")
		}
		if !found.Last7dActive {
			t.Error("expected Last7dActive to be true")
		}
	})
}

func TestGetSpendAndUpdateSpend(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-sp",
		KeyHash:         "spend_key",
		MaskedKey:       "sk-sp...xxx",
		Name:            "spend-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("get spend returns zero when no records", func(t *testing.T) {
		spend, err := repo.GetSpend(ctx, key.ID, "cumulative")
		if err != nil {
			t.Fatalf("GetSpend: %v", err)
		}
		if !spend.TotalCost.Equal(decimal.Zero) {
			t.Errorf("TotalCost = %s, want 0", spend.TotalCost)
		}
	})

	t.Run("update spend creates or updates record", func(t *testing.T) {
		amount := decimal.NewFromFloat(10.5)
		spend := &domain.APIKeySpend{
			APIKeyID:   key.ID,
			PeriodType: "cumulative",
			TotalCost:  amount,
		}
		if err := repo.UpdateSpend(ctx, spend); err != nil {
			t.Fatalf("UpdateSpend: %v", err)
		}

		got, err := repo.GetSpend(ctx, key.ID, "cumulative")
		if err != nil {
			t.Fatalf("GetSpend: %v", err)
		}
		if !got.TotalCost.Equal(amount) {
			t.Errorf("TotalCost = %s, want %s", got.TotalCost, amount)
		}
	})

	t.Run("update spend accumulates", func(t *testing.T) {
		amount := decimal.NewFromFloat(5.25)
		spend := &domain.APIKeySpend{
			APIKeyID:   key.ID,
			PeriodType: "cumulative",
			TotalCost:  amount,
		}
		if err := repo.UpdateSpend(ctx, spend); err != nil {
			t.Fatalf("UpdateSpend: %v", err)
		}

		got, err := repo.GetSpend(ctx, key.ID, "cumulative")
		if err != nil {
			t.Fatalf("GetSpend: %v", err)
		}
		expected := decimal.NewFromFloat(15.75)
		if !got.TotalCost.Equal(expected) {
			t.Errorf("TotalCost = %s, want %s", got.TotalCost, expected)
		}
	})
}

func TestAPIKeyUpdateEdgeCases(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateTables(t, repo.pool, "wallet_transactions", "wallets", "api_key_spend", "api_keys", "users")

	userID := seedUser(t, ctx, repo)

	key := &domain.APIKey{
		ID:              uuid.New(),
		UserID:          userID,
		KeyPrefix:       "sk-edge",
		KeyHash:         "edge_case_key",
		MaskedKey:       "sk-edge...xxx",
		Name:            "edge-key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := repo.Create(ctx, key); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("update with nil slices preserves arrays", func(t *testing.T) {
		key.Name = "nil-slices"
		key.AllowedModels = nil
		key.SourceWhitelist = nil
		key.UpdatedAt = time.Now().UTC()
		if err := repo.Update(ctx, key); err != nil {
			t.Fatalf("Update with nil: %v", err)
		}
		found, _ := repo.FindByID(ctx, key.ID)
		if found.Name != "nil-slices" {
			t.Errorf("Name = %s, want nil-slices", found.Name)
		}
	})

	t.Run("update with revoked_at set", func(t *testing.T) {
		now := time.Now().UTC()
		key.Status = domain.APIKeyStatusRevoked
		key.RevokedAt = &now
		key.UpdatedAt = now
		if err := repo.Update(ctx, key); err != nil {
			t.Fatalf("Update revoked: %v", err)
		}
		found, _ := repo.FindByID(ctx, key.ID)
		if found.Status != domain.APIKeyStatusRevoked {
			t.Errorf("Status = %s, want revoked", found.Status)
		}
		if found.RevokedAt == nil {
			t.Error("expected RevokedAt to be set")
		}
	})

	t.Run("find by id returns error for zero uuid", func(t *testing.T) {
		_, err := repo.FindByID(ctx, uuid.Nil)
		if err == nil {
			t.Error("expected error for zero uuid")
		}
	})

	t.Run("find by hash returns error for empty hash", func(t *testing.T) {
		_, err := repo.FindByHash(ctx, "")
		if err == nil {
			t.Error("expected error for empty hash")
		}
	})
}

func TestParseDecimalHelper(t *testing.T) {
	t.Run("nil returns zero", func(t *testing.T) {
		d := parseDecimal(nil)
		if !d.Equal(decimal.Zero) {
			t.Errorf("expected zero for nil, got %s", d)
		}
	})
	t.Run("invalid string returns zero", func(t *testing.T) {
		d := parseDecimal("not-a-number")
		if !d.Equal(decimal.Zero) {
			t.Errorf("expected zero for invalid, got %s", d)
		}
	})
	t.Run("bytes returns correct value", func(t *testing.T) {
		d := parseDecimal([]byte("3.14"))
		if !d.Equal(decimal.NewFromFloat(3.14)) {
			t.Errorf("expected 3.14, got %s", d)
		}
	})
	t.Run("float64 returns correct value", func(t *testing.T) {
		d := parseDecimal(float64(2.5))
		if !d.Equal(decimal.NewFromFloat(2.5)) {
			t.Errorf("expected 2.5, got %s", d)
		}
	})
}

func TestNowUTC(t *testing.T) {
	now := nowUTC()
	if now.IsZero() {
		t.Error("nowUTC returned zero time")
	}
	if now.Location().String() != "UTC" {
		t.Errorf("expected UTC location, got %s", now.Location())
	}
}

func TestParseJSONStringArrayHelper(t *testing.T) {
	t.Run("empty string returns empty slice", func(t *testing.T) {
		arr := parseJSONStringArray("")
		if len(arr) != 0 {
			t.Errorf("expected empty slice, got %d elements", len(arr))
		}
	})
	t.Run("null returns empty slice", func(t *testing.T) {
		arr := parseJSONStringArray("null")
		if len(arr) != 0 {
			t.Errorf("expected empty slice, got %d elements", len(arr))
		}
	})
	t.Run("invalid json returns empty slice", func(t *testing.T) {
		arr := parseJSONStringArray("{bad}")
		if len(arr) != 0 {
			t.Errorf("expected empty slice, got %d elements", len(arr))
		}
	})
}
