package gateway

import (
	"context"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/repository/testutil"
	subscriptionsvc "github.com/deeptrols/api/internal/service/subscription"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func TestApplySubscriptionAllowance(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@allow.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days, token_quota)
		 VALUES ($1, 'Quota', '10', 30, 100)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	svc := subscriptionsvc.New(pool)
	if _, err := svc.Activate(ctx, userID, planID); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	a := &app.App{Subscriptions: svc}

	// Inside the quota → cost waived.
	cost, covered := applySubscriptionAllowance(ctx, a, userID.String(), 80, decimal.NewFromInt(10))
	if !covered || !cost.IsZero() {
		t.Fatalf("covered=%v cost=%s", covered, cost)
	}
	// Over the remaining 20 → billed normally.
	cost, covered = applySubscriptionAllowance(ctx, a, userID.String(), 50, decimal.NewFromInt(10))
	if covered || !cost.Equal(decimal.NewFromInt(10)) {
		t.Fatalf("covered=%v cost=%s", covered, cost)
	}
	// Zero cost requests never trigger quota.
	cost, covered = applySubscriptionAllowance(ctx, a, userID.String(), 10, decimal.Zero)
	if covered || !cost.IsZero() {
		t.Fatalf("zero-cost covered=%v", covered)
	}
	// Missing service no-ops.
	if _, covered := applySubscriptionAllowance(ctx, &app.App{}, userID.String(), 10, decimal.NewFromInt(1)); covered {
		t.Fatal("expected no coverage without subscription service")
	}
}
