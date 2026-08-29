package gateway

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
)

func TestMemoryAffinityStore(t *testing.T) {
	store := NewMemoryAffinityStore()
	ctx := context.Background()

	if v, _ := store.Get(ctx, "u:m"); v != "" {
		t.Fatalf("expected empty on miss, got %q", v)
	}
	if err := store.Set(ctx, "u:m", "ch-1", time.Minute); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, _ := store.Get(ctx, "u:m"); v != "ch-1" {
		t.Fatalf("expected ch-1, got %q", v)
	}

	// Expired entries must behave as misses.
	if err := store.Set(ctx, "u:m2", "ch-2", time.Millisecond); err != nil {
		t.Fatalf("set: %v", err)
	}
	time.Sleep(5 * time.Millisecond)
	if v, _ := store.Get(ctx, "u:m2"); v != "" {
		t.Fatalf("expected expired entry to be empty, got %q", v)
	}
}

func TestRoute_PrefersAffinityChannel(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	affinityID := uuid.New()
	weightedID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			weighted := makeTestChannel(weightedID, model.ID, domain.ChannelStatusActive, 100, 100)
			affinity := makeTestChannel(affinityID, model.ID, domain.ChannelStatusActive, 100, 1)
			return []domain.Channel{weighted, affinity}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{makeTestInstance(uuid.New(), channelID, "https://up.example.com", "gpt-4o", 0)}, nil
		},
	}

	identity := makeIdentity(nil)
	store := NewMemoryAffinityStore()
	_ = store.Set(context.Background(), affinityKey(identity.UserID.String(), "gpt-4o"), affinityID.String(), time.Minute)

	router := NewRouter(models, channels)
	router.EnableAffinity(store)
	result, err := router.Route(context.Background(), identity, "gpt-4o")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if result.Channel.ID != affinityID {
		t.Errorf("affinity channel should be preferred despite lower weight, got %s", result.Channel.ID)
	}
}

func TestRoute_RecordsAffinity(t *testing.T) {
	model := makeTestModel("gpt-4o", domain.ModelStatusActive)
	channelID := uuid.New()

	models := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return model, nil
		},
	}
	channels := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{makeTestChannel(channelID, model.ID, domain.ChannelStatusActive, 100, 1)}, nil
		},
		listInstancesFn: func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{makeTestInstance(uuid.New(), channelID, "https://up.example.com", "gpt-4o", 0)}, nil
		},
	}

	identity := makeIdentity(nil)
	store := NewMemoryAffinityStore()
	router := NewRouter(models, channels)
	router.EnableAffinity(store)
	if _, err := router.Route(context.Background(), identity, "gpt-4o"); err != nil {
		t.Fatalf("route: %v", err)
	}
	got, _ := store.Get(context.Background(), affinityKey(identity.UserID.String(), "gpt-4o"))
	if got != channelID.String() {
		t.Fatalf("expected affinity recorded for channel %s, got %q", channelID, got)
	}
}

func TestApplyAffinity_IgnoresUnknownChannel(t *testing.T) {
	store := NewMemoryAffinityStore()
	_ = store.Set(context.Background(), "u:m", "missing-channel", time.Minute)

	a := makeTestChannel(uuid.New(), uuid.New(), domain.ChannelStatusActive, 100, 1)
	b := makeTestChannel(uuid.New(), uuid.New(), domain.ChannelStatusActive, 100, 1)
	out := applyAffinity(context.Background(), store, "u", "m", []domain.Channel{a, b})
	if len(out) != 2 || out[0].ID != a.ID {
		t.Fatalf("unknown remembered channel must be ignored, got %+v", out)
	}
}

func TestRecordAffinity_NoStoreNoop(t *testing.T) {
	router := NewRouter(nil, nil)
	router.RecordAffinity(context.Background(), "u", "m", "ch") // must not panic
}
