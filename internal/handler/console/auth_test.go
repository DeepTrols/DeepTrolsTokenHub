package console

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/repository/membership"
	"github.com/deeptrols/api/internal/repository/tenant"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForConsoleTest creates a minimal App with a real DB pool for testing.
func appForConsoleTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-handler-32-byte",
			ExpiryHours: 24,
		},
		Bootstrap: config.BootstrapConfig{
			AdminEmail:    "bootstrap@admin.test",
			AdminPassword: "bootstrap-pass-123",
		},
		// Console handler integration tests exercise the demo money faucet
		// (signup bonus) on the "enabled" branch; the disabled branch is
		// covered by TestRegister_NoBonusWhenFakePaymentDisabled.
		FakePayment: true,
	}

	return &app.App{
		Pool:        pool,
		Config:      cfg,
		Users:       user.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Tenants:     tenant.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// seedUserForConsoleTest creates a user with a bcrypt password hash and returns the domain.User.
func seedUserForConsoleTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
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
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForConsoleTest: create: %v", err)
	}
	return u
}

func TestHandleLogin_SuccessWithDBUser(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, "testuser@example.com", "secure-password", "Test User")

	body := map[string]string{
		"email":    "testuser@example.com",
		"password": "secure-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("response token is empty")
	}
	if resp.UserID != seedUser.ID.String() {
		t.Errorf("UserID = %s, want %s", resp.UserID, seedUser.ID.String())
	}
	if resp.Email != seedUser.Email {
		t.Errorf("Email = %s, want %s", resp.Email, seedUser.Email)
	}
	if resp.Name != seedUser.DisplayName {
		t.Errorf("Name = %s, want %s", resp.Name, seedUser.DisplayName)
	}

	// Verify the token is valid
	claims, err := jwtutil.ParseToken(resp.Token, a.Config.JWT.Secret)
	if err != nil {
		t.Fatalf("returned token is invalid: %v", err)
	}
	if claims.Subject != seedUser.ID.String() {
		t.Errorf("token Subject = %s, want %s", claims.Subject, seedUser.ID.String())
	}
	if claims.Email != seedUser.Email {
		t.Errorf("token Email = %s, want %s", claims.Email, seedUser.Email)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUserForConsoleTest(t, a, "wrongpw@example.com", "correct-password", "Wrong PW User")

	body := map[string]string{
		"email":    "wrongpw@example.com",
		"password": "wrong-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLogin_UserNotFoundFallsBackToBootstrap(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	// Do NOT create the user in the database

	body := map[string]string{
		"email":    a.Config.Bootstrap.AdminEmail,
		"password": a.Config.Bootstrap.AdminPassword,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert: bootstrap admin should succeed
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("response token is empty")
	}
	if resp.Email != a.Config.Bootstrap.AdminEmail {
		t.Errorf("Email = %s, want %s", resp.Email, a.Config.Bootstrap.AdminEmail)
	}
}

func TestHandleLogin_WrongBootstrapPassword(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	body := map[string]string{
		"email":    a.Config.Bootstrap.AdminEmail,
		"password": "wrong-bootstrap-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLogin_InvalidRequestBody(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleLogin_EmptyBody(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert: empty email/password should result in 401
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleLogin_LoginHistoryRecorded(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, "history@example.com", "test-password", "History User")

	body := map[string]string{
		"email":    "history@example.com",
		"password": "test-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.RemoteAddr = "192.168.1.1:12345"
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify login_history was recorded for the seeded user
	var count int
	err := a.Pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM login_history WHERE user_id = $1 AND success = true",
		seedUser.ID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("login_history query: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 login_history entry, got %d", count)
	}
}

func TestHandleLogin_FailedLoginHistoryRecorded(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	_ = seedUserForConsoleTest(t, a, "faillog@example.com", "right-password", "FailLog User")

	body := map[string]string{
		"email":    "faillog@example.com",
		"password": "wrong-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.1:9999"
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}

	// Verify failed login_history was recorded
	var count int
	err := a.Pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM login_history WHERE success = false AND ip_address LIKE '10.0%'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("login_history query: %v", err)
	}
	if count < 1 {
		t.Errorf("expected at least 1 failed login_history entry, got %d", count)
	}
}

func TestHandleLogin_DBUserPreferredOverBootstrap(t *testing.T) {
	// Arrange: create a DB user with same email as bootstrap admin
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, a.Config.Bootstrap.AdminEmail, "db-specific-password", "DB Admin")

	body := map[string]string{
		"email":    a.Config.Bootstrap.AdminEmail,
		"password": "db-specific-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should return the DB user's info, not bootstrap
	if resp.UserID != seedUser.ID.String() {
		t.Errorf("UserID = %s, want %s (DB user should take priority)", resp.UserID, seedUser.ID.String())
	}
}

func TestHandleLogin_UserAgentAndIPRecorded(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUserForConsoleTest(t, a, "agent@example.com", "test-password", "Agent User")

	body := map[string]string{
		"email":    "agent@example.com",
		"password": "test-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MyCustomAgent/2.0")
	req.RemoteAddr = "172.16.0.1:54321"
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var userAgent, ipAddress string
	err := a.Pool.QueryRow(context.Background(),
		"SELECT user_agent, ip_address FROM login_history WHERE success = true ORDER BY created_at DESC LIMIT 1",
	).Scan(&userAgent, &ipAddress)
	if err != nil {
		t.Fatalf("query login_history: %v", err)
	}
	if userAgent != "MyCustomAgent/2.0" {
		t.Errorf("user_agent = %s, want MyCustomAgent/2.0", userAgent)
	}
	if ipAddress != "172.16.0.1" {
		t.Errorf("ip_address = %s, want 172.16.0.1", ipAddress)
	}
}

func TestHandleLogin_BannedUser(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	// Create user directly with banned status
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	now := time.Now().UTC()
	bannedUser := &domain.User{
		ID:           uuid.New(),
		Email:        "banned@example.com",
		PasswordHash: string(hash),
		DisplayName:  "Banned User",
		Role:         "user",
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusBanned,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), bannedUser); err != nil {
		t.Fatalf("create banned user: %v", err)
	}

	body := map[string]string{
		"email":    "banned@example.com",
		"password": "password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert: banned users should not be able to login
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d (banned user should be denied)", w.Code, http.StatusUnauthorized)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

// --- HandleMe tests ---

// setUserInContext creates a request with the given user ID stored in context.
func setUserInContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

func TestHandleMe_Success(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, "me@example.com", "test-password", "Me User")

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp meResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != seedUser.ID.String() {
		t.Errorf("ID = %s, want %s", resp.ID, seedUser.ID.String())
	}
	if resp.Email != seedUser.Email {
		t.Errorf("Email = %s, want %s", resp.Email, seedUser.Email)
	}
	if resp.Name != seedUser.DisplayName {
		t.Errorf("Name = %s, want %s", resp.Name, seedUser.DisplayName)
	}
	if resp.Status != string(domain.UserStatusActive) {
		t.Errorf("Status = %s, want %s", resp.Status, string(domain.UserStatusActive))
	}
}

func TestHandleMe_NoUserIDInContext(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	// No context value set
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleMe_InvalidUUIDInContext(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, "not-a-valid-uuid")
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleMe_UserNotFound(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	// Use a UUID that does not exist in the database
	randomID := uuid.New().String()

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, randomID)
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleMe_ReturnsCorrectUser(t *testing.T) {
	// Arrange: create two users, ensure HandleMe returns the right one
	a := appForConsoleTest(t)
	userA := seedUserForConsoleTest(t, a, "userA@example.com", "passA", "User A")
	_ = seedUserForConsoleTest(t, a, "userB@example.com", "passB", "User B")

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, userA.ID.String())
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp meResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != userA.ID.String() {
		t.Errorf("ID = %s, want %s (should return user A, not B)", resp.ID, userA.ID.String())
	}
	if resp.Email != "userA@example.com" {
		t.Errorf("Email = %s, want userA@example.com", resp.Email)
	}
}

func TestHandleMe_ResponseContentTypeJSON(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, "ctype@example.com", "test-password", "CType User")

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	// Act
	handler := HandleMe(a)
	handler.ServeHTTP(w, req)

	// Assert
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

// --- HandleRegister tests ---

func TestRegister_Success(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	body := map[string]string{
		"email":    "newuser@example.com",
		"password": "secure-password-123",
		"name":     "New User",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp registerResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("response token is empty")
	}
	if resp.User.ID == "" {
		t.Fatal("response user ID is empty")
	}
	if resp.User.Email != "newuser@example.com" {
		t.Errorf("User.Email = %s, want newuser@example.com", resp.User.Email)
	}
	if resp.User.Name != "New User" {
		t.Errorf("User.Name = %s, want New User", resp.User.Name)
	}

	// Verify the user was created in the database
	dbUser, err := a.Users.FindByEmail(context.Background(), "newuser@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	if dbUser.DisplayName != "New User" {
		t.Errorf("DisplayName = %s, want New User", dbUser.DisplayName)
	}
	if dbUser.Status != domain.UserStatusActive {
		t.Errorf("Status = %s, want active", dbUser.Status)
	}

	// Verify the password was bcrypt hashed (not plaintext)
	if dbUser.PasswordHash == "secure-password-123" {
		t.Fatal("password was stored as plaintext, must be bcrypt hashed")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dbUser.PasswordHash), []byte("secure-password-123")); err != nil {
		t.Fatalf("bcrypt verify failed: %v", err)
	}

	// Verify the token is valid
	claims, err := jwtutil.ParseToken(resp.Token, a.Config.JWT.Secret)
	if err != nil {
		t.Fatalf("returned token is invalid: %v", err)
	}
	if claims.Subject != dbUser.ID.String() {
		t.Errorf("token Subject = %s, want %s", claims.Subject, dbUser.ID.String())
	}
	if claims.Email != "newuser@example.com" {
		t.Errorf("token Email = %s, want %s", claims.Email, "newuser@example.com")
	}
}

