package console

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/google/uuid"
)

func createKeyWithGroup(t *testing.T, a *app.App, userID string, group string, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys",
		strings.NewReader(`{"name":"group-key","group_name":"`+group+`"}`))
	if admin {
		req = setAdminCtx(req, userID)
	} else {
		req = setUserInWalletContext(req, userID)
	}
	w := httptest.NewRecorder()
	HandleCreateAPIKey(a).ServeHTTP(w, req)
	return w
}

func TestCreateAPIKey_GroupRequiresSubscription(t *testing.T) {
	a := appForAPIKeyTest(t)
	user := seedUserForAPIKeyTest(t, a, "group@example.com", "pass12345", "Group User")

	// No subscription → group denied.
	w := createKeyWithGroup(t, a, user.ID.String(), "pro", false)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}

	// Plan granting the group + active subscription → allowed.
	planID := uuid.New()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO subscription_plans (id, name, price, duration_days, group_name)
		 VALUES ($1, 'Pro', '10', 30, 'pro')`, planID); err != nil {
		t.Fatalf("seed plan: %v", err)
	}
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status)
		 VALUES ($1, $2, $3, 'Pro', '10', NOW(), NOW() + INTERVAL '30 days', 'active')`,
		uuid.New(), user.ID, planID); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}
	w2 := createKeyWithGroup(t, a, user.ID.String(), "pro", false)
	if w2.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w2.Code, w2.Body.String())
	}

	// Expired subscription no longer grants the group.
	if _, err := a.Pool.Exec(context.Background(),
		`UPDATE user_subscriptions SET status = 'expired', expires_at = NOW() - INTERVAL '1 day'
		 WHERE user_id = $1`, user.ID); err != nil {
		t.Fatalf("expire subscription: %v", err)
	}
	w3 := createKeyWithGroup(t, a, user.ID.String(), "pro", false)
	if w3.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for expired subscription; body = %s", w3.Code, w3.Body.String())
	}
}

func TestCreateAPIKey_AdminBypassesGroupGate(t *testing.T) {
	a := appForAPIKeyTest(t)
	admin := seedUserForAPIKeyTest(t, a, "group-admin@example.com", "pass12345", "Group Admin")
	w := createKeyWithGroup(t, a, admin.ID.String(), "enterprise", true)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", w.Code, w.Body.String())
	}
}

func TestUserMayUseGroup(t *testing.T) {
	a := appForAPIKeyTest(t)
	user := seedUserForAPIKeyTest(t, a, "mayuse@example.com", "pass12345", "May Use")
	if userMayUseGroup(context.Background(), a, user.ID, "pro") {
		t.Fatal("expected no entitlement without subscription")
	}
	planID := uuid.New()
	_, _ = a.Pool.Exec(context.Background(),
		`INSERT INTO subscription_plans (id, name, price, duration_days, group_name)
		 VALUES ($1, 'Pro', '10', 30, 'pro')`, planID)
	_, _ = a.Pool.Exec(context.Background(),
		`INSERT INTO user_subscriptions (id, user_id, plan_id, plan_name, price, starts_at, expires_at, status)
		 VALUES ($1, $2, $3, 'Pro', '10', NOW(), NOW() + INTERVAL '30 days', 'active')`,
		uuid.New(), user.ID, planID)
	if !userMayUseGroup(context.Background(), a, user.ID, "pro") {
		t.Fatal("expected entitlement with active subscription")
	}
	if userMayUseGroup(context.Background(), a, user.ID, "other") {
		t.Fatal("expected no entitlement for another group")
	}
	// Expired subscription revokes the entitlement.
	_, _ = a.Pool.Exec(context.Background(),
		`UPDATE user_subscriptions SET status = 'expired' WHERE user_id = $1`, user.ID)
	if userMayUseGroup(context.Background(), a, user.ID, "pro") {
		t.Fatal("expected no entitlement after expiry")
	}
}
