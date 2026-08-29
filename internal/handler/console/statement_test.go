package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHandleMonthlyStatement_AggregatesMonth(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "stmt@example.com", "pass", "Stmt User")

	now := time.Now().In(statementZone)
	year, month := now.Year(), int(now.Month())
	start := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, statementZone)
	prevStart := start.AddDate(0, -1, 0)

	// Wallet + API key + usage + top-ups.
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), seedUser.ID); err != nil {
		t.Fatalf("wallet: %v", err)
	}
	var walletID uuid.UUID
	if err := a.Pool.QueryRow(context.Background(),
		`SELECT id FROM wallets WHERE user_id = $1`, seedUser.ID).Scan(&walletID); err != nil {
		t.Fatalf("wallet id: %v", err)
	}
	keyID := uuid.New()
	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO api_keys (id, user_id, key_prefix, key_hash, masked_key, name, created_at, updated_at)
		 VALUES ($1, $2, 'sk-', $3, 'sk-****stmt', 'stmt key', NOW(), NOW())`,
		keyID, seedUser.ID, "hash-stmt-"+uuid.New().String()[:8]); err != nil {
		t.Fatalf("api key: %v", err)
	}

	seedUsage := func(model, cost string, ts time.Time) {
		if _, err := a.Pool.Exec(context.Background(),
			`INSERT INTO usage_logs (id, user_id, api_key_id, request_id, request_type,
			                         public_model_code, usage_source, usage_normalized, usage_raw,
			                         list_cost, final_cost, status, created_at)
			 VALUES ($1, $2, $3, $4, 'chat', $5, 'upstream', '{"input_tokens": 10, "output_tokens": 5}'::jsonb,
			         '{"total_tokens": 15}'::jsonb, 0, $6, 'completed', $7)`,
			uuid.New(), seedUser.ID, keyID, "req-stmt-"+uuid.New().String()[:8], model, cost, ts); err != nil {
			t.Fatalf("usage %s: %v", model, err)
		}
	}
	seedTopup := func(amount string, ts time.Time) {
		if _, err := a.Pool.Exec(context.Background(),
			`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, created_at)
			 VALUES ($1, $2, $3, 'topup', $4, 0, $4, $5)`,
			uuid.New(), walletID, "stmttx-"+uuid.New().String()[:8], amount, ts); err != nil {
			t.Fatalf("topup: %v", err)
		}
	}

	// Current month: gpt-4o x2 @ 10, claude x1 @ 5, topup 100 + 50.
	seedUsage("gpt-4o", "10", start.Add(time.Hour))
	seedUsage("gpt-4o", "10", start.Add(2*time.Hour))
	seedUsage("claude-sonnet", "5", start.Add(3*time.Hour))
	seedTopup("100", start.Add(time.Hour))
	seedTopup("50", start.Add(2*time.Hour))
	// Previous month must be excluded.
	seedUsage("gpt-4o", "999", prevStart.Add(time.Hour))
	seedTopup("999", prevStart.Add(time.Hour))

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/console/billing/statement?year=%d&month=%d", year, month), nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()
	HandleMonthlyStatement(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var got monthlyStatement
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Year != year || got.Month != month {
		t.Errorf("period = %d-%d, want %d-%d", got.Year, got.Month, year, month)
	}
	if got.TotalCost != "25.00" {
		t.Errorf("total_cost = %q, want 25.00", got.TotalCost)
	}
	if got.TotalTopup != "150.00" {
		t.Errorf("total_topup = %q, want 150.00", got.TotalTopup)
	}
	if got.ChargeCount != 3 {
		t.Errorf("charge_count = %d, want 3", got.ChargeCount)
	}
	if len(got.ByModel) != 2 {
		t.Fatalf("by_model len = %d, want 2", len(got.ByModel))
	}
	if got.ByModel[0].Model != "gpt-4o" || got.ByModel[0].Cost != "20.00" || got.ByModel[0].Count != 2 {
		t.Errorf("by_model[0] = %+v, want gpt-4o 20.00 x2", got.ByModel[0])
	}
	if got.ByModel[1].Model != "claude-sonnet" || got.ByModel[1].Cost != "5.00" {
		t.Errorf("by_model[1] = %+v, want claude-sonnet 5.00", got.ByModel[1])
	}
}

func TestHandleMonthlyStatement_PreviousMonth(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "stmt-prev@example.com", "pass", "Stmt Prev")

	now := time.Now().In(statementZone)
	prevStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, statementZone).AddDate(0, -1, 0)
	prevYear, prevMonth := prevStart.Year(), int(prevStart.Month())

	if _, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		uuid.New(), seedUser.ID); err != nil {
		t.Fatalf("wallet: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/console/billing/statement?year=%d&month=%d", prevYear, prevMonth), nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()
	HandleMonthlyStatement(a).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var got monthlyStatement
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.TotalCost != "0.00" || got.TotalTopup != "0.00" || got.ChargeCount != 0 || len(got.ByModel) != 0 {
		t.Fatalf("empty month mismatch: %+v", got)
	}
}

func TestHandleMonthlyStatement_InvalidParamsAndAuth(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "stmt-bad@example.com", "pass", "Stmt Bad")

	// Invalid month.
	req := httptest.NewRequest(http.MethodGet, "/api/console/billing/statement?year=2026&month=13", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()
	HandleMonthlyStatement(a).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad month status = %d, want 400", w.Code)
	}
	// Invalid year.
	req = httptest.NewRequest(http.MethodGet, "/api/console/billing/statement?year=abc&month=1", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w = httptest.NewRecorder()
	HandleMonthlyStatement(a).ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad year status = %d, want 400", w.Code)
	}
	// No auth.
	req = httptest.NewRequest(http.MethodGet, "/api/console/billing/statement", nil)
	w = httptest.NewRecorder()
	HandleMonthlyStatement(a).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no auth status = %d, want 401", w.Code)
	}
}
