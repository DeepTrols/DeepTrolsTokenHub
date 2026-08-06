package ratelimit

import (
	"context"
	"log"
	"time"
)

// FallbackRateLimiter tries a primary limiter (typically Redis) and, when it
// returns an error, transparently delegates to a fallback (typically memory).
// This provides graceful degradation: an unavailable Redis must never drop a
// request.
type FallbackRateLimiter struct {
	primary  RateLimiter
	fallback RateLimiter
}

// NewFallbackRateLimiter composes a primary and fallback limiter.
func NewFallbackRateLimiter(primary, fallback RateLimiter) *FallbackRateLimiter {
	return &FallbackRateLimiter{
		primary:  primary,
		fallback: fallback,
	}
}

// Allow delegates to the primary limiter, falling back on error.
func (f *FallbackRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	allowed, retryAfter, err := f.primary.Allow(ctx, key, limit, window)
	if err == nil {
		return allowed, retryAfter, nil
	}

	log.Printf("ratelimit: primary limiter failed (%v); using in-memory fallback", err)
	return f.fallback.Allow(ctx, key, limit, window)
}
