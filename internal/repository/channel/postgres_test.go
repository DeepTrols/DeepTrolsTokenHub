package channel

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

func seedChannelModel(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	modelID := uuid.New()
	code := "ch-model-" + uuid.New().String()[:8]
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO models (id, code, provider, category, display_name, status, release_stage)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, modelID, code, "openai", "chat", code+" display", "active", "GA")
	if err != nil {
		t.Fatalf("seed model: %v", err)
	}
	return modelID
}

func seedChannelTenant(t *testing.T, ctx context.Context, repo *PostgresRepository) uuid.UUID {
	t.Helper()
	tenantID := uuid.New()
	code := "ch-tenant-" + uuid.New().String()[:8]
	_, err := repo.pool.Exec(ctx, `INSERT INTO tenants (id, code, name) VALUES ($1, $2, $3)`,
		tenantID, code, code+" name")
	if err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	return tenantID
}

func marshalJSONB(v map[string]any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestChannelListByModel(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	modelID := seedChannelModel(t, ctx, repo)
	tenantID := seedChannelTenant(t, ctx, repo)

	ch1 := domain.Channel{
		ID: uuid.New(), Name: "shared-channel", ModelID: modelID,
		PoolType: domain.PoolTypeShared, HealthScore: 100,
		HealthStatus: domain.HealthStatusHealthy, Status: domain.ChannelStatusActive,
		Weight: 100, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	ch2 := domain.Channel{
		ID: uuid.New(), Name: "dedicated-channel", ModelID: modelID,
		TenantID: &tenantID, PoolType: domain.PoolTypeDedicated,
		HealthScore: 90, HealthStatus: domain.HealthStatusHealthy,
		Status: domain.ChannelStatusActive, Weight: 200,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	for _, ch := range []domain.Channel{ch1, ch2} {
		_, err := repo.pool.Exec(ctx, `
			INSERT INTO channels (id, name, model_id, tenant_id, pool_type, health_score, health_status, status, weight, max_concurrency)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		`, ch.ID, ch.Name, ch.ModelID, ch.TenantID, ch.PoolType, ch.HealthScore, ch.HealthStatus, ch.Status, ch.Weight, ch.MaxConcurrency)
		if err != nil {
			t.Fatalf("seed channel: %v", err)
		}
	}

	t.Run("lists all channels for model ordered by weight", func(t *testing.T) {
		channels, err := repo.ListByModel(ctx, modelID, nil)
		if err != nil {
			t.Fatalf("ListByModel: %v", err)
		}
		if len(channels) != 2 {
			t.Errorf("len(channels) = %d, want 2", len(channels))
		}
		if len(channels) == 2 && channels[0].Weight != 200 {
			t.Errorf("first channel weight = %d, want 200", channels[0].Weight)
		}
	})

	t.Run("tenant requests include shared channels, dedicated first", func(t *testing.T) {
		channels, err := repo.ListByModel(ctx, modelID, &tenantID)
		if err != nil {
			t.Fatalf("ListByModel filtered: %v", err)
		}
		if len(channels) != 2 {
			t.Fatalf("len(channels) = %d, want 2 (dedicated + shared)", len(channels))
		}
		if channels[0].TenantID == nil {
			t.Error("channels[0].TenantID = nil, want the dedicated channel first")
		}
		if channels[1].TenantID != nil {
			t.Errorf("channels[1].TenantID = %v, want the shared channel second", *channels[1].TenantID)
		}
	})

	t.Run("tenant B cannot see tenant A's dedicated channel", func(t *testing.T) {
		tenantB := seedChannelTenant(t, ctx, repo)
		channels, err := repo.ListByModel(ctx, modelID, &tenantB)
		if err != nil {
			t.Fatalf("ListByModel tenant B: %v", err)
		}
		if len(channels) != 1 {
			t.Fatalf("len(channels) = %d, want 1 (shared only, no tenant A channel)", len(channels))
		}
		if channels[0].TenantID != nil {
			t.Errorf("channels[0].TenantID = %v, want nil (shared channel)", *channels[0].TenantID)
		}
	})

	t.Run("empty for unknown model", func(t *testing.T) {
		channels, err := repo.ListByModel(ctx, uuid.New(), nil)
		if err != nil {
			t.Fatalf("ListByModel unknown: %v", err)
		}
		if len(channels) != 0 {
			t.Errorf("len(channels) = %d, want 0", len(channels))
		}
	})
}

func TestChannelFindByID(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	modelID := seedChannelModel(t, ctx, repo)

	chID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, chID, "findable", modelID, "shared", 100, "healthy", "active", 100, 10)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	t.Run("finds by id", func(t *testing.T) {
		found, err := repo.FindByID(ctx, chID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}
		if found.Name != "findable" {
			t.Errorf("Name = %s, want findable", found.Name)
		}
	})

	t.Run("returns error for unknown id", func(t *testing.T) {
		_, err := repo.FindByID(ctx, uuid.New())
		if err == nil {
			t.Error("expected error for unknown channel id")
		}
	})
}

func TestChannelInstances(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	modelID := seedChannelModel(t, ctx, repo)

	chID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, chID, "instance-test", modelID, "shared", 100, "healthy", "active", 100, 10)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	inst1 := domain.ChannelInstance{
		ID: uuid.New(), ChannelID: chID, InstanceType: "litellm",
		BaseURL: "http://litellm:4000", CurrentLoad: 0, MaxLoad: 10,
		Config: map[string]any{"api_key": "sk-xxx"},
		Status: domain.InstanceStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	inst2 := domain.ChannelInstance{
		ID: uuid.New(), ChannelID: chID, InstanceType: "litellm",
		BaseURL: "http://litellm-2:4000", CurrentLoad: 5, MaxLoad: 10,
		Config: map[string]any{"api_key": "sk-yyy"},
		Status: domain.InstanceStatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}

	for _, inst := range []domain.ChannelInstance{inst1, inst2} {
		_, err := repo.pool.Exec(ctx, `
			INSERT INTO channel_instances (id, channel_id, instance_type, base_url, current_load, max_load, config, status)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, inst.ID, inst.ChannelID, inst.InstanceType, inst.BaseURL, inst.CurrentLoad, inst.MaxLoad, marshalJSONB(inst.Config), inst.Status)
		if err != nil {
			t.Fatalf("seed instance: %v", err)
		}
	}

	t.Run("lists instances", func(t *testing.T) {
		instances, err := repo.ListInstances(ctx, chID)
		if err != nil {
			t.Fatalf("ListInstances: %v", err)
		}
		if len(instances) != 2 {
			t.Errorf("len(instances) = %d, want 2", len(instances))
		}
	})

	t.Run("empty for unknown channel", func(t *testing.T) {
		instances, err := repo.ListInstances(ctx, uuid.New())
		if err != nil {
			t.Fatalf("ListInstances unknown: %v", err)
		}
		if len(instances) != 0 {
			t.Errorf("len(instances) = %d, want 0", len(instances))
		}
	})

	t.Run("updates instance load", func(t *testing.T) {
		if err := repo.UpdateInstanceLoad(ctx, inst1.ID, 8); err != nil {
			t.Fatalf("UpdateInstanceLoad: %v", err)
		}
	})

	t.Run("update load for non-existent instance returns error", func(t *testing.T) {
		err := repo.UpdateInstanceLoad(ctx, uuid.New(), 5)
		if err == nil {
			t.Error("expected error for unknown instance")
		}
	})
}

func TestChannelUpdateHealth(t *testing.T) {
	repo := NewPostgresRepository(testutil.SetupPool(t))
	ctx := context.Background()
	testutil.TruncateAll(t, repo.pool)

	modelID := seedChannelModel(t, ctx, repo)

	chID := uuid.New()
	_, err := repo.pool.Exec(ctx, `
		INSERT INTO channels (id, name, model_id, pool_type, health_score, health_status, status, weight, max_concurrency)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, chID, "health-test", modelID, "shared", 100, "healthy", "active", 100, 10)
	if err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	t.Run("updates health", func(t *testing.T) {
		if err := repo.UpdateHealth(ctx, chID, 75, domain.HealthStatusDegraded); err != nil {
			t.Fatalf("UpdateHealth: %v", err)
		}
		found, _ := repo.FindByID(ctx, chID)
		if found.HealthScore != 75 {
			t.Errorf("HealthScore = %d, want 75", found.HealthScore)
		}
		if found.HealthStatus != domain.HealthStatusDegraded {
			t.Errorf("HealthStatus = %s, want degraded", found.HealthStatus)
		}
	})

	t.Run("update health for non-existent channel returns error", func(t *testing.T) {
		err := repo.UpdateHealth(ctx, uuid.New(), 50, domain.HealthStatusUnhealthy)
		if err == nil {
			t.Error("expected error for unknown channel")
		}
	})
}
