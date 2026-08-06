package lease

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return client, mr
}

func TestAcquire_FirstGetsLease_SecondRejected(t *testing.T) {
	client, _ := newTestRedis(t)
	ctx := context.Background()

	ok1, err := Acquire(ctx, client, "worker:lease:test", 30*time.Second)
	if err != nil || !ok1 {
		t.Fatalf("first acquire: ok=%v err=%v, want ok", ok1, err)
	}
	ok2, err := Acquire(ctx, client, "worker:lease:test", 30*time.Second)
	if err != nil || ok2 {
		t.Fatalf("second acquire: ok=%v err=%v, want rejected", ok2, err)
	}
}

func TestAcquire_AfterTTL_AvailableAgain(t *testing.T) {
	client, mr := newTestRedis(t)
	ctx := context.Background()

	ok, err := Acquire(ctx, client, "worker:lease:ttl", 1*time.Second)
	if err != nil || !ok {
		t.Fatalf("acquire: ok=%v err=%v", ok, err)
	}
	mr.FastForward(2 * time.Second)
	ok2, err := Acquire(ctx, client, "worker:lease:ttl", 30*time.Second)
	if err != nil || !ok2 {
		t.Fatalf("acquire after TTL: ok=%v err=%v, want ok", ok2, err)
	}
}

func TestAcquire_NilRedis_AlwaysGranted(t *testing.T) {
	// No Redis configured → single-instance mode: every worker runs.
	ok, err := Acquire(context.Background(), nil, "worker:lease:x", time.Second)
	if err != nil || !ok {
		t.Fatalf("nil redis acquire: ok=%v err=%v, want ok", ok, err)
	}
}
