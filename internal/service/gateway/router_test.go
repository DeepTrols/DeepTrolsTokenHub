package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/repository/channel"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// --- mock types ---

type mockModelRepo struct {
	findByCodeFn  func(ctx context.Context, code string) (*domain.Model, error)
	tenantModelFn func(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error)
}

func (m *mockModelRepo) ListActive(ctx context.Context) ([]domain.Model, error) { return nil, nil }
func (m *mockModelRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Model, error) {
	return nil, nil
}
func (m *mockModelRepo) ListByTenant(ctx context.Context, tenantID *uuid.UUID) ([]model.TenantModelView, error) {
	return nil, nil
}
func (m *mockModelRepo) FindByCode(ctx context.Context, code string) (*domain.Model, error) {
	if m.findByCodeFn != nil {
		return m.findByCodeFn(ctx, code)
	}
	return nil, errors.New("not implemented")
}
func (m *mockModelRepo) GetTenantModel(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error) {
	if m.tenantModelFn != nil {
		return m.tenantModelFn(ctx, tenantID, modelCode)
	}
	return nil, errors.New("not implemented")
}

var _ model.Repository = (*mockModelRepo)(nil)

type mockChannelRepo struct {
	listByModelFn   func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error)
	listInstancesFn func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error)
}

func (m *mockChannelRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error) {
	return nil, nil
}
func (m *mockChannelRepo) ListByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
	if m.listByModelFn != nil {
		return m.listByModelFn(ctx, modelID, tenantID)
	}
	return nil, errors.New("not implemented")
}
func (m *mockChannelRepo) ListInstances(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
	if m.listInstancesFn != nil {
		return m.listInstancesFn(ctx, channelID)
	}
	return nil, errors.New("not implemented")
}
func (m *mockChannelRepo) UpdateHealth(ctx context.Context, id uuid.UUID, score int, status domain.HealthStatus) error {
	return nil
}
func (m *mockChannelRepo) UpdateInstanceLoad(ctx context.Context, id uuid.UUID, load int) error {
	return nil
}
func (m *mockChannelRepo) EnterCooldown(ctx context.Context, id uuid.UUID, until time.Time) error {
	return nil
}
func (m *mockChannelRepo) ClearCooldown(ctx context.Context, id uuid.UUID) error {
	return nil
}

var _ channel.Repository = (*mockChannelRepo)(nil)

// --- helpers ---

func makeTestModel(code string, status domain.ModelStatus) *domain.Model {
	return &domain.Model{
		ID:     uuid.New(),
		Code:   code,
		Status: status,
	}
}

func makeTestChannel(id uuid.UUID, modelID uuid.UUID, status domain.ChannelStatus, healthScore int, weight int) domain.Channel {
	return domain.Channel{
		ID:             id,
		Name:           "test-channel",
		ModelID:        modelID,
		PoolType:       domain.PoolTypeShared,
		HealthScore:    healthScore,
		HealthStatus:   domain.HealthStatusHealthy,
		Status:         status,
		Weight:         weight,
		MaxConcurrency: 10,
	}
}

func makeTestInstance(id uuid.UUID, channelID uuid.UUID, baseURL, providerRoute string, load int) domain.ChannelInstance {
	return domain.ChannelInstance{
		ID:            id,
		ChannelID:     channelID,
		InstanceType:  "serverless",
		BaseURL:       baseURL,
		ProviderRoute: providerRoute,
		CurrentLoad:   load,
		MaxLoad:       10,
		Status:        domain.InstanceStatusActive,
	}
}

func makeIdentity(tenantID *uuid.UUID) *domain.RequestIdentity {
	return &domain.RequestIdentity{
		APIKeyID:  uuid.New(),
		UserID:    uuid.New(),
		TenantID:  tenantID,
		RequestID: "test-req-1",
	}
}

// --- tests ---

func TestRoute_ModelNotFound(t *testing.T) {
	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return nil, errors.New("no rows")
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "nonexistent")
	if err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
}

