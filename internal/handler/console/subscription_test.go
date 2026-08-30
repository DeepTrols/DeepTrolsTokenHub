package console

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func subscriptionApp(t *testing.T) (*app.App, *domain.User) {
	t.Helper()
	a := appForWalletTest(t)
	user := seedUserForWalletTest(t, a, "sub@example.com", "pass12345", "Sub User")
	// Create a wallet with generous balance.
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '1000', '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), user.ID); err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	return a, user
}

func createPlanRequest(t *testing.T, a *app.App, adminID uuid.UUID, name, price string, days int) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscription-plans",
		strings.NewReader(`{"name":"`+name+`","description":"d","price":"`+price+`","duration_days":`+strconv.Itoa(days)+`}`))
	req = setAdminCtx(req, adminID.String())
	w := httptest.NewRecorder()
	HandleCreateSubscriptionPlan(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create plan status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.ID
}

func TestSubscription_PurchaseAndStack(t *testing.T) {
	a, user := subscriptionApp(t)
	admin := seedUserForWalletTest(t, a, "sub-admin@example.com", "pass12345", "Sub Admin")
	planID := createPlanRequest(t, a, admin.ID, "Pro 月付", "10", 30)

	// Purchase 1: deducts balance and activates.
	req := httptest.NewRequest(http.MethodPost, "/api/console/subscription/purchase",
		strings.NewReader(`{"plan_id":"`+planID+`"}`))
	req = setUserInWalletContext(req, user.ID.String())
	w := httptest.NewRecorder()
	HandlePurchaseSubscription(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("purchase status = %d: %s", w.Code, w.Body.String())
	}
	var first struct {
		ExpiresAt string `json:"expires_at"`
		Price     string `json:"price"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &first)
	if first.Price != "10" {
		t.Fatalf("price = %q", first.Price)
	}

	// Balance deducted.
	var balance decimal.Decimal
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT balance FROM wallets WHERE user_id = $1`, user.ID).Scan(&balance)
	if !balance.Equal(decimal.NewFromInt(990)) {
		t.Fatalf("balance = %s, want 990", balance)
	}

	// Purchase 2 stacks onto the first (expiry extends by another 30 days).
	req2 := httptest.NewRequest(http.MethodPost, "/api/console/subscription/purchase",
		strings.NewReader(`{"plan_id":"`+planID+`"}`))
	req2 = setUserInWalletContext(req2, user.ID.String())
	w2 := httptest.NewRecorder()
	HandlePurchaseSubscription(a).ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("purchase 2 status = %d: %s", w2.Code, w2.Body.String())
	}
	var count int
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM user_subscriptions WHERE user_id = $1`, user.ID).Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 subscription rows, got %d", count)
	}

	// Wallet transaction recorded as subscription spend.
	var txType string
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT tx_type FROM wallet_transactions WHERE wallet_id = (SELECT id FROM wallets WHERE user_id = $1)
		 ORDER BY created_at DESC LIMIT 1`, user.ID).Scan(&txType)
	if txType != "subscription" {
		t.Fatalf("tx_type = %q, want subscription", txType)
	}
}

func TestSubscription_InsufficientBalance(t *testing.T) {
	a, user := subscriptionApp(t)
	admin := seedUserForWalletTest(t, a, "sub-admin2@example.com", "pass12345", "Sub Admin 2")
	planID := createPlanRequest(t, a, admin.ID, "Expensive", "5000", 30)

	req := httptest.NewRequest(http.MethodPost, "/api/console/subscription/purchase",
		strings.NewReader(`{"plan_id":"`+planID+`"}`))
	req = setUserInWalletContext(req, user.ID.String())
	w := httptest.NewRecorder()
	HandlePurchaseSubscription(a).ServeHTTP(w, req)
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want 402; body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "余额不足") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestSubscription_AdminCRUDAndUserList(t *testing.T) {
	a, user := subscriptionApp(t)
	admin := seedUserForWalletTest(t, a, "sub-admin3@example.com", "pass12345", "Sub Admin 3")
	planID := createPlanRequest(t, a, admin.ID, "Basic", "5", 7)

	// Admin list contains the plan.
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/subscription-plans", nil)
	listReq = setAdminCtx(listReq, admin.ID.String())
	listW := httptest.NewRecorder()
	HandleListSubscriptionPlans(a).ServeHTTP(listW, listReq)
	if !strings.Contains(listW.Body.String(), "Basic") {
		t.Fatalf("admin list missing plan: %s", listW.Body.String())
	}

	// User list shows enabled plans.
	userReq := httptest.NewRequest(http.MethodGet, "/api/console/subscription/plans", nil)
	userReq = setUserInWalletContext(userReq, user.ID.String())
	userW := httptest.NewRecorder()
	HandleListPlans(a).ServeHTTP(userW, userReq)
	if !strings.Contains(userW.Body.String(), "Basic") {
		t.Fatalf("user list missing plan: %s", userW.Body.String())
	}

	// Disable via update → user list hides it.
	upd := httptest.NewRequest(http.MethodPut, "/api/admin/subscription-plans/"+planID,
		strings.NewReader(`{"enabled":false}`))
	upd = chiRouteMultiCtx(upd, map[string]string{"id": planID})
	upd = setAdminCtx(upd, admin.ID.String())
	updW := httptest.NewRecorder()
	HandleUpdateSubscriptionPlan(a).ServeHTTP(updW, upd)
	if updW.Code != http.StatusOK {
		t.Fatalf("update status = %d: %s", updW.Code, updW.Body.String())
	}
	userW2 := httptest.NewRecorder()
	HandleListPlans(a).ServeHTTP(userW2, userReq)
	if strings.Contains(userW2.Body.String(), "Basic") {
		t.Fatalf("disabled plan must not be listed: %s", userW2.Body.String())
	}
}

