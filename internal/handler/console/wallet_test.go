package console

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/deeptrols/api/internal/repository/wallet"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

// appForWalletTest creates a minimal App with repos wired for wallet tests.
func appForWalletTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-wallet-32-byte",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		Wallets: wallet.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForWalletTest creates a user with bcrypt hash.
func seedUserForWalletTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	now := time.Now().UTC()
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Role:         "user",
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForWalletTest: create: %v", err)
	}
	return u
}

// setUserInWalletContext adds a user ID to the request context.
func setUserInWalletContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

// =============================================================================
// HandleGetWallet Tests
// =============================================================================

func TestHandleGetWallet_NoAuth(t *testing.T) {
	a := appForWalletTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet", nil)
	w := httptest.NewRecorder()

	handler := HandleGetWallet(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetWallet_NoWallet_ReturnsZeroBalance(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "no-wallet@example.com", "pass", "No Wallet")

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetWallet(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Balance      string `json:"balance"`
		Frozen       string `json:"frozen"`
		Available    string `json:"available"`
		Currency     string `json:"currency"`
		TotalCharged string `json:"total_charged,omitempty"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Balance != "0" {
		t.Errorf("balance = %s, want '0'", resp.Balance)
	}
	if resp.Frozen != "0" {
		t.Errorf("frozen = %s, want '0'", resp.Frozen)
	}
	if resp.Available != "0" {
		t.Errorf("available = %s, want '0'", resp.Available)
	}
	if resp.Currency == "" {
		t.Error("currency should not be empty")
	}
}

func TestHandleGetWallet_WithBalance(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "has-wallet@example.com", "pass", "Has Wallet")

	// Direct SQL insert wallet
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, 'CNY', 0, NOW(), NOW())`,
		uuid.New(), seedUser.ID, "250.75", "50.25",
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetWallet(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Balance   string `json:"balance"`
		Frozen    string `json:"frozen"`
		Available string `json:"available"`
		Currency  string `json:"currency"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Balance != "250.75" {
		t.Errorf("balance = %s, want '250.75'", resp.Balance)
	}
	if resp.Frozen != "50.25" {
		t.Errorf("frozen = %s, want '50.25'", resp.Frozen)
	}

	// available = balance - frozen = 250.75 - 50.25 = 200.50 (String() = "200.5")
	if resp.Available != "200.5" {
		t.Errorf("available = %s, want '200.5'", resp.Available)
	}
	if resp.Currency != "CNY" {
		t.Errorf("currency = %s, want 'CNY'", resp.Currency)
	}
}

func TestHandleGetWallet_ResponseContentTypeJSON(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "ctype-wallet@example.com", "pass", "CType Wallet")

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetWallet(a)
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

// =============================================================================
// HandleListTransactions Tests
// =============================================================================

func TestHandleListTransactions_NoAuth(t *testing.T) {
	a := appForWalletTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions", nil)
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListTransactions_NoWallet_ReturnsEmptyArray(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "tx-no-wallet@example.com", "pass", "No Wallet Tx")

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp.Data))
	}
	if resp.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Total)
	}
}

func TestHandleListTransactions_EmptyTransactions(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "tx-empty@example.com", "pass", "Empty Tx")

	// Create wallet with no transactions
	walletID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		walletID, seedUser.ID,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty array, got %d items", len(resp.Data))
	}
}

func TestHandleListTransactions_WithTransactions(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "tx-with@example.com", "pass", "With Tx")

	// Create wallet
	walletID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '99.9965', '0', 'CNY', 0, NOW(), NOW())`,
		walletID, seedUser.ID,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}

	// Create transactions
	tx1 := uuid.New()
	tx2 := uuid.New()
	now := time.Now().UTC()
	_, err = a.Pool.Exec(context.Background(),
		`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, created_at)
		 VALUES ($1, $2, 'ik-1', 'charge', '-0.0035', '100.0000', '99.9965', 'chat_completion', $3)`,
		tx1, walletID, now,
	)
	if err != nil {
		t.Fatalf("insert tx1: %v", err)
	}
	_, err = a.Pool.Exec(context.Background(),
		`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, created_at)
		 VALUES ($1, $2, 'ik-2', 'topup', '50.0000', '49.9965', '99.9965', 'manual_topup', $3)`,
		tx2, walletID, now.Add(-time.Hour),
	)
	if err != nil {
		t.Fatalf("insert tx2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data []struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			Amount       string `json:"amount"`
			BalanceAfter string `json:"balance_after"`
			Reference    string `json:"reference"`
			CreatedAt    string `json:"created_at"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Verify transaction data is correct (most recent first)
	if resp.Data[0].ID != tx1.String() {
		t.Logf("first tx ID = %s, expected %s (most recent first)", resp.Data[0].ID, tx1.String())
	}
	if resp.Data[0].Type != "charge" {
		t.Errorf("first tx type = %s, want 'charge'", resp.Data[0].Type)
	}
	if resp.Data[0].Amount != "-0.0035" {
		t.Errorf("first tx amount = %s, want '-0.0035'", resp.Data[0].Amount)
	}
}

func TestHandleListTransactions_Pagination(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "tx-page@example.com", "pass", "Page Tx")

	// Create wallet
	walletID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		walletID, seedUser.ID,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		secs := fmt.Sprintf("%d", i)
		_, err = a.Pool.Exec(context.Background(),
			`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, created_at)
			 VALUES ($1, $2, $3, 'charge', '-0.01', '100', '99.99', 'test', NOW() - ($4 || ' seconds')::INTERVAL)`,
			uuid.New(), walletID, "ik-page-"+fmt.Sprintf("%d", i), secs,
		)
		if err != nil {
			t.Fatalf("insert tx %d: %v", i, err)
		}
	}

	// Request with limit=2
	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions?limit=2&offset=0", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total int           `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 transactions (limit=2), got %d", len(resp.Data))
	}
}

func TestHandleListTransactions_DefaultLimit(t *testing.T) {
	a := appForWalletTest(t)
	seedUser := seedUserForWalletTest(t, a, "tx-default@example.com", "pass", "Default Limit")

	// Create wallet
	walletID := uuid.New()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO wallets (id, user_id, balance, frozen, currency, version, created_at, updated_at)
		 VALUES ($1, $2, '100', '0', 'CNY', 0, NOW(), NOW())`,
		walletID, seedUser.ID,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}

	// Create 25 transactions (1 more than default limit of 20)
	for i := 0; i < 25; i++ {
		secs := fmt.Sprintf("%d", i)
		_, err = a.Pool.Exec(context.Background(),
			`INSERT INTO wallet_transactions (id, wallet_id, idempotency_key, tx_type, amount, balance_before, balance_after, reference_type, created_at)
			 VALUES ($1, $2, $3, 'charge', '-0.01', '100', '99.99', 'test', NOW() - ($4 || ' seconds')::INTERVAL)`,
			uuid.New(), walletID, "ik-default-"+fmt.Sprintf("%d", i), secs,
		)
		if err != nil {
			t.Fatalf("insert tx %d: %v", i, err)
		}
	}

	// Request without limit/offset
	req := httptest.NewRequest(http.MethodGet, "/api/console/wallet/transactions", nil)
	req = setUserInWalletContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListTransactions(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []interface{} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Default limit should cap at 20
	if len(resp.Data) > 20 {
		t.Errorf("expected at most 20 transactions (default limit), got %d", len(resp.Data))
	}
}

// =============================================================================
// Decimal helper
// =============================================================================

func decMustParse(s string) decimal.Decimal {
	return decimal.RequireFromString(s)
}
