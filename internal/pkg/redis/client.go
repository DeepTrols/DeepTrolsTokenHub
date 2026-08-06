// Package redis provides a small factory for creating a connected Redis client.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// NewClient parses a redis URL, creates a client, and verifies connectivity
// with a Ping before returning. The caller owns Close on the returned client.
//
// Parse errors are deliberately reported generically: redis.ParseURL surfaces
// the full URL (including any password) in its error text, which must not be
// logged.
func NewClient(ctx context.Context, redisURL string) (*redis.Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("redis: invalid connection URL configuration")
	}

	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		client.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return client, nil
}