func TestRegister_AutoCreatesWallet(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	body := map[string]string{
		"email":    "walletuser@example.com",
		"password": "secure-password-123",
		"name":     "Wallet User",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify wallet was created for the new user
	dbUser, err := a.Users.FindByEmail(context.Background(), "walletuser@example.com")
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}

	var walletCount int
	err = a.Pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM wallets WHERE user_id = $1",
		dbUser.ID,
	).Scan(&walletCount)
	if err != nil {
		t.Fatalf("wallet count query: %v", err)
	}
	if walletCount != 1 {
		t.Errorf("expected 1 wallet for new user, got %d", walletCount)
	}

	// Verify wallet fields
	var balance, frozen, currency string
	var version int64
	err = a.Pool.QueryRow(context.Background(),
		"SELECT balance::text, frozen::text, currency, version FROM wallets WHERE user_id = $1",
		dbUser.ID,
	).Scan(&balance, &frozen, &currency, &version)
	if err != nil {
		t.Fatalf("wallet query: %v", err)
	}
	if balance != "1000.000000" {
		t.Errorf("wallet balance = %s, want 1000.000000", balance)
	}
	if frozen != "0.000000" {
		t.Errorf("wallet frozen = %s, want 0.000000", frozen)
	}
	if currency != "CNY" {
		t.Errorf("wallet currency = %s, want CNY", currency)
	}
	if version != 0 {
		t.Errorf("wallet version = %d, want 0", version)
	}
}

