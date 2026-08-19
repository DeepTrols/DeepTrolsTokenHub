package gateway

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// ErrLoadTrackingDisabled is returned by Load when no Redis client is wired.
// Callers fall back to the database current_load in that case.
var ErrLoadTrackingDisabled = fmt.Errorf("load tracking disabled (no Redis)")

const loadKeyPrefix = "ai:channel:load:"

// Load scripts: acquire INCRs atomically; release DECRs with a floor at zero
// (a double release must never drive the counter negative). TTLs are applied
// with the non-script PExpire command: some Redis-compatible servers do not
// honor PEXPIRE inside EVAL, and the TTL is refreshed periodically anyway.
var (
	loadAcquireScript = redis.NewScript(`return redis.call('INCR', KEYS[1])`)
	loadReleaseScript = redis.NewScript(`
local v = redis.call('GET', KEYS[1])
if not v then return 0 end
local n = redis.call('DECR', KEYS[1])
if n < 0 then redis.call('DEL', KEYS[1]) return 0 end
return n`)
)

// LoadTracker keeps per-instance in-flight request counts in Redis
// (ai:channel:load:<instance_id>). It is the real-time source for
// least-load routing: the database current_load column is never maintained at
// runtime, so without this tracker load-aware routing has no data to read.
type LoadTracker struct {
	client redis.UniversalClient
	ttl    time.Duration
}

// NewLoadTracker builds a tracker. A nil client disables tracking: Acquire
// becomes a no-op returning a nil hold and Load returns ErrLoadTrackingDisabled.
// A typed-nil pointer (e.g. a nil *goredis.Client wrapped in the interface, as
// wired by app.go when Redis is unavailable) is normalized to nil so the
// disabled path works: an interface holding a typed nil is not == nil.
func NewLoadTracker(client redis.UniversalClient, ttl time.Duration) *LoadTracker {
	client = normalizeLoadClient(client)
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &LoadTracker{client: client, ttl: ttl}
}

// normalizeLoadClient returns nil for both untyped-nil and typed-nil clients
// (pointer/interface kinds that are nil), so callers do not need to care which
// flavor they hold.
func normalizeLoadClient(client redis.UniversalClient) redis.UniversalClient {
	if client == nil {
		return nil
	}
	v := reflect.ValueOf(client)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		if v.IsNil() {
			return nil
		}
	}
	return client
}

func loadKey(instanceID uuid.UUID) string {
	return loadKeyPrefix + instanceID.String()
}

// Acquire increments the in-flight counter for the instance and starts a
// heartbeat that refreshes the TTL until Release. On a disabled tracker it
// returns (nil, nil): callers treat a nil hold as "not tracked". Errors are
// returned so callers can decide whether to fail closed or proceed untracked.
func (t *LoadTracker) Acquire(ctx context.Context, instanceID uuid.UUID) (*LoadHold, error) {
	if t.client == nil {
		return nil, nil
	}
	key := loadKey(instanceID)
	if _, err := loadAcquireScript.Run(ctx, t.client, []string{key}).Result(); err != nil {
		return nil, fmt.Errorf("load acquire: %w", err)
	}
	// Best-effort TTL: a failure here only means the counter can outlive a
	// crashed process; the heartbeat keeps refreshing it while in flight.
	if err := t.client.PExpire(ctx, key, t.ttl).Err(); err != nil {
		log.Printf("gateway: load acquire ttl error key=%s: %v", key, err)
	}
	h := &LoadHold{tracker: t, key: key, stop: make(chan struct{})}
	h.startHeartbeat()
	return h, nil
}

// LoadHold is a live in-flight counter reservation for one upstream attempt.
// Release must be called exactly once (a deferred call in the caller); a
// double release is tolerated by the script's floor-at-zero.
type LoadHold struct {
	tracker *LoadTracker
	key     string
	stop    chan struct{}
	once    sync.Once
}

// startHeartbeat refreshes the key TTL every ttl/2 until Release, so long
// streaming requests never lose their counter to expiry while the process is
// alive. The goroutine exits when Release closes stop.
func (h *LoadHold) startHeartbeat() {
	if h.tracker == nil {
		return
	}
	interval := h.tracker.ttl / 2
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-h.stop:
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				_ = h.tracker.client.PExpire(ctx, h.key, h.tracker.ttl).Err()
				cancel()
			}
		}
	}()
}

// Release decrements the counter and stops the heartbeat. It runs with a
// detached context so a cancelled request context can never leak a counter.
func (h *LoadHold) Release() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		close(h.stop)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if _, err := loadReleaseScript.Run(ctx, h.tracker.client, []string{h.key}).Result(); err != nil {
			log.Printf("gateway: load release error key=%s: %v", h.key, err)
		}
	})
}

// Load returns the current in-flight count for the instance (0 when the key
// is missing or has expired after a crashed process).
func (t *LoadTracker) Load(ctx context.Context, instanceID uuid.UUID) (int64, error) {
	if t.client == nil {
		return 0, ErrLoadTrackingDisabled
	}
	v, err := t.client.Get(ctx, loadKey(instanceID)).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("load get: %w", err)
	}
	return v, nil
}
