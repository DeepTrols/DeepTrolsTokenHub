package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestRedisLimiter(t *testing.T) (*RedisRateLimiter, *miniredis.Miniredis) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(s.Close)

	rdb := goredis.NewClient(&goredis.Options{Addr: s.Addr()})
	t.Cleanup(func() { rdb.Close() })

	return NewRedisRateLimiter(rdb), s
}

func TestRedisRateLimiter_AllowsWithinLimit(t *testing.T) {
	limiter, _ := newTestRedisLimiter(t)
	limit := 5
	window := time.Minute

	for i := 0; i < limit; i++ {
		allowed, _, err := limiter.Allow(context.Background(), "rl:test:key1", limit, window)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("request %d: expected allowed", i+1)
		}
	}
}

func TestRedisRateLimiter_BlocksWhenExceedingLimit(t *testing.T) {
	limiter, _ := newTestRedisLimiter(t)
	limit := 3
	window := time.Minute

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "rl:test:key1", limit, window)
	}

	allowed, retryAfter, err := limiter.Allow(context.Background(), "rl:test:key1", limit, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected denied after limit exceeded")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retryAfter, got %v", retryAfter)
	}
}

func TestRedisRateLimiter_IndependentKeys(t *testing.T) {
	limiter, _ := newTestRedisLimiter(t)
	limit := 2
	window := time.Minute

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "rl:test:key1", limit, window)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "rl:test:key1", limit, window); allowed {
		t.Fatal("key1 should be denied")
	}

	allowed, _, err := limiter.Allow(context.Background(), "rl:test:key2", limit, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("key2 should be allowed")
	}
}

func TestRedisRateLimiter_WindowExpires(t *testing.T) {
	limiter, s := newTestRedisLimiter(t)
	limit := 2
	window := time.Minute

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "rl:test:key1", limit, window)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "rl:test:key1", limit, window); allowed {
		t.Fatal("expected denied before window expiry")
	}

	// Fast-forward past the window.
	s.FastForward(window + time.Second)

	allowed, _, err := limiter.Allow(context.Background(), "rl:test:key1", limit, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed after window expiry")
	}
}

func TestRedisRateLimiter_ReturnsErrorWhenRedisUnavailable(t *testing.T) {
	// Create a client pointing at a closed miniredis.
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	addr := s.Addr()
	s.Close()

	rdb := goredis.NewClient(&goredis.Options{Addr: addr})
	defer rdb.Close()

	limiter := NewRedisRateLimiter(rdb)

	_, _, err = limiter.Allow(context.Background(), "rl:test:key1", 5, time.Minute)
	if err == nil {
		t.Fatal("expected error when Redis is unavailable")
	}
}

func TestRedisRateLimiter_ZeroLimitDenies(t *testing.T) {
	limiter, _ := newTestRedisLimiter(t)
	allowed, _, err := limiter.Allow(context.Background(), "rl:test:key1", 0, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("zero limit should deny")
	}
}