// TestRegister_NoBonusWhenFakePaymentDisabled verifies the production default:
// with ENABLE_FAKE_PAYMENT=false the signup wallet is created with 0 balance.
func TestRegister_NoBonusWhenFakePaymentDisabled(t *testing.T) {
	a := appForConsoleTest(t)
	a.Config.FakePayment = false

	email := "nobonus@example.com"
	body := map[string]string{
		"email":    email,
		"password": "secure-password-123",
		"name":     "No Bonus",
	}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	HandleRegister(a).ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	dbUser, err := a.Users.FindByEmail(context.Background(), email)
	if err != nil {
		t.Fatalf("FindByEmail: %v", err)
	}
	var balance string
	if err := a.Pool.QueryRow(context.Background(),
		"SELECT balance::text FROM wallets WHERE user_id = $1", dbUser.ID).Scan(&balance); err != nil {
		t.Fatalf("wallet query: %v", err)
	}
	if balance != "0.000000" {
		t.Errorf("wallet balance = %s, want 0.000000 (production must not grant signup bonus)", balance)
	}
}

func TestRegister_DuplicateEmail_409(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUserForConsoleTest(t, a, "existing@example.com", "old-password", "Existing User")

	body := map[string]string{
		"email":    "existing@example.com",
		"password": "new-password-123",
		"name":     "Duplicate User",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["error"] == "" {
		t.Error("expected error message in response")
	}
}

func TestRegister_InvalidEmail_400(t *testing.T) {
	tests := []struct {
		name  string
		email string
	}{
		{"empty email", ""},
		{"missing @", "notanemail"},
		{"missing domain", "user@"},
		{"missing username", "@example.com"},
		{"with spaces", "user @example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			a := appForConsoleTest(t)

			body := map[string]string{
				"email":    tt.email,
				"password": "secure-password-123",
				"name":     "Test User",
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act
			handler := HandleRegister(a)
			handler.ServeHTTP(w, req)

			// Assert
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for email %q", w.Code, http.StatusBadRequest, tt.email)
			}
		})
	}
}

