package lease

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Acquire attempts to acquire a distributed lease with the given TTL using
// Redis SET NX EX. Returns true when this instance holds the lease.
//
// When redis is nil (Redis not configured/unavailable) the lease is always
// granted — single-instance mode where every worker runs every cycle.
// On Redis errors the lease is NOT granted (fail-closed: the cycle is skipped
// rather than risk duplicate execution on a degraded cluster).
func Acquire(ctx context.Context, redis *redis.Client, key string, ttl time.Duration) (bool, error) {
	if redis == nil {
		return true, nil
	}
	ok, err := redis.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}
	return ok, nil
}
