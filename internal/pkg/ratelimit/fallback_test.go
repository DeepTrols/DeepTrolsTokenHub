package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// stubLimiter is a programmable RateLimiter for testing fallback behavior.
type stubLimiter struct {
	allowed    bool
	retryAfter time.Duration
	err        error
	callCount  int
}

func (s *stubLimiter) Allow(_ context.Context, _ string, _ int, _ time.Duration) (bool, time.Duration, error) {
	s.callCount++
	return s.allowed, s.retryAfter, s.err
}

func TestFallbackRateLimiter_UsesPrimaryWhenSucceeds(t *testing.T) {
	primary := &stubLimiter{allowed: true, retryAfter: 0}
	fallback := &stubLimiter{allowed: false}

	fb := NewFallbackRateLimiter(primary, fallback)

	allowed, _, err := fb.Allow(context.Background(), "key", 5, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed from primary")
	}
	if fallback.callCount != 0 {
		t.Errorf("fallback should not be called when primary succeeds, got %d calls", fallback.callCount)
	}
}

func TestFallbackRateLimiter_DelegatesToFallbackOnPrimaryError(t *testing.T) {
	primary := &stubLimiter{err: errors.New("redis down")}
	fallback := &stubLimiter{allowed: true}

	fb := NewFallbackRateLimiter(primary, fallback)

	allowed, _, err := fb.Allow(context.Background(), "key", 5, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatal("expected allowed via fallback")
	}
	if fallback.callCount != 1 {
		t.Errorf("fallback should be called once, got %d", fallback.callCount)
	}
}

func TestFallbackRateLimiter_PassesRetryAfterFromPrimary(t *testing.T) {
	primary := &stubLimiter{allowed: false, retryAfter: 30 * time.Second}
	fallback := &stubLimiter{allowed: true}

	fb := NewFallbackRateLimiter(primary, fallback)

	allowed, retryAfter, err := fb.Allow(context.Background(), "key", 5, time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if allowed {
		t.Fatal("expected denied (primary denied)")
	}
	if retryAfter != 30*time.Second {
		t.Errorf("expected retryAfter 30s, got %v", retryAfter)
	}
	if fallback.callCount != 0 {
		t.Errorf("fallback should not be called when primary succeeds (even with denial), got %d", fallback.callCount)
	}
}

func TestFallbackRateLimiter_ReturnsFallbackErrorIfBothFail(t *testing.T) {
	primary := &stubLimiter{err: errors.New("redis down")}
	fallback := &stubLimiter{err: errors.New("fallback down")}

	fb := NewFallbackRateLimiter(primary, fallback)

	_, _, err := fb.Allow(context.Background(), "key", 5, time.Minute)
	if err == nil {
		t.Fatal("expected error when both limiters fail")
	}
}
