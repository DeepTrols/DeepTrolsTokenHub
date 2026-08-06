// Package ratelimit provides pluggable fixed-window rate limiters.
//
// A limiter decides whether a request identified by a key is allowed within
// a window, and how long the caller must wait when it is not. Implementations
// back this with in-memory state or Redis; a FallbackRateLimiter composes them
// for graceful degradation when Redis is unavailable.
package ratelimit

import (
	"context"
	"time"
)

// RateLimiter decides whether a request keyed by `key` may proceed.
//
// Implementations must be safe for concurrent use. On allowed=false, retryAfter
// reports how long to wait before the window resets.
type RateLimiter interface {
	Allow(ctx context.Context, key string, limit int, window time.Duration) (allowed bool, retryAfter time.Duration, err error)
}
