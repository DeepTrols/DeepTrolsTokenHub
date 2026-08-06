package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestMemoryRateLimiter_AllowsWithinLimit(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 5
	window := time.Minute

	for i := 0; i < limit; i++ {
		allowed, retryAfter, err := limiter.Allow(context.Background(), "key1", limit, window)
		if err != nil {
			t.Fatalf("request %d: unexpected error: %v", i+1, err)
		}
		if !allowed {
			t.Fatalf("request %d: expected allowed, got denied", i+1)
		}
		if retryAfter != 0 {
			t.Errorf("request %d: expected retryAfter 0, got %v", i+1, retryAfter)
		}
	}
}

func TestMemoryRateLimiter_BlocksWhenExceedingLimit(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 3
	window := time.Minute

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "key1", limit, window)
	}

	allowed, retryAfter, err := limiter.Allow(context.Background(), "key1", limit, window)
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

func TestMemoryRateLimiter_IndependentKeys(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 2
	window := time.Minute

	// Exhaust key1.
	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "key1", limit, window)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "key1", limit, window); allowed {
		t.Fatal("key1 should be denied")
	}

	// key2 unaffected.
	allowed, _, err := limiter.Allow(context.Background(), "key2", limit, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("key2 should be allowed")
	}
}

func TestMemoryRateLimiter_WindowExpires(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 2
	window := 50 * time.Millisecond

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "key1", limit, window)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "key1", limit, window); allowed {
		t.Fatal("expected denied before window expiry")
	}

	time.Sleep(window + 20*time.Millisecond)

	allowed, _, err := limiter.Allow(context.Background(), "key1", limit, window)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed after window expiry")
	}
}

func TestMemoryRateLimiter_EmptyKeyIsTracked(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 2
	window := time.Minute

	for i := 0; i < limit; i++ {
		limiter.Allow(context.Background(), "", limit, window)
	}
	if allowed, _, _ := limiter.Allow(context.Background(), "", limit, window); allowed {
		t.Fatal("empty key should be rate limited after exhaustion")
	}
}

func TestMemoryRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 100
	window := time.Minute

	var wg sync.WaitGroup
	for i := 0; i < limit; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiter.Allow(context.Background(), "key1", limit, window)
		}()
	}
	wg.Wait()

	// Exactly limit requests were allowed; the next one is denied.
	allowed, _, _ := limiter.Allow(context.Background(), "key1", limit, window)
	if allowed {
		t.Fatal("expected denied after concurrent exhaustion")
	}
}

func TestMemoryRateLimiter_ZeroLimitDenies(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	allowed, _, err := limiter.Allow(context.Background(), "key1", 0, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("zero limit should deny")
	}
}

func TestMemoryRateLimiter_CloseClearsState(t *testing.T) {
	limiter := NewMemoryRateLimiter()
	limit := 1
	window := time.Minute

	limiter.Allow(context.Background(), "key1", limit, window)
	limiter.Close()

	allowed, _, _ := limiter.Allow(context.Background(), "key1", limit, window)
	if !allowed {
		t.Fatal("expected allowed after Close clears state")
	}
}