func TestSubscription_AdminPlanFlexIntFields(t *testing.T) {
	a, _ := subscriptionApp(t)
	admin := seedUserForWalletTest(t, a, "sub-admin-flex@example.com", "pass12345", "Sub Admin Flex")

	// Create with string numeric fields, exactly like the admin form sends them.
	req := httptest.NewRequest(http.MethodPost, "/api/admin/subscription-plans",
		strings.NewReader(`{"name":"Flex","description":"d","price":"10","duration_days":"30","group_name":"g","token_quota":"100","sort_order":"2"}`))
	req = setAdminCtx(req, admin.ID.String())
	w := httptest.NewRecorder()
	HandleCreateSubscriptionPlan(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create with string fields status = %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Fatal("create returned empty id")
	}

	var durationDays int
	var tokenQuota int64
	var sortOrder int
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT duration_days, token_quota, sort_order FROM subscription_plans WHERE id = $1`, resp.ID).
		Scan(&durationDays, &tokenQuota, &sortOrder)
	if durationDays != 30 || tokenQuota != 100 || sortOrder != 2 {
		t.Fatalf("stored = %d/%d/%d, want 30/100/2", durationDays, tokenQuota, sortOrder)
	}

	// Toggle enabled off with the full string form, like the row switch does.
	upd := httptest.NewRequest(http.MethodPut, "/api/admin/subscription-plans/"+resp.ID,
		strings.NewReader(`{"name":"Flex","description":"d","price":"10","duration_days":"30","group_name":"g","token_quota":"100","sort_order":"2","enabled":false}`))
	upd = chiRouteMultiCtx(upd, map[string]string{"id": resp.ID})
	upd = setAdminCtx(upd, admin.ID.String())
	updW := httptest.NewRecorder()
	HandleUpdateSubscriptionPlan(a).ServeHTTP(updW, upd)
	if updW.Code != http.StatusOK {
		t.Fatalf("update with string fields status = %d: %s", updW.Code, updW.Body.String())
	}
	var enabled bool
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT enabled FROM subscription_plans WHERE id = $1`, resp.ID).Scan(&enabled)
	if enabled {
		t.Fatal("plan still enabled after toggle")
	}
}

func TestSubscription_AdminListAndCancel(t *testing.T) {
	a, user := subscriptionApp(t)
	admin := seedUserForWalletTest(t, a, "sub-admin4@example.com", "pass12345", "Sub Admin 4")
	planID := createPlanRequest(t, a, admin.ID, "Ops", "10", 30)

	// Create an active subscription via the purchase flow.
	purchase := httptest.NewRequest(http.MethodPost, "/api/console/subscription/purchase",
		strings.NewReader(`{"plan_id":"`+planID+`"}`))
	purchase = setUserInWalletContext(purchase, user.ID.String())
	purchaseW := httptest.NewRecorder()
	HandlePurchaseSubscription(a).ServeHTTP(purchaseW, purchase)
	if purchaseW.Code != http.StatusOK {
		t.Fatalf("purchase status = %d: %s", purchaseW.Code, purchaseW.Body.String())
	}

	// Admin list shows the user email.
	listReq := httptest.NewRequest(http.MethodGet, "/api/admin/subscriptions", nil)
	listReq = setAdminCtx(listReq, admin.ID.String())
	listW := httptest.NewRecorder()
	HandleListAllSubscriptions(a).ServeHTTP(listW, listReq)
	if !strings.Contains(listW.Body.String(), "sub@example.com") {
		t.Fatalf("admin list missing user email: %s", listW.Body.String())
	}

	var subID string
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT id::text FROM user_subscriptions WHERE user_id = $1 LIMIT 1`, user.ID).Scan(&subID)
	if subID == "" {
		t.Fatal("no subscription row found")
	}

	// Cancel → status flips; cancelling again returns 404.
	cancel := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/"+subID+"/cancel", nil)
	cancel = chiRouteMultiCtx(cancel, map[string]string{"id": subID})
	cancel = setAdminCtx(cancel, admin.ID.String())
	cancelW := httptest.NewRecorder()
	HandleCancelSubscription(a).ServeHTTP(cancelW, cancel)
	if cancelW.Code != http.StatusOK {
		t.Fatalf("cancel status = %d: %s", cancelW.Code, cancelW.Body.String())
	}
	var status string
	_ = a.Pool.QueryRow(context.Background(),
		`SELECT status FROM user_subscriptions WHERE id = $1`, subID).Scan(&status)
	if status != "cancelled" {
		t.Fatalf("status = %q, want cancelled", status)
	}
	cancel2 := httptest.NewRequest(http.MethodPost, "/api/admin/subscriptions/"+subID+"/cancel", nil)
	cancel2 = chiRouteMultiCtx(cancel2, map[string]string{"id": subID})
	cancel2 = setAdminCtx(cancel2, admin.ID.String())
	cancel2W := httptest.NewRecorder()
	HandleCancelSubscription(a).ServeHTTP(cancel2W, cancel2)
	if cancel2W.Code != http.StatusNotFound {
		t.Fatalf("second cancel status = %d, want 404", cancel2W.Code)
	}
}