func TestRegister_ShortPassword_400(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{"empty password", ""},
		{"1 char", "a"},
		{"7 chars", "1234567"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			a := appForConsoleTest(t)

			body := map[string]string{
				"email":    "pwtest@example.com",
				"password": tt.password,
				"name":     "Test User",
			}
			bodyBytes, _ := json.Marshal(body)

			req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act
			handler := HandleRegister(a)
			handler.ServeHTTP(w, req)

			// Assert
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for password %q", w.Code, http.StatusBadRequest, tt.password)
			}
		})
	}
}

func TestRegister_MissingFields_400(t *testing.T) {
	tests := []struct {
		name string
		body map[string]string
	}{
		{
			name: "missing name",
			body: map[string]string{
				"email":    "test@example.com",
				"password": "secure-password-123",
			},
		},
		{
			name: "empty name",
			body: map[string]string{
				"email":    "test@example.com",
				"password": "secure-password-123",
				"name":     "",
			},
		},
		{
			name: "whitespace-only name",
			body: map[string]string{
				"email":    "test@example.com",
				"password": "secure-password-123",
				"name":     "   ",
			},
		},
		{
			name: "missing email",
			body: map[string]string{
				"password": "secure-password-123",
				"name":     "Test User",
			},
		},
		{
			name: "missing password",
			body: map[string]string{
				"email": "test@example.com",
				"name":  "Test User",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Arrange
			a := appForConsoleTest(t)

			bodyBytes, _ := json.Marshal(tt.body)

			req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Act
			handler := HandleRegister(a)
			handler.ServeHTTP(w, req)

			// Assert
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d for case %q, body: %s", w.Code, http.StatusBadRequest, tt.name, w.Body.String())
			}
		})
	}
}

func TestRegister_InvalidRequestBody(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// appForConsoleCookieTest creates an App with Cookie config for cookie-based tests.
func appForConsoleCookieTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-handler-32-byte",
			ExpiryHours: 24,
		},
		Bootstrap: config.BootstrapConfig{
			AdminEmail:    "bootstrap@admin.test",
			AdminPassword: "bootstrap-pass-123",
		},
		Cookie: config.CookieConfig{
			Name:          "auth_token",
			Secure:        true,
			MaxAgeSeconds: 86400,
			SameSite:      "Strict",
		},
	}

	return &app.App{
		Pool:        pool,
		Config:      cfg,
		Users:       user.NewPostgresRepository(pool),
		Memberships: membership.NewPostgresRepository(pool),
		Healthy:     true,
	}
}

// TestHandleLogin_SetsAuthCookie verifies that HandleLogin sets an
// httpOnly cookie before writing the JSON body.
func TestHandleLogin_SetsAuthCookie(t *testing.T) {
	// Arrange
	a := appForConsoleCookieTest(t)
	seeded := seedUserForConsoleTest(t, a, "cookie-login@test.com", "test-password", "Cookie User")

	body := map[string]string{
		"email":    "cookie-login@test.com",
		"password": "test-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify Set-Cookie header exists
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header, got none")
	}

	var authCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatal("no auth_token cookie found in response")
	}

	if !authCookie.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if !authCookie.Secure {
		t.Error("cookie must be Secure")
	}
	if authCookie.Path != "/" {
		t.Errorf("cookie Path = %q, want %q", authCookie.Path, "/")
	}
	if authCookie.MaxAge != 86400 {
		t.Errorf("cookie MaxAge = %d, want 86400", authCookie.MaxAge)
	}
	if authCookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite = %v, want Strict", authCookie.SameSite)
	}
	if authCookie.Value == "" {
		t.Error("cookie value must not be empty")
	}

	// Verify the cookie value is the same as the token
	var resp loginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if authCookie.Value != resp.Token {
		t.Errorf("cookie value != token in response body: cookie = %s, token = %s", authCookie.Value, resp.Token)
	}

	// Verify cookie is a valid JWT
	claims, err := jwtutil.ParseToken(authCookie.Value, a.Config.JWT.Secret)
	if err != nil {
		t.Fatalf("cookie JWT is invalid: %v", err)
	}
	if claims.Subject != seeded.ID.String() {
		t.Errorf("cookie JWT subject = %s, want %s", claims.Subject, seeded.ID.String())
	}
}