func TestRoute_ModelNotActive(t *testing.T) {
	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return makeTestModel("inactive-model", domain.ModelStatusInactive), nil
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "inactive-model")
	if err != ErrModelNotActive {
		t.Errorf("expected ErrModelNotActive, got %v", err)
	}
}

func TestRoute_TenantNotAllowed(t *testing.T) {
	model := makeTestModel("restricted-model", domain.ModelStatusActive)
	tenantID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, code string) (*domain.TenantModel, error) {
			return &domain.TenantModel{IsListed: false}, nil
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(&tenantID), "restricted-model")
	if err != ErrTenantNotAllowed {
		t.Errorf("expected ErrTenantNotAllowed, got %v", err)
	}
}

func TestRoute_TenantNoRecordAllowed(t *testing.T) {
	// A tenant with no tenant_models row inherits the shared catalog
	// (default-open). GetTenantModel reports that absence as pgx.ErrNoRows.
	model := makeTestModel("open-model", domain.ModelStatusActive)
	tenantID := uuid.New()
	chID := uuid.New()
	instID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, code string) (*domain.TenantModel, error) {
			return nil, pgx.ErrNoRows
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tid *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://litellm.example.com", "openai/open-model", 0),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(&tenantID), "open-model")
	if err != nil {
		t.Fatalf("tenant with no tenant_models row should inherit the shared catalog: %v", err)
	}
	if result.Channel.ID != chID {
		t.Errorf("Channel.ID = %s, want %s", result.Channel.ID, chID)
	}
}

func TestRoute_TenantDBErrorFailsClosed(t *testing.T) {
	// A tenant_models lookup failure must never widen access; fail closed.
	model := makeTestModel("active-model", domain.ModelStatusActive)
	tenantID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, code string) (*domain.TenantModel, error) {
			return nil, errors.New("connection refused")
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(&tenantID), "active-model")
	if err != ErrTenantNotAllowed {
		t.Errorf("expected ErrTenantNotAllowed (fail closed), got %v", err)
	}
}

func TestRoute_TenantNilNilFailsClosed(t *testing.T) {
	// Defense-in-depth: if GetTenantModel ever returns (nil, nil) instead of
	// the documented (nil, pgx.ErrNoRows), the gate must fail closed rather
	// than silently inheriting the shared catalog.
	model := makeTestModel("active-model", domain.ModelStatusActive)
	tenantID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, code string) (*domain.TenantModel, error) {
			return nil, nil
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(&tenantID), "active-model")
	if err != ErrTenantNotAllowed {
		t.Errorf("expected ErrTenantNotAllowed (fail closed on nil,nil), got %v", err)
	}
}

func TestRoute_NoChannelsAvailable(t *testing.T) {
	model := makeTestModel("active-model", domain.ModelStatusActive)

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return nil, nil
		},
	}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "active-model")
	if err != ErrNoChannelAvailable {
		t.Errorf("expected ErrNoChannelAvailable, got %v", err)
	}
}

func TestRoute_AllChannelsUnroutable(t *testing.T) {
	model := makeTestModel("active-model", domain.ModelStatusActive)
	chID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusInactive, 100, 100),
				makeTestChannel(uuid.New(), model.ID, domain.ChannelStatusDisabled, 0, 50),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "active-model")
	if err != ErrNoChannelAvailable {
		t.Errorf("expected ErrNoChannelAvailable, got %v", err)
	}
}

func TestRoute_NoInstances(t *testing.T) {
	model := makeTestModel("active-model", domain.ModelStatusActive)
	chID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return nil, nil
		},
	}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "active-model")
	if err != ErrNoChannelAvailable {
		t.Errorf("expected ErrNoChannelAvailable, got %v", err)
	}
}

func TestRoute_SuccessfulRoute(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	chID := uuid.New()
	instID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://litellm.example.com", "openai/gpt-4o", 5),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(nil), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Channel.ID != chID {
		t.Errorf("Channel.ID = %s, want %s", result.Channel.ID, chID)
	}
	if result.Instance.ID != instID {
		t.Errorf("Instance.ID = %s, want %s", result.Instance.ID, instID)
	}
	if result.UpstreamModel != "openai/gpt-4o" {
		t.Errorf("UpstreamModel = %s, want openai/gpt-4o", result.UpstreamModel)
	}
}

