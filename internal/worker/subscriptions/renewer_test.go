package subscriptions

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/wallet"
	subscriptionsvc "github.com/deeptrols/api/internal/service/subscription"
	"github.com/google/uuid"
)

func TestRenewer_RenewsDueSubscription(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@renew.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), userID); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days)
		 VALUES ($1, 'Renew Pro', '10', 30)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	subID := uuid.New()
	expiresAt := time.Now().UTC().Add(6 * time.Hour)
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status, auto_renew)
		 VALUES ($1, $2, $3, 'Renew Pro', '10', NOW(), $4, 'active', TRUE)`,
		subID, userID, planID, expiresAt); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	wallets := wallet.NewPostgresRepository(pool)
	svc := subscriptionsvc.New(pool)
	renewer := NewRenewer(pool, wallets, svc)

	n, err := renewer.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 renewal, got %d", n)
	}

	// Wallet debited and a new (stacked) subscription row exists.
	var balance string
	_ = pool.QueryRow(ctx, `SELECT balance::text FROM wallets WHERE user_id = $1`, userID).Scan(&balance)
	if balance != "90.000000" {
		t.Fatalf("balance = %s, want 90", balance)
	}
	var count int
	_ = pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_subscriptions WHERE user_id = $1`, userID).Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 subscription rows after renewal, got %d", count)
	}

	// Replay: nothing more to renew (the due sub is out of the window now and
	// the idempotency key protects the period).
	n2, err := renewer.Run(ctx)
	if err != nil {
		t.Fatalf("Run replay: %v", err)
	}
	if n2 != 0 {
		t.Fatalf("expected 0 renewals on replay, got %d", n2)
	}
}

func TestRenewer_SkipsInsufficientBalance(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@poor.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '1', '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), userID); err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days)
		 VALUES ($1, 'Expensive', '50', 30)`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status, auto_renew)
		 VALUES ($1, $2, $3, 'Expensive', '50', NOW(), NOW() + INTERVAL '6 hours', 'active', TRUE)`,
		uuid.New(), userID, planID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	n, err := NewRenewer(pool, wallet.NewPostgresRepository(pool), subscriptionsvc.New(pool)).Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 renewals with insufficient balance, got %d", n)
	}
}
