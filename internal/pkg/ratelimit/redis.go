package ratelimit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// fixedWindowScript atomically increments the counter and sets the expiry on
// the first call — or when the key pre-exists without a TTL (operator-set) —
// returning the current count and remaining TTL.
//
//	INCR key
//	ttl = TTL key
//	if count == 1 or ttl == -1 then EXPIRE key window end
//	return {count, TTL}
const fixedWindowScript = `
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('TTL', KEYS[1])
if current == 1 or ttl == -1 then
  redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return {current, redis.call('TTL', KEYS[1])}
`

// RedisRateLimiter is a fixed-window limiter backed by Redis, safe across
// multiple API instances. Window state survives process restarts.
type RedisRateLimiter struct {
	rdb    *redis.Client
	script *redis.Script
}

// NewRedisRateLimiter returns a limiter that stores counts in `rdb`.
func NewRedisRateLimiter(rdb *redis.Client) *RedisRateLimiter {
	return &RedisRateLimiter{
		rdb:    rdb,
		script: redis.NewScript(fixedWindowScript),
	}
}

// Allow increments the counter for `key` and applies the `limit` for `window`.
// Returns an error when Redis is unreachable so callers can degrade gracefully.
func (r *RedisRateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	if limit <= 0 || window <= 0 {
		return false, 0, nil
	}

	windowSecs := int(window.Seconds())
	if windowSecs < 1 {
		windowSecs = 1
	}

	res, err := r.script.Run(ctx, r.rdb, []string{key}, windowSecs).Result()
	if err != nil {
		return false, 0, fmt.Errorf("ratelimit: redis allow: %w", err)
	}

	items, ok := res.([]interface{})
	if !ok || len(items) < 2 {
		return false, 0, errors.New("ratelimit: unexpected redis script result")
	}

	count, ok1 := items[0].(int64)
	ttl, ok2 := items[1].(int64)
	if !ok1 || !ok2 {
		return false, 0, errors.New("ratelimit: unexpected redis script result types")
	}

	if count > int64(limit) {
		if ttl < 0 {
			ttl = 0
		}
		return false, time.Duration(ttl) * time.Second, nil
	}
	return true, 0, nil
}
