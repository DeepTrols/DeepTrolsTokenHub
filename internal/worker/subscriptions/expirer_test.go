package subscriptions

import (
	"context"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/google/uuid"
)

func TestExpirer_MarksExpiredActive(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)
	ctx := context.Background()

	// User + plan rows are needed for the FK constraints.
	userID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash) VALUES ($1, $2, 'hash')`,
		userID, userID.String()+"@expire.com"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	planID := uuid.New()
	if _, err := pool.Exec(ctx,
		`INSERT INTO subscription_plans (id, name, price, duration_days) VALUES ($1, 'Pro', '10', 30)`,
		planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}

	now := time.Now().UTC()
	seed := func(expires time.Time, status string) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status)
			 VALUES ($1, $2, $3, 'Pro', '10', $4, $5, $6)`,
			uuid.New(), userID, planID, now.Add(-time.Hour), expires, status); err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	}
	seed(now.Add(-time.Minute), "active")  // expired-active → must flip
	seed(now.Add(time.Hour), "active")     // still valid → keep
	seed(now.Add(-time.Minute), "expired") // already terminal → keep

	e := New(pool)
	n, err := e.Run(ctx)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 row flipped, got %d", n)
	}

	var active, expired int
	if err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FILTER (WHERE status='active'), COUNT(*) FILTER (WHERE status='expired')
		 FROM user_subscriptions`).Scan(&active, &expired); err != nil {
		t.Fatalf("count: %v", err)
	}
	if active != 1 || expired != 2 {
		t.Fatalf("active=%d expired=%d, want 1/2", active, expired)
	}
}
