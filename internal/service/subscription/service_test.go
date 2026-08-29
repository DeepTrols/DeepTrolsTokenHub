package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

func TestActivate_CreatesAndStacks(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@subsvc.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days) VALUES ($1, 'Pro', '10', 30)`,
		planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	svc := New(pool)
	first, err := svc.Activate(ctx, userID, planID)
	if err != nil {
		t.Fatalf("Activate: %v", err)
	}
	second, err := svc.Activate(ctx, userID, planID)
	if err != nil {
		t.Fatalf("Activate second: %v", err)
	}
	if !second.After(first.Add(29 * 24 * time.Hour)) {
		t.Fatalf("expected stacking (second after first+30d), first=%s second=%s", first, second)
	}

	var count int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_subscriptions WHERE user_id = $1`, userID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestFindEnabled_RejectsDisabled(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days, enabled)
		 VALUES ($1, 'Off', '10', 30, FALSE)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := New(pool).FindEnabled(ctx, planID); err != ErrPlanNotFound {
		t.Fatalf("expected ErrPlanNotFound, got %v", err)
	}
}

func TestQuota_ConsumeAndExhaust(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@quota.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days, token_quota)
		 VALUES ($1, 'Quota Pro', '10', 30, 100)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	svc := New(pool)
	if _, err := svc.Activate(ctx, userID, planID); err != nil {
		t.Fatalf("Activate: %v", err)
	}

	remaining, ok, err := svc.RemainingQuota(ctx, userID)
	if err != nil || !ok || remaining != 100 {
		t.Fatalf("remaining = %d ok=%v err=%v", remaining, ok, err)
	}

	// Consume 40, then 60: total 100 = full allowance.
	if n, err := svc.ConsumeQuota(ctx, userID, 40); err != nil || n != 40 {
		t.Fatalf("consume 40 = %d err=%v", n, err)
	}
	if n, err := svc.ConsumeQuota(ctx, userID, 60); err != nil || n != 60 {
		t.Fatalf("consume 60 = %d err=%v", n, err)
	}
	remaining, ok, _ = svc.RemainingQuota(ctx, userID)
	if !ok || remaining != 0 {
		t.Fatalf("expected 0 remaining, got %d", remaining)
	}

	// Exhausted: a further consume returns 0 (bill normally).
	if n, err := svc.ConsumeQuota(ctx, userID, 10); err != nil || n != 0 {
		t.Fatalf("exhausted consume = %d err=%v", n, err)
	}
}

func TestQuota_ResetAfterPeriod(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@reset.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days, token_quota)
		 VALUES ($1, 'Quota Reset', '10', 30, 50)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	svc := New(pool)
	if _, err := svc.Activate(ctx, userID, planID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	if n, _ := svc.ConsumeQuota(ctx, userID, 50); n != 50 {
		t.Fatalf("initial consume = %d", n)
	}
	// Force the period to lapse.
	if _, err := pool.Exec(ctx,
		`UPDATE user_subscriptions SET quota_reset_at = NOW() - INTERVAL '1 day'
		 WHERE user_id = $1`, userID); err != nil {
		t.Fatalf("lapse period: %v", err)
	}
	remaining, ok, _ := svc.RemainingQuota(ctx, userID)
	if !ok || remaining != 50 {
		t.Fatalf("expected full quota after reset, got %d", remaining)
	}
	if n, _ := svc.ConsumeQuota(ctx, userID, 30); n != 30 {
		t.Fatalf("consume after reset = %d", n)
	}
	var used int64
	_ = pool.QueryRow(ctx,
		`SELECT quota_used FROM user_subscriptions WHERE user_id = $1`, userID).Scan(&used)
	if used != 30 {
		t.Fatalf("quota_used = %d, want 30 (reset before consume)", used)
	}
}
