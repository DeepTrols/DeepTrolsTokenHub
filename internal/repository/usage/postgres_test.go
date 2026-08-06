package usage

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jsonb"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func seedUsageUser(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := repo.pool.Exec(ctx, `INSERT INTO users (id, email, password_hash) VALUES ($1, $2, $3)`,
		userID, userID.String()+"@test.com", "hash")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return userID
}

func seedUsageKey(t *testing.T, ctx context.Context, repo *PostgresRepository, userID uuid.UUID) uuid.UUID {
	t.Helper()
	keyID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, status, over_limit_action)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, keyID, userID, "sk-test", "hash_"+keyID.String(), "sk-xxx...xxx", "test-key", "active", "block")
	if err != nil {
		t.Fatalf("seed key: %v", err)
	}
	return keyID
}

func TestCreateUsageLog(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	userID := seedUsageUser(t, ctx, repo)
	keyID := seedUsageKey(t, ctx, repo, userID)

	log := &domain.UsageLog{
		ID:              uuid.New(),
		UserID:          userID,
		APIKeyID:        keyID,
		RequestID:       "req-" + uuid.New().String()[:8],
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.NewFromFloat(0.01),
		FinalCost:       decimal.NewFromFloat(0.01),
		Currency:        "CNY",
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}

	t.Run("creates usage log", func(t *testing.T) {
		if err := repo.CreateUsageLog(ctx, log); err != nil {
			t.Fatalf("CreateUsageLog: %v", err)
		}
	})

	t.Run("finds by request_id", func(t *testing.T) {
		found, err := repo.FindByRequestID(ctx, log.RequestID)
		if err != nil {
			t.Fatalf("FindByRequestID: %v", err)
		}
		if found.PublicModelCode != log.PublicModelCode {
			t.Errorf("PublicModelCode = %s, want %s", found.PublicModelCode, log.PublicModelCode)
		}
	})

	t.Run("returns error for unknown request_id", func(t *testing.T) {
		_, err := repo.FindByRequestID(ctx, "nonexistent")
		if err == nil {
			t.Error("expected error for unknown request_id")
		}
	})
}

func TestCreateChargeLines(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	userID := seedUsageUser(t, ctx, repo)
	keyID := seedUsageKey(t, ctx, repo, userID)

	logID := uuid.New()
	log := &domain.UsageLog{
		ID:              logID,
		UserID:          userID,
		APIKeyID:        keyID,
		RequestID:       "req-charges-" + uuid.New().String()[:8],
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.NewFromFloat(0.01),
		FinalCost:       decimal.NewFromFloat(0.01),
		Currency:        "CNY",
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	if err := repo.CreateUsageLog(ctx, log); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	lines := []domain.ChargeLine{
		{
			ID:         uuid.New(),
			UsageLogID: logID,
			Dimension:  "input",
			UnitName:   "token",
			Quantity:   100,
			UnitPrice:  decimal.NewFromFloat(0.000005),
			LineCost:   decimal.NewFromFloat(0.0005),
			CreatedAt:  time.Now().UTC(),
		},
		{
			ID:         uuid.New(),
			UsageLogID: logID,
			Dimension:  "output",
			UnitName:   "token",
			Quantity:   50,
			UnitPrice:  decimal.NewFromFloat(0.000015),
			LineCost:   decimal.NewFromFloat(0.00075),
			CreatedAt:  time.Now().UTC(),
		},
	}

	t.Run("creates charge lines in batch", func(t *testing.T) {
		if err := repo.CreateChargeLines(ctx, lines); err != nil {
			t.Fatalf("CreateChargeLines: %v", err)
		}
	})

	t.Run("creates empty batch without error", func(t *testing.T) {
		if err := repo.CreateChargeLines(ctx, []domain.ChargeLine{}); err != nil {
			t.Fatalf("CreateChargeLines empty: %v", err)
		}
	})
}

func TestCreateProviderEvidence(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	userID := seedUsageUser(t, ctx, repo)
	keyID := seedUsageKey(t, ctx, repo, userID)

	logID := uuid.New()
	log := &domain.UsageLog{
		ID:              logID,
		UserID:          userID,
		APIKeyID:        keyID,
		RequestID:       "req-evidence-" + uuid.New().String()[:8],
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.NewFromFloat(0.01),
		FinalCost:       decimal.NewFromFloat(0.01),
		Currency:        "CNY",
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	if err := repo.CreateUsageLog(ctx, log); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	evidence := &domain.ProviderEvidence{
		ID:                uuid.New(),
		UsageLogID:        &logID,
		Provider:          "openai",
		ProviderRequestID: "openai-req-123",
		RequestBody:       map[string]any{"model": "gpt-4o"},
		ResponseBody:      map[string]any{"choices": []any{}},
		StatusCode:        200,
		DurationMs:        1500,
		ProviderCost:      decimal.NewFromFloat(0.008),
		ProviderCurrency:  "USD",
		CreatedAt:         time.Now().UTC(),
	}

	t.Run("creates provider evidence", func(t *testing.T) {
		if err := repo.CreateProviderEvidence(ctx, evidence); err != nil {
			t.Fatalf("CreateProviderEvidence: %v", err)
		}
	})
}

func TestListUsageLogs(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	userID := seedUsageUser(t, ctx, repo)
	keyID := seedUsageKey(t, ctx, repo, userID)

	for i := 0; i < 3; i++ {
		log := &domain.UsageLog{
			ID:              uuid.New(),
			UserID:          userID,
			APIKeyID:        keyID,
			RequestID:       "req-list-" + uuid.New().String()[:8],
			RequestType:     "chat",
			PublicModelCode: "gpt-4o",
			UsageSource:     domain.UsageSourceUpstream,
			ListCost:        decimal.NewFromFloat(0.01),
			FinalCost:       decimal.NewFromFloat(0.01),
			Currency:        "CNY",
			Status:          domain.UsageLogStatusCompleted,
			CreatedAt:       time.Now().UTC(),
		}
		if err := repo.CreateUsageLog(ctx, log); err != nil {
			t.Fatalf("seed log %d: %v", i, err)
		}
	}

	t.Run("lists by user", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{Limit: 10})
		if err != nil {
			t.Fatalf("ListByUser: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(logs) != 3 {
			t.Errorf("len(logs) = %d, want 3", len(logs))
		}
	})

	t.Run("lists by api key", func(t *testing.T) {
		logs, total, err := repo.ListByAPIKey(ctx, keyID, UsageFilter{Limit: 10})
		if err != nil {
			t.Fatalf("ListByAPIKey: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(logs) != 3 {
			t.Errorf("len(logs) = %d, want 3", len(logs))
		}
	})

	t.Run("filters by model code", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{
			ModelCode: "gpt-4o",
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("ListByUser filtered: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(logs) != 3 {
			t.Errorf("len(logs) = %d, want 3", len(logs))
		}
	})

	t.Run("filters by status", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{
			Status: "completed",
			Limit:  10,
		})
		if err != nil {
			t.Fatalf("ListByUser by status: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(logs) != 3 {
			t.Errorf("len(logs) = %d, want 3", len(logs))
		}
	})

	t.Run("empty for unknown user", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, uuid.New(), UsageFilter{Limit: 10})
		if err != nil {
			t.Fatalf("ListByUser unknown: %v", err)
		}
		if total != 0 {
			t.Errorf("total = %d, want 0", total)
		}
		if len(logs) != 0 {
			t.Errorf("len(logs) = %d, want 0", len(logs))
		}
	})

	t.Run("pagination respects limit", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{Limit: 1, Offset: 0})
		if err != nil {
			t.Fatalf("ListByUser paginated: %v", err)
		}
		if total != 3 {
			t.Errorf("total = %d, want 3", total)
		}
		if len(logs) != 1 {
			t.Errorf("len(logs) = %d, want 1", len(logs))
		}
	})
}