// TestHandleRegister_SetsAuthCookie verifies that HandleRegister sets
// an httpOnly cookie with the JWT token.
func TestHandleRegister_SetsAuthCookie(t *testing.T) {
	// Arrange
	a := appForConsoleCookieTest(t)

	body := map[string]string{
		"email":    "cookie-reg@test.com",
		"password": "secure-pass-123",
		"name":     "Cookie Reg User",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatal("no auth_token cookie found in response")
	}

	if !authCookie.HttpOnly {
		t.Error("cookie must be HttpOnly")
	}
	if !authCookie.Secure {
		t.Error("cookie must be Secure")
	}
	if authCookie.Value == "" {
		t.Error("cookie value must not be empty")
	}
}

// TestHandleLogout_ClearsAuthCookie verifies that HandleLogout
// clears the auth cookie by setting MaxAge=0.
func TestHandleLogout_ClearsAuthCookie(t *testing.T) {
	// Arrange
	a := appForConsoleCookieTest(t)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/logout", nil)
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogout(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["message"] != "Logged out" {
		t.Errorf("message = %q, want \"Logged out\"", resp["message"])
	}

	cookies := w.Result().Cookies()
	var authCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	if authCookie == nil {
		t.Fatal("no auth_token cookie found in logout response")
	}

	if authCookie.MaxAge != -1 {
		t.Errorf("cookie MaxAge = %d, want -1 (cleared)", authCookie.MaxAge)
	}
	if authCookie.Value != "" {
		t.Errorf("cookie value = %q, want empty", authCookie.Value)
	}
}

// TestHandleLogin_CookieBeforeBody ensures cookie is set before
// writing the response body.
func TestHandleLogin_CookieBeforeBody(t *testing.T) {
	// Arrange
	a := appForConsoleCookieTest(t)
	seedUserForConsoleTest(t, a, "order@test.com", "test-password", "Order User")

	body := map[string]string{
		"email":    "order@test.com",
		"password": "test-password",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert: headers (including Set-Cookie) are written before body
	// httptest.ResponseRecorder captures headers when WriteHeader is called
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// If cookie was set after body write, Go http would log a warning
	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie in response headers")
	}
}

// TestHandleLogin_BootstrapSetsCookie verifies bootstrap admin login
// also sets the auth cookie.
func TestHandleLogin_BootstrapSetsCookie(t *testing.T) {
	// Arrange
	a := appForConsoleCookieTest(t)

	body := map[string]string{
		"email":    a.Config.Bootstrap.AdminEmail,
		"password": a.Config.Bootstrap.AdminPassword,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleLogin(a)
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	cookies := w.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("bootstrap login should set auth cookie")
	}
	var hasAuthCookie bool
	for _, c := range cookies {
		if c.Name == "auth_token" && c.Value != "" {
			hasAuthCookie = true
			break
		}
	}
	if !hasAuthCookie {
		t.Error("bootstrap login should set auth_token cookie with non-empty value")
	}
}

func TestRegister_ResponseContentTypeJSON(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)

	body := map[string]string{
		"email":    "ctype-reg@example.com",
		"password": "secure-password-123",
		"name":     "CType Reg User",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/auth/register", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Act
	handler := HandleRegister(a)
	handler.ServeHTTP(w, req)

	// Assert
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", ct)
	}
}

// TestHandleMe_PersonalHasNoTenantStatus verifies a personal account has no
// tenant-scoped fields (tenant_status is omitted).
func TestHandleMe_PersonalHasNoTenantStatus(t *testing.T) {
	// Arrange
	a := appForConsoleTest(t)
	seedUser := seedUserForConsoleTest(t, a, "personal-me@example.com", "test-password", "Personal")

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req = setUserInContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	// Act
	HandleMe(a).ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp meResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.TenantStatus != "" {
		t.Errorf("tenant_status = %q, want empty for a personal user", resp.TenantStatus)
	}
	if resp.UserType != string(domain.UserTypePersonal) {
		t.Errorf("user_type = %s, want personal", resp.UserType)
	}
}
