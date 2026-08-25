package minutebucket

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// reserveScript atomically increments the request and token counters and
// rolls back when either limit is exceeded.
var reserveScript = goredis.NewScript(`
local requests = redis.call('HINCRBY', KEYS[1], 'requests', 1)
local tokens = redis.call('HINCRBY', KEYS[1], 'tokens', tonumber(ARGV[1]))
redis.call('EXPIRE', KEYS[1], 120)
local rpm = tonumber(ARGV[2])
local tpm = tonumber(ARGV[3])
if rpm > 0 and requests > rpm then
  redis.call('HINCRBY', KEYS[1], 'requests', -1)
  redis.call('HINCRBY', KEYS[1], 'tokens', -tonumber(ARGV[1]))
  return 0
end
if tpm > 0 and tokens > tpm then
  redis.call('HINCRBY', KEYS[1], 'requests', -1)
  redis.call('HINCRBY', KEYS[1], 'tokens', -tonumber(ARGV[1]))
  return 0
end
return 1
`)

// RedisStore is the Redis-backed minute bucket implementation.
type RedisStore struct {
	client *goredis.Client
}

// NewRedisStore returns a Redis-backed minute bucket store.
func NewRedisStore(client *goredis.Client) *RedisStore {
	return &RedisStore{client: client}
}

func (s *RedisStore) Reserve(ctx context.Context, keyID string, tokens int64, rpmLimit int, tpmLimit int64, now time.Time) (Result, error) {
	key := bucketKey(keyID, bucketMinute(now))
	allowed, err := reserveScript.Run(ctx, s.client, []string{key}, tokens, rpmLimit, tpmLimit).Int()
	if err != nil {
		return Result{}, fmt.Errorf("minute bucket reserve: %w", err)
	}
	fields, err := s.client.HMGet(ctx, key, "requests", "tokens").Result()
	if err != nil {
		return Result{}, fmt.Errorf("minute bucket read: %w", err)
	}
	var requests, used int64
	if len(fields) == 2 {
		if v, ok := fields[0].(string); ok {
			fmt.Sscanf(v, "%d", &requests)
		}
		if v, ok := fields[1].(string); ok {
			fmt.Sscanf(v, "%d", &used)
		}
	}
	return Result{Allowed: allowed == 1, Requests: requests, Tokens: used}, nil
}
