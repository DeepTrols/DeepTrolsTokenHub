// Package minutebucket implements per-API-key RPM/TPM minute quota buckets
// for gateway admission. Redis is the primary store (atomic Lua); a
// PostgreSQL table is the fallback when Redis is unavailable.
package minutebucket

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// Result reports the bucket state after a reservation attempt.
type Result struct {
	Allowed  bool
	Requests int64
	Tokens   int64
}

// Store reserves requests/tokens against a key's current minute bucket.
// Limits <= 0 mean unlimited for that dimension.
type Store interface {
	Reserve(ctx context.Context, keyID string, tokens int64, rpmLimit int, tpmLimit int64, now time.Time) (Result, error)
}

// NewStore returns a Redis-backed store when redis is available, otherwise a
// PostgreSQL fallback. Callers must pass the same pool for DB access.
func NewStore(redis *goredis.Client, pgStore Store) Store {
	if redis != nil {
		return &RedisStore{client: redis}
	}
	if pgStore != nil {
		return pgStore
	}
	return nil
}

// bucketMinute formats a time into the bucket key minute (YYYYMMDDHHMM).
func bucketMinute(now time.Time) string {
	return now.UTC().Format("200601021504")
}

// bucketKey is the Redis key for one key's one-minute bucket.
func bucketKey(keyID, minute string) string {
	return "rl:mb:" + keyID + ":" + minute
}

// ErrBucketUnavailable is returned when neither Redis nor the DB fallback is
// configured; admission treats it as degraded (fail-open, logged).
var ErrBucketUnavailable = fmt.Errorf("minute bucket store unavailable")
