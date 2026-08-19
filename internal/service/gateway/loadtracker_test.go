package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

// newLoadTestRedis starts an in-process Redis (miniredis) so load-tracker
// tests run everywhere, including CI, without a live Redis dependency. Keys
// are unique per instance ID so parallel tests never collide.
func newLoadTestRedis(t *testing.T) *goredis.Client {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return client
}

func TestLoadTracker_AcquireRelease(t *testing.T) {
	client := newLoadTestRedis(t)
	tracker := NewLoadTracker(client, 60*time.Second)
	id := uuid.New()
	t.Cleanup(func() { client.Del(context.Background(), loadKey(id)) })

	ctx := context.Background()
	if got, err := tracker.Load(ctx, id); err != nil || got != 0 {
		t.Fatalf("Load before acquire = %d, err=%v, want 0,nil", got, err)
	}

	h1, err := tracker.Acquire(ctx, id)
	if err != nil {
		t.Fatalf("Acquire 1: %v", err)
	}
	defer h1.Release()
	h2, err := tracker.Acquire(ctx, id)
	if err != nil {
		t.Fatalf("Acquire 2: %v", err)
	}
	defer h2.Release()

	if got, _ := tracker.Load(ctx, id); got != 2 {
		t.Errorf("Load after 2 acquires = %d, want 2", got)
	}
	h1.Release()
	if got, _ := tracker.Load(ctx, id); got != 1 {
		t.Errorf("Load after 1 release = %d, want 1", got)
	}
	h2.Release()
	if got, _ := tracker.Load(ctx, id); got != 0 {
		t.Errorf("Load after 2 releases = %d, want 0", got)
	}
}

func TestLoadTracker_DoubleRelease_NoNegative(t *testing.T) {
	client := newLoadTestRedis(t)
	tracker := NewLoadTracker(client, 60*time.Second)
	id := uuid.New()
	t.Cleanup(func() { client.Del(context.Background(), loadKey(id)) })

	ctx := context.Background()
	h, err := tracker.Acquire(ctx, id)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	h.Release()
	h.Release() // double release must not drive the counter negative

	if got, err := tracker.Load(ctx, id); err != nil || got != 0 {
		t.Errorf("Load after double release = %d, err=%v, want 0,nil", got, err)
	}
}

func TestLoadTracker_AcquireSetsTTL(t *testing.T) {
	client := newLoadTestRedis(t)
	tracker := NewLoadTracker(client, 60*time.Second)
	id := uuid.New()
	t.Cleanup(func() { client.Del(context.Background(), loadKey(id)) })

	ctx := context.Background()
	h, err := tracker.Acquire(ctx, id)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release()

	ttl := client.TTL(ctx, loadKey(id)).Val()
	if ttl <= 0 || ttl > 60*time.Second {
		t.Errorf("key TTL = %v, want in (0, 60s]", ttl)
	}
}

func TestLoadTracker_HeartbeatKeepsKeyAlive(t *testing.T) {
	client := newLoadTestRedis(t)
	tracker := NewLoadTracker(client, 2*time.Second)
	id := uuid.New()
	t.Cleanup(func() { client.Del(context.Background(), loadKey(id)) })

	ctx := context.Background()
	h, err := tracker.Acquire(ctx, id)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release()

	// Wait past the TTL; the heartbeat (ttl/2 interval) must have refreshed it.
	time.Sleep(3 * time.Second)
	if got, _ := tracker.Load(ctx, id); got != 1 {
		t.Errorf("Load after TTL + heartbeat = %d, want 1 (counter must survive)", got)
	}
}

func TestLoadTracker_Disabled(t *testing.T) {
	tracker := NewLoadTracker(nil, 60*time.Second)
	ctx := context.Background()
	id := uuid.New()

	h, err := tracker.Acquire(ctx, id)
	if err != nil || h != nil {
		t.Fatalf("Acquire on disabled tracker = (%v, %v), want (nil, nil)", h, err)
	}
	if _, err := tracker.Load(ctx, id); !errors.Is(err, ErrLoadTrackingDisabled) {
		t.Errorf("Load on disabled tracker err = %v, want ErrLoadTrackingDisabled", err)
	}
}

// TestLoadTracker_TypedNilClient_Disabled covers the wiring in app.go: when
// Redis is unavailable the App still holds a typed-nil *goredis.Client, and
// passing that through the redis.UniversalClient interface must disable
// tracking exactly like an untyped nil (an interface holding a typed nil is
// not == nil, so the constructor must normalize it).
func TestLoadTracker_TypedNilClient_Disabled(t *testing.T) {
	var typedNil *goredis.Client
	tracker := NewLoadTracker(typedNil, 60*time.Second)
	ctx := context.Background()
	id := uuid.New()

	h, err := tracker.Acquire(ctx, id)
	if err != nil || h != nil {
		t.Fatalf("Acquire on typed-nil client = (%v, %v), want (nil, nil)", h, err)
	}
	if _, err := tracker.Load(ctx, id); !errors.Is(err, ErrLoadTrackingDisabled) {
		t.Errorf("Load on typed-nil client err = %v, want ErrLoadTrackingDisabled", err)
	}
}

func TestLoadTracker_LoadMissingKey(t *testing.T) {
	client := newLoadTestRedis(t)
	tracker := NewLoadTracker(client, 60*time.Second)
	id := uuid.New()
	t.Cleanup(func() { client.Del(context.Background(), loadKey(id)) })

	if got, err := tracker.Load(context.Background(), id); err != nil || got != 0 {
		t.Errorf("Load on missing key = %d, err=%v, want 0,nil", got, err)
	}
}