func TestUsageListFilters(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	userID := seedUsageUser(t, ctx, repo)
	keyID := seedUsageKey(t, ctx, repo, userID)

	reqID := "req-filter-" + uuid.New().String()[:8]
	log := &domain.UsageLog{
		ID:              uuid.New(),
		UserID:          userID,
		APIKeyID:        keyID,
		RequestID:       reqID,
		RequestType:     "chat",
		PublicModelCode: "gpt-4o",
		UsageSource:     domain.UsageSourceUpstream,
		ListCost:        decimal.NewFromFloat(0.01),
		FinalCost:       decimal.NewFromFloat(0.01),
		Currency:        "CNY",
		Status:          domain.UsageLogStatusCompleted,
		CreatedAt:       time.Now().UTC(),
	}
	if err := repo.CreateUsageLog(ctx, log); err != nil {
		t.Fatalf("seed: %v", err)
	}

	t.Run("filters by request_id", func(t *testing.T) {
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{
			RequestID: reqID,
			Limit:     10,
		})
		if err != nil {
			t.Fatalf("ListByUser by request_id: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(logs) != 1 {
			t.Errorf("len(logs) = %d, want 1", len(logs))
		}
	})

	t.Run("filters by date range", func(t *testing.T) {
		yesterday := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
		tomorrow := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		logs, total, err := repo.ListByUser(ctx, userID, UsageFilter{
			From:  yesterday,
			To:    tomorrow,
			Limit: 10,
		})
		if err != nil {
			t.Fatalf("ListByUser date range: %v", err)
		}
		if total != 1 {
			t.Errorf("total = %d, want 1", total)
		}
		if len(logs) != 1 {
			t.Errorf("len(logs) = %d, want 1", len(logs))
		}
	})

	t.Run("handles negative offset", func(t *testing.T) {
		_, _, err := repo.ListByUser(ctx, userID, UsageFilter{
			Limit:  10,
			Offset: -1,
		})
		if err != nil {
			t.Fatalf("ListByUser negative offset: %v", err)
		}
	})
}

func TestUsageJSONBHelpers(t *testing.T) {
	t.Run("marshal nil returns empty object", func(t *testing.T) {
		b := jsonb.Marshal(nil)
		if string(b) != "{}" {
			t.Errorf("expected {}, got %s", string(b))
		}
	})

	t.Run("unmarshal empty returns empty map", func(t *testing.T) {
		m := jsonb.Unmarshal("")
		if len(m) != 0 {
			t.Errorf("expected empty map, got %d keys", len(m))
		}
	})

	t.Run("unmarshal null returns empty map", func(t *testing.T) {
		m := jsonb.Unmarshal("null")
		if len(m) != 0 {
			t.Errorf("expected empty map, got %d keys", len(m))
		}
	})

	t.Run("unmarshal invalid returns empty map", func(t *testing.T) {
		m := jsonb.Unmarshal("{not valid}")
		if len(m) != 0 {
			t.Errorf("expected empty map, got %d keys", len(m))
		}
	})
}

func TestParseDecimalStrUsage(t *testing.T) {
	t.Run("invalid string returns zero", func(t *testing.T) {
		d := parseDecimalStr("bad")
		if !d.Equal(decimal.Zero) {
			t.Errorf("expected zero, got %s", d)
		}
	})
}
