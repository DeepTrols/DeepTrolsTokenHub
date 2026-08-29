package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/deeptrols/api/internal/domain"
	goredis "github.com/redis/go-redis/v9"
)

// affinityTTL is how long a remembered channel stays sticky (new-api's
// channel-affinity default). The binding self-heals: when the remembered
// channel disappears from the healthy candidate set it is simply ignored.
const affinityTTL = time.Hour

const affinityNamespace = "deeptrols:affinity:v1"

// AffinityStore remembers the last channel chosen for a routing key (user +
// model) so subsequent requests prefer the same upstream, improving upstream
// cache-hit rates (new-api channel-affinity parity).
type AffinityStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, channelID string, ttl time.Duration) error
}

// RedisAffinityStore is the multi-instance affinity store.
type RedisAffinityStore struct {
	client *goredis.Client
	ttl    time.Duration
}

func (s *RedisAffinityStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, affinityNamespace+":"+key).Result()
}

func (s *RedisAffinityStore) Set(ctx context.Context, key, channelID string, ttl time.Duration) error {
	return s.client.Set(ctx, affinityNamespace+":"+key, channelID, ttl).Err()
}

// MemoryAffinityStore is the single-process fallback (also used by tests).
type MemoryAffinityStore struct {
	mu    sync.Mutex
	items map[string]memoryAffinityItem
	ttl   time.Duration
}

type memoryAffinityItem struct {
	channelID string
	expiresAt time.Time
}

func NewMemoryAffinityStore() *MemoryAffinityStore {
	return &MemoryAffinityStore{items: map[string]memoryAffinityItem{}, ttl: affinityTTL}
}

func (s *MemoryAffinityStore) Get(ctx context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[key]
	if !ok {
		return "", nil
	}
	if time.Now().After(item.expiresAt) {
		delete(s.items, key)
		return "", nil
	}
	return item.channelID, nil
}

func (s *MemoryAffinityStore) Set(ctx context.Context, key, channelID string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.ttl
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = memoryAffinityItem{channelID: channelID, expiresAt: time.Now().Add(ttl)}
	return nil
}

// NewAffinityStore picks Redis when available, otherwise an in-memory store
// (single-node deployments and tests).
func NewAffinityStore(client *goredis.Client, ttl time.Duration) AffinityStore {
	if client != nil {
		return &RedisAffinityStore{client: client, ttl: ttl}
	}
	return NewMemoryAffinityStore()
}

func affinityKey(userID, model string) string {
	return userID + ":" + model
}

// applyAffinity moves the remembered channel to the front of the candidate
// list when it is still a healthy candidate; otherwise it is ignored.
func applyAffinity(ctx context.Context, store AffinityStore, userID, model string, candidates []domain.Channel) []domain.Channel {
	if store == nil || len(candidates) <= 1 {
		return candidates
	}
	channelID, err := store.Get(ctx, affinityKey(userID, model))
	if err != nil || channelID == "" {
		return candidates
	}
	for i, ch := range candidates {
		if ch.ID.String() == channelID {
			out := make([]domain.Channel, 0, len(candidates))
			out = append(out, ch)
			out = append(out, candidates[:i]...)
			out = append(out, candidates[i+1:]...)
			return out
		}
	}
	return candidates
}