func TestRoute_WeightedSelection(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)

	chA := makeTestChannel(uuid.New(), model.ID, domain.ChannelStatusActive, 100, 100)
	chB := makeTestChannel(uuid.New(), model.ID, domain.ChannelStatusActive, 90, 50)
	chA.MaxConcurrency = 10
	chB.MaxConcurrency = 10

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{chA, chB}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(uuid.New(), channelID, "https://litellm.example.com", "openai/gpt-4o", 5),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(nil), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Channel.ID != chA.ID {
		t.Errorf("Channel.ID = %s, want %s (higher weight should win)", result.Channel.ID, chA.ID)
	}
}

func TestRoute_LowestLoadInstanceSelected(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	chID := uuid.New()
	lowLoadInst := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 2)
	highLoadInst := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 8)

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{highLoadInst, lowLoadInst}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(nil), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Instance.ID != lowLoadInst.ID {
		t.Errorf("Instance.ID = %s, want %s (lowest load should win)", result.Instance.ID, lowLoadInst.ID)
	}
}

type mockLoadSource struct {
	loads map[uuid.UUID]int64
	err   error
}

func (m mockLoadSource) Load(ctx context.Context, instanceID uuid.UUID) (int64, error) {
	if m.err != nil {
		return 0, m.err
	}
	return m.loads[instanceID], nil
}

func TestRoute_RedisLoadOverridesDBLoad(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	chID := uuid.New()
	// DB says A is idle (2) and B is loaded (8); Redis says the opposite.
	instA := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 2)
	instB := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 8)

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{instA, instB}, nil
		},
	}
	router := NewRouter(models, channels)
	router.SetLoadSource(mockLoadSource{loads: map[uuid.UUID]int64{
		instA.ID: 10, // A is actually busy in Redis
		instB.ID: 1,  // B is actually free
	}})

	result, err := router.Route(context.Background(), makeIdentity(nil), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Instance.ID != instB.ID {
		t.Errorf("Instance.ID = %s, want %s (Redis load must override DB current_load)", result.Instance.ID, instB.ID)
	}
}

func TestRoute_LoadSourceError_FallsBackToDBLoad(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	chID := uuid.New()
	lowLoadInst := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 2)
	highLoadInst := makeTestInstance(uuid.New(), chID, "https://litellm.example.com", "openai/gpt-4o", 8)

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{highLoadInst, lowLoadInst}, nil
		},
	}
	router := NewRouter(models, channels)
	router.SetLoadSource(mockLoadSource{err: errors.New("redis down")})

	result, err := router.Route(context.Background(), makeIdentity(nil), "gpt-4o")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Instance.ID != lowLoadInst.ID {
		t.Errorf("Instance.ID = %s, want %s (DB fallback on load source error)", result.Instance.ID, lowLoadInst.ID)
	}
}

func TestRoute_TenantAllowed(t *testing.T) {
	model := makeTestModel("tenant-model", domain.ModelStatusActive)
	tenantID := uuid.New()
	chID := uuid.New()
	instID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, code string) (*domain.TenantModel, error) {
			return &domain.TenantModel{IsListed: true}, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tid *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://tenant.example.com", "openai/gpt-4o-tenant", 1),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(&tenantID), "tenant-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Channel.ID != chID {
		t.Errorf("Channel.ID = %s, want %s", result.Channel.ID, chID)
	}
	if result.UpstreamModel != "openai/gpt-4o-tenant" {
		t.Errorf("UpstreamModel = %s, want openai/gpt-4o-tenant", result.UpstreamModel)
	}
}

func TestRoute_BetaModelIsCallable(t *testing.T) {
	model := makeTestModel("beta-model", domain.ModelStatusBeta)
	chID := uuid.New()
	instID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://litellm.example.com", "openai/beta-model", 3),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	result, err := router.Route(context.Background(), makeIdentity(nil), "beta-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.UpstreamModel != "openai/beta-model" {
		t.Errorf("UpstreamModel = %s, want openai/beta-model", result.UpstreamModel)
	}
}

