package minutebucket

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

func testNow() time.Time {
	return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
}

func TestRedisStore_ReserveLimits(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	store := &RedisStore{client: client}
	ctx := context.Background()

	// rpm=2, tpm=100: two requests allowed, third rejected.
	r1, err := store.Reserve(ctx, "key-1", 40, 2, 100, testNow())
	if err != nil || !r1.Allowed || r1.Requests != 1 || r1.Tokens != 40 {
		t.Fatalf("reserve 1 = %+v err=%v", r1, err)
	}
	r2, _ := store.Reserve(ctx, "key-1", 40, 2, 100, testNow())
	if !r2.Allowed || r2.Requests != 2 || r2.Tokens != 80 {
		t.Fatalf("reserve 2 = %+v", r2)
	}
	r3, _ := store.Reserve(ctx, "key-1", 40, 2, 100, testNow())
	if r3.Allowed {
		t.Fatalf("reserve 3 should be rejected, got %+v", r3)
	}
	if r3.Requests != 2 || r3.Tokens != 80 {
		t.Errorf("rollback counters = %+v, want 2/80", r3)
	}

	// TPM-only limit.
	r4, _ := store.Reserve(ctx, "key-2", 90, 0, 100, testNow())
	if !r4.Allowed {
		t.Fatalf("key-2 first reserve rejected: %+v", r4)
	}
	r5, _ := store.Reserve(ctx, "key-2", 20, 0, 100, testNow())
	if r5.Allowed {
		t.Fatalf("key-2 over TPM should be rejected: %+v", r5)
	}

	// Different minute bucket is independent.
	next := testNow().Add(time.Minute)
	r6, _ := store.Reserve(ctx, "key-1", 40, 2, 100, next)
	if !r6.Allowed || r6.Requests != 1 {
		t.Fatalf("next minute reserve = %+v", r6)
	}
}

func TestPostgresStore_ReserveLimits(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()
	userID := uuid.New()
	keyID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, 'bucket@test.local', 'x')`,
		userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-bucket', 'sk-***')`,
		keyID, userID); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	store := NewPostgresStore(pool)

	r1, err := store.Reserve(ctx, keyID.String(), 50, 1, 100, testNow())
	if err != nil || !r1.Allowed || r1.Requests != 1 {
		t.Fatalf("reserve 1 = %+v err=%v", r1, err)
	}
	r2, err := store.Reserve(ctx, keyID.String(), 50, 1, 100, testNow())
	if err != nil {
		t.Fatalf("reserve 2: %v", err)
	}
	if r2.Allowed {
		t.Fatalf("reserve 2 should be rejected (rpm=1): %+v", r2)
	}
	if r2.Requests != 1 || r2.Tokens != 50 {
		t.Errorf("post-rollback counters = %+v, want 1/50", r2)
	}

	key2 := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-bucket-2', 'sk-***')`,
		key2, userID); err != nil {
		t.Fatalf("seed api key 2: %v", err)
	}
	r3, err := store.Reserve(ctx, key2.String(), 120, 0, 100, testNow())
	if err != nil {
		t.Fatalf("reserve tpm: %v", err)
	}
	if r3.Allowed {
		t.Fatalf("over TPM should be rejected: %+v", r3)
	}
}

func TestRedisStore_SettleAdjustsTokens(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	store := &RedisStore{client: client}
	ctx := context.Background()
	keyID := "key-settle"

	if _, err := store.Reserve(ctx, keyID, 100, 0, 1000, testNow()); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	// Actual usage below estimate → refund.
	if err := store.Settle(ctx, keyID, 100, 60, testNow()); err != nil {
		t.Fatalf("settle refund: %v", err)
	}
	r, _ := store.Reserve(ctx, keyID, 0, 0, 1000, testNow())
	if r.Tokens != 60 {
		t.Errorf("tokens after refund = %d, want 60", r.Tokens)
	}
	// Actual usage above estimate → additional consume.
	if err := store.Settle(ctx, keyID, 60, 200, testNow()); err != nil {
		t.Fatalf("settle consume: %v", err)
	}
	r, _ = store.Reserve(ctx, keyID, 0, 0, 1000, testNow())
	if r.Tokens != 200 {
		t.Errorf("tokens after consume = %d, want 200", r.Tokens)
	}
	// Oversized refund clamps at zero.
	if err := store.Settle(ctx, keyID, 200, 0, testNow()); err != nil {
		t.Fatalf("settle clamp: %v", err)
	}
	r, _ = store.Reserve(ctx, keyID, 0, 0, 1000, testNow())
	if r.Tokens != 0 {
		t.Errorf("tokens after clamp = %d, want 0", r.Tokens)
	}
}

func TestPostgresStore_SettleAdjustsTokens(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()
	userID := uuid.New()
	keyID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, 'settle@test.local', 'x')`,
		userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key) VALUES ($1, $2, 'sk-', 'hash-settle', 'sk-***')`,
		keyID, userID); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	store := NewPostgresStore(pool)

	if _, err := store.Reserve(ctx, keyID.String(), 100, 0, 1000, testNow()); err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if err := store.Settle(ctx, keyID.String(), 100, 70, testNow()); err != nil {
		t.Fatalf("settle: %v", err)
	}
	r, err := store.Reserve(ctx, keyID.String(), 0, 0, 1000, testNow())
	if err != nil {
		t.Fatalf("re-reserve: %v", err)
	}
	if r.Tokens != 70 {
		t.Errorf("tokens after settle = %d, want 70", r.Tokens)
	}
	if err := store.Settle(ctx, keyID.String(), 70, 0, testNow()); err != nil {
		t.Fatalf("settle clamp: %v", err)
	}
	r, _ = store.Reserve(ctx, keyID.String(), 0, 0, 1000, testNow())
	if r.Tokens != 0 {
		t.Errorf("tokens after clamp = %d, want 0", r.Tokens)
	}
}