func TestRoute_DegradedChannelIsRoutable(t *testing.T) {
	model := makeTestModel("active-model", domain.ModelStatusActive)
	chID := uuid.New()
	instID := uuid.New()

	ch := makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 60, 100)
	ch.HealthStatus = domain.HealthStatusDegraded

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{ch}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://litellm.example.com", "openai/model", 5),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "active-model")
	if err != nil {
		t.Fatalf("degraded channel should still be routable: %v", err)
	}
}

func TestRoute_UnhealthyChannelNotRoutable(t *testing.T) {
	model := makeTestModel("active-model", domain.ModelStatusActive)

	ch := makeTestChannel(uuid.New(), model.ID, domain.ChannelStatusActive, 30, 100)
	ch.HealthStatus = domain.HealthStatusUnhealthy

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{ch}, nil
		},
	}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "active-model")
	if err != ErrNoChannelAvailable {
		t.Errorf("expected ErrNoChannelAvailable, got %v", err)
	}
}

func TestRoute_DBErrorIsNotKeyNotFound(t *testing.T) {
	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return nil, errors.New("connection refused")
		},
	}
	channels := &mockChannelRepo{}
	router := NewRouter(models, channels)

	_, err := router.Route(context.Background(), makeIdentity(nil), "model")
	if err != ErrModelNotFound {
		t.Errorf("expected ErrModelNotFound (current behavior), got %v", err)
	}
}

func TestRoute_WithNoAuthIdentity(t *testing.T) {
	model := makeTestModel("public-model", domain.ModelStatusActive)
	chID := uuid.New()
	instID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				makeTestChannel(chID, model.ID, domain.ChannelStatusActive, 100, 100),
			}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{
				makeTestInstance(instID, chID, "https://litellm.example.com", "openai/public-model", 0),
			}, nil
		},
	}
	router := NewRouter(models, channels)

	identity := &domain.RequestIdentity{
		APIKeyID:  uuid.New(),
		UserID:    uuid.New(),
		TenantID:  nil,
		RequestID: "test",
	}

	_, err := router.Route(context.Background(), identity, "public-model")
	if err != nil {
		t.Fatalf("unexpected error for public model: %v", err)
	}
}

func makeTestChannelWithPool(id uuid.UUID, modelID uuid.UUID, status domain.ChannelStatus, healthScore int, weight int, poolType domain.PoolType) domain.Channel {
	return domain.Channel{
		ID:             id,
		Name:           "test-channel",
		ModelID:        modelID,
		PoolType:       poolType,
		HealthScore:    healthScore,
		HealthStatus:   domain.HealthStatusHealthy,
		Status:         status,
		Weight:         weight,
		MaxConcurrency: 10,
	}
}

func TestSelectWeightedLeastLoad(t *testing.T) {
	// score = weight / (maxConcurrency + 1)
	chA := domain.Channel{Weight: 100, MaxConcurrency: 10} // 100/11 ≈ 9.09
	chB := domain.Channel{Weight: 50, MaxConcurrency: 5}   // 50/6 ≈ 8.33
	chC := domain.Channel{Weight: 10, MaxConcurrency: 1}   // 10/2 = 5

	result := selectWeightedLeastLoad([]domain.Channel{chA, chB, chC})
	if result.Weight != 100 {
		t.Errorf("Weight = %d, want 100 (highest score should win)", result.Weight)
	}

	// Edge case: equal scores → first one wins
	chX := domain.Channel{ID: uuid.New(), Weight: 10, MaxConcurrency: 0}
	chY := domain.Channel{ID: uuid.New(), Weight: 10, MaxConcurrency: 0}

	result = selectWeightedLeastLoad([]domain.Channel{chX, chY})
	if result.ID != chX.ID {
		t.Errorf("ID = %s, want %s (first of equal scores)", result.ID, chX.ID)
	}
}
