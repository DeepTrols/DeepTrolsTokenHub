package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/jwtutil"
	"github.com/deeptrols/api/internal/pkg/keyhash"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/google/uuid"
)

// helper to create a minimal config with the given JWT secret.
func configWithJWTSecret(secret string) *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			Secret:      secret,
			ExpiryHours: 24,
		},
		Encryption: config.EncryptionConfig{
			Key: "test-hmac-secret-32-bytes-!!!!",
		},
	}
}

// helper to generate a valid JWT token for testing.
func generateTestJWT(t *testing.T, userID uuid.UUID, email, name, secret string) string {
	t.Helper()
	token, err := jwtutil.GenerateToken(userID, email, name, "", "personal", "", "", secret, 1)
	if err != nil {
		t.Fatalf("generateTestJWT: %v", err)
	}
	return token
}

// mockUserRepo implements user.Repository with a configurable FindByID
// (the only method ConsoleAuth now consults for live account checks).
type mockUserRepo struct {
	findByIDFn func(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

func (m *mockUserRepo) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	return nil, userErrNotFound
}
func (m *mockUserRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, userErrNotFound
}
func (m *mockUserRepo) Create(ctx context.Context, user *domain.User) error { return nil }
func (m *mockUserRepo) List(ctx context.Context, filter user.ListFilter, limit, offset int) ([]domain.User, error) {
	return nil, nil
}
func (m *mockUserRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.UserStatus) error {
	return nil
}
func (m *mockUserRepo) UpdateRole(ctx context.Context, id uuid.UUID, role string) error { return nil }
func (m *mockUserRepo) UpdateProfile(ctx context.Context, id uuid.UUID, displayName, phone, avatarURL string) error {
	return nil
}
func (m *mockUserRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return nil
}
func (m *mockUserRepo) Count(ctx context.Context, filter user.ListFilter) (int, error) { return 0, nil }

// consoleTestApp builds an App for ConsoleAuth tests. A nil user makes
// FindByID return "not found" (the disabled/deleted-account path).
func consoleTestApp(cfg *config.Config, u *domain.User) *app.App {
	return &app.App{
		Config: cfg,
		Users: &mockUserRepo{
			findByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
				if u == nil {
					return nil, userErrNotFound
				}
				return u, nil
			},
		},
	}
}

var userErrNotFound = errors.New("user not found")

// handler that captures context values for assertions.
type contextCapturer struct {
	UserID   string
	Email    string
	UserName string
}

func (c *contextCapturer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if v := r.Context().Value(CtxUserID); v != nil {
		c.UserID, _ = v.(string)
	}
	if v := r.Context().Value(CtxEmail); v != nil {
		c.Email, _ = v.(string)
	}
	if v := r.Context().Value(CtxUserName); v != nil {
		c.UserName, _ = v.(string)
	}
	w.WriteHeader(http.StatusOK)
}

func TestConsoleAuth_ValidJWT(t *testing.T) {
	// Arrange
	secret := "test-jwt-secret-key-min-32-bytes!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	email := "admin@deeptrols.com"
	name := "Admin User"
	token := generateTestJWT(t, userID, email, name, secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturer.UserID != userID.String() {
		t.Errorf("CtxUserID = %s, want %s", capturer.UserID, userID.String())
	}
	if capturer.Email != email {
		t.Errorf("CtxEmail = %s, want %s", capturer.Email, email)
	}
	if capturer.UserName != name {
		t.Errorf("CtxUserName = %s, want %s", capturer.UserName, name)
	}
}

func TestConsoleAuth_MissingAuthorizationHeader(t *testing.T) {
	// Arrange
	cfg := configWithJWTSecret("test-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_MissingBearerPrefix(t *testing.T) {
	// Arrange
	cfg := configWithJWTSecret("test-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "some-token-without-bearer")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_EmptyBearerToken(t *testing.T) {
	// Arrange
	cfg := configWithJWTSecret("test-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_InvalidJWT(t *testing.T) {
	// Arrange
	secret := "test-jwt-secret-key-min-32-bytes!!"
	cfg := configWithJWTSecret(secret)
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_WrongSecret(t *testing.T) {
	// Arrange
	signingSecret := "correct-secret-key-at-least-32-bytes"
	validationSecret := "wrong-secret-key-at-least-32-byte!!!"
	cfg := configWithJWTSecret(validationSecret)
	token := generateTestJWT(t, uuid.New(), "admin@test.com", "Admin", signingSecret)

	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_ExpiredJWT(t *testing.T) {
	// Arrange
	secret := "expired-test-secret-key-32-bytes!"
	token, err := jwtutil.GenerateToken(uuid.New(), "exp@test.com", "Exp", "", "personal", "", "", secret, -1)
	if err != nil {
		t.Fatalf("generate expired token: %v", err)
	}

	cfg := configWithJWTSecret(secret)
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: expired token should return 401
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestConsoleAuth_ContextKeysAreUnsetOnFailure(t *testing.T) {
	// Arrange
	cfg := configWithJWTSecret("test-secret-key-at-least-32-bytes!!")
	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: the next handler should NOT have been called with context keys set
	if capturer.UserID != "" {
		t.Error("CtxUserID should not be set on failed auth")
	}
	if capturer.Email != "" {
		t.Error("CtxEmail should not be set on failed auth")
	}
}

func TestConsoleAuth_DifferentEmail(t *testing.T) {
	// Arrange
	secret := "another-test-secret-key-32-bytes!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	email := "another@test.com"
	name := "Another User"
	token := generateTestJWT(t, userID, email, name, secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturer.Email != email {
		t.Errorf("CtxEmail = %s, want %s", capturer.Email, email)
	}
	if capturer.UserName != name {
		t.Errorf("CtxUserName = %s, want %s", capturer.UserName, name)
	}
}

func TestConsoleAuth_ResponseContentTypeJSON(t *testing.T) {
	// Arrange: ensure error responses have JSON content type
	cfg := configWithJWTSecret("test-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("Content-Type header should be set on error responses")
	}
}

func TestConsoleAuth_ContextValuesAccessibleViaJWTUtil(t *testing.T) {
	// Arrange
	secret := "test-jwt-secret-key-min-32-bytes!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	email := "accessible@test.com"
	name := "Accessible User"
	token := generateTestJWT(t, userID, email, name, secret)

	var ctx context.Context
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: extract via jwtutil helpers
	extractedID, err := jwtutil.UserIDFromContext(ctx)
	if err != nil {
		t.Fatalf("UserIDFromContext: %v", err)
	}
	if extractedID != userID {
		t.Errorf("extracted ID = %s, want %s", extractedID, userID)
	}

	extractedEmail, err := jwtutil.EmailFromContext(ctx)
	if err != nil {
		t.Fatalf("EmailFromContext: %v", err)
	}
	if extractedEmail != email {
		t.Errorf("extracted email = %s, want %s", extractedEmail, email)
	}
}

// ---------------------------------------------------------------------------
// Cookie-based auth tests
// ---------------------------------------------------------------------------

// configWithCookie creates a test config with cookie settings.
func configWithCookie(name string, secure bool, maxAge int, sameSite string, jwtSecret string) *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			Secret:      jwtSecret,
			ExpiryHours: 24,
		},
		Cookie: config.CookieConfig{
			Name:          name,
			Secure:        secure,
			MaxAgeSeconds: maxAge,
			SameSite:      sameSite,
		},
		Encryption: config.EncryptionConfig{
			Key: "test-hmac-secret-32-bytes-!!!!",
		},
	}
}

// TestConsoleAuth_ReadsJWTFromCookie verifies that ConsoleAuth reads
// the JWT from a cookie when no Authorization header is present.
func TestConsoleAuth_ReadsJWTFromCookie(t *testing.T) {
	// Arrange
	secret := "test-jwt-cookie-secret-32-bytes!!"
	cfg := configWithCookie("auth_token", true, 86400, "Strict", secret)
	userID := uuid.New()
	email := "cookie@test.com"
	name := "Cookie User"
	token := generateTestJWT(t, userID, email, name, secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth_token",
		Value: token,
	})
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturer.UserID != userID.String() {
		t.Errorf("CtxUserID = %s, want %s", capturer.UserID, userID.String())
	}
	if capturer.Email != email {
		t.Errorf("CtxEmail = %s, want %s", capturer.Email, email)
	}
}

// TestConsoleAuth_FallsBackToHeaderWhenCookieMissing verifies that
// ConsoleAuth uses Authorization header when cookie is not present.
func TestConsoleAuth_FallsBackToHeaderWhenCookieMissing(t *testing.T) {
	email := "fallback@test.com"
	name := "Fallback"
	// Arrange
	secret := "test-jwt-fallback-secret-32-byte!"
	cfg := configWithCookie("auth_token", true, 86400, "Strict", secret)
	userID := uuid.New()
	token := generateTestJWT(t, userID, "fallback@test.com", "Fallback", secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (should fall back to header)", w.Code, http.StatusOK)
	}
	if capturer.UserID != userID.String() {
		t.Errorf("CtxUserID = %s, want %s", capturer.UserID, userID.String())
	}
}

// TestConsoleAuth_FallsBackToHeaderWhenCookieEmpty verifies that
// ConsoleAuth falls back to Authorization header when cookie value is empty.
func TestConsoleAuth_FallsBackToHeaderWhenCookieEmpty(t *testing.T) {
	email := "emptyck@test.com"
	name := "EmptyCk"
	// Arrange
	secret := "test-jwt-emptyck-secret-32-byte!"
	cfg := configWithCookie("auth_token", true, 86400, "Strict", secret)
	userID := uuid.New()
	token := generateTestJWT(t, userID, "emptyck@test.com", "EmptyCk", secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: email, DisplayName: name, Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth_token",
		Value: "",
	})
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (should fall back to header when cookie empty)", w.Code, http.StatusOK)
	}
	if capturer.UserID != userID.String() {
		t.Errorf("CtxUserID = %s, want %s", capturer.UserID, userID.String())
	}
}

// TestConsoleAuth_PrefersCookieOverHeader verifies that when both
// cookie and header are present, the cookie takes precedence.
func TestConsoleAuth_PrefersCookieOverHeader(t *testing.T) {
	// Arrange
	secret := "test-jwt-prefer-secret-32-byte!!"
	cfg := configWithCookie("auth_token", true, 86400, "Strict", secret)

	cookieUserID := uuid.New()
	cookieEmail := "cookie-priority@test.com"
	cookieToken := generateTestJWT(t, cookieUserID, cookieEmail, "CookiePri", secret)

	headerUserID := uuid.New()
	headerToken := generateTestJWT(t, headerUserID, "header@test.com", "Header", secret)

	capturer := &contextCapturer{}
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: cookieUserID, Email: cookieEmail, DisplayName: "CookiePri", Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(capturer)

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth_token",
		Value: cookieToken,
	})
	req.Header.Set("Authorization", "Bearer "+headerToken)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert: cookie should win
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if capturer.UserID != cookieUserID.String() {
		t.Errorf("CtxUserID = %s, want %s (cookie should take priority)", capturer.UserID, cookieUserID.String())
	}
	if capturer.Email != cookieEmail {
		t.Errorf("CtxEmail = %s, want %s (cookie should take priority)", capturer.Email, cookieEmail)
	}
}

// TestConsoleAuth_Returns401WhenNoCookieOrHeader verifies that
// ConsoleAuth returns 401 when neither cookie nor header is present.
func TestConsoleAuth_Returns401WhenNoCookieOrHeader(t *testing.T) {
	// Arrange
	cfg := configWithCookie("auth_token", true, 86400, "Strict", "test-jwt-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// TestConsoleAuth_InvalidJWTInCookie_401 verifies that
// ConsoleAuth returns 401 for an invalid JWT in the cookie.
func TestConsoleAuth_InvalidJWTInCookie_401(t *testing.T) {
	// Arrange
	cfg := configWithCookie("auth_token", true, 86400, "Strict", "test-jwt-secret-key-at-least-32-bytes!!")
	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.AddCookie(&http.Cookie{
		Name:  "auth_token",
		Value: "not-a-valid-jwt",
	})
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ---------------------------------------------------------------------------
// GatewayAuth tests
// ---------------------------------------------------------------------------

// mockAPIKeyRepo implements apikey.Repository for testing GatewayAuth.
type mockAPIKeyRepo struct {
	findByHashFn func(ctx context.Context, keyHash string) (*domain.APIKey, error)

	mu                   sync.Mutex
	updateLastUsedCalled int
	lastUpdatedKeyID     uuid.UUID
}

func (m *mockAPIKeyRepo) FindByHash(ctx context.Context, keyHash string) (*domain.APIKey, error) {
	if m.findByHashFn != nil {
		return m.findByHashFn(ctx, keyHash)
	}
	return nil, nil
}
func (m *mockAPIKeyRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyRepo) ListByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) ([]domain.APIKey, error) {
	return nil, nil
}
func (m *mockAPIKeyRepo) Create(ctx context.Context, key *domain.APIKey) error { return nil }
func (m *mockAPIKeyRepo) Update(ctx context.Context, key *domain.APIKey) error { return nil }
func (m *mockAPIKeyRepo) UpdateLastUsed(ctx context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updateLastUsedCalled++
	m.lastUpdatedKeyID = id
	return nil
}
func (m *mockAPIKeyRepo) GetSpend(ctx context.Context, keyID uuid.UUID, periodType string) (*domain.APIKeySpend, error) {
	return nil, nil
}
func (m *mockAPIKeyRepo) UpdateSpend(ctx context.Context, spend *domain.APIKeySpend) error {
	return nil
}

// appWithMockRepo creates a minimal *app.App with the given mock API key repository.
func appWithMockRepo(repo *mockAPIKeyRepo) *app.App {
	return &app.App{
		Config:  configWithJWTSecret("test-secret"),
		APIKeys: repo,
	}
}

// gatewayContextCapturer captures context values set by GatewayAuth.
type gatewayContextCapturer struct {
	APIKeyID  string
	UserID    string
	RequestID string
}

func (c *gatewayContextCapturer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if v := r.Context().Value(CtxAPIKeyID); v != nil {
		c.APIKeyID, _ = v.(string)
	}
	if v := r.Context().Value(CtxUserID); v != nil {
		c.UserID, _ = v.(string)
	}
	if v := r.Context().Value(CtxRequestID); v != nil {
		c.RequestID, _ = v.(string)
	}
	w.WriteHeader(http.StatusOK)
}

func TestGatewayAuth_ValidActiveKey_200_StoresContextValues(t *testing.T) {
	// Arrange
	plaintextKey := "dt-my-plaintext-api-key-for-testing"
	keyHash := keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!")
	expectedKeyID := uuid.New()
	expectedUserID := uuid.New()

	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyHash {
				return &domain.APIKey{
					ID:     expectedKeyID,
					UserID: expectedUserID,
					Status: domain.APIKeyStatusActive,
				}, nil
			}
			return nil, nil
		},
	}

	application := appWithMockRepo(repo)
	capturer := &gatewayContextCapturer{}
	mw := GatewayAuth(application)
	handler := mw(capturer)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	req.Header.Set("X-Request-ID", "req-12345")
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturer.APIKeyID != expectedKeyID.String() {
		t.Errorf("CtxAPIKeyID = %q, want %q", capturer.APIKeyID, expectedKeyID.String())
	}
	if capturer.UserID != expectedUserID.String() {
		t.Errorf("CtxUserID = %q, want %q", capturer.UserID, expectedUserID.String())
	}
	if capturer.RequestID != "req-12345" {
		t.Errorf("CtxRequestID = %q, want %q", capturer.RequestID, "req-12345")
	}
}

func TestGatewayAuth_ValidKey_RecordsLastUsed(t *testing.T) {
	plaintextKey := "dt-last-used-key"
	expectedKeyID := uuid.New()
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, keyHash string) (*domain.APIKey, error) {
			if keyHash != keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!") {
				return nil, nil
			}
			return &domain.APIKey{
				ID:     expectedKeyID,
				Status: domain.APIKeyStatusActive,
				UserID: uuid.New(),
			}, nil
		},
	}

	application := appWithMockRepo(repo)
	capturer := &gatewayContextCapturer{}
	mw := GatewayAuth(application)
	handler := mw(capturer)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// UpdateLastUsed runs in a detached goroutine; poll briefly for it.
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		repo.mu.Lock()
		called := repo.updateLastUsedCalled
		repo.mu.Unlock()
		if called > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.updateLastUsedCalled == 0 {
		t.Fatal("UpdateLastUsed was never called for a valid key")
	}
	if repo.lastUpdatedKeyID != expectedKeyID {
		t.Errorf("UpdateLastUsed id = %v, want %v", repo.lastUpdatedKeyID, expectedKeyID)
	}
}

func TestGatewayAuth_InvalidKey_DoesNotRecordLastUsed(t *testing.T) {
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, keyHash string) (*domain.APIKey, error) {
			return nil, nil // unknown hash
		},
	}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer sk-invalid-key")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if repo.updateLastUsedCalled != 0 {
		t.Errorf("UpdateLastUsed called %d times for an invalid key, want 0", repo.updateLastUsedCalled)
	}
}

func TestGatewayAuth_MissingAuthorizationHeader_401(t *testing.T) {
	repo := &mockAPIKeyRepo{}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGatewayAuth_NoBearerPrefix_401(t *testing.T) {
	repo := &mockAPIKeyRepo{}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Basic somecredentials")
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGatewayAuth_EmptyAPIKey_401(t *testing.T) {
	repo := &mockAPIKeyRepo{}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer ")
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGatewayAuth_InvalidKeyHash_401(t *testing.T) {
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, keyHash string) (*domain.APIKey, error) {
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer unknown-key")
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusUnauthorized, w.Body.String())
	}
}

func TestGatewayAuth_DisabledKey_403(t *testing.T) {
	plaintextKey := "dt-my-disabled-key"
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!") {
				return &domain.APIKey{
					ID:     uuid.New(),
					UserID: uuid.New(),
					Status: domain.APIKeyStatusDisabled,
				}, nil
			}
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestGatewayAuth_RevokedKey_403(t *testing.T) {
	plaintextKey := "dt-my-revoked-key"
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!") {
				return &domain.APIKey{
					ID:     uuid.New(),
					UserID: uuid.New(),
					Status: domain.APIKeyStatusRevoked,
				}, nil
			}
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d, body = %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestGatewayAuth_OverLimitKey_200_StillStoresIdentity(t *testing.T) {
	plaintextKey := "dt-my-overlimit-key"
	expectedKeyID := uuid.New()
	expectedUserID := uuid.New()

	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!") {
				return &domain.APIKey{
					ID:     expectedKeyID,
					UserID: expectedUserID,
					Status: domain.APIKeyStatusOverLimit,
				}, nil
			}
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	capturer := &gatewayContextCapturer{}
	mw := GatewayAuth(application)
	handler := mw(capturer)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	req.Header.Set("X-Request-ID", "req-12345")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturer.APIKeyID != expectedKeyID.String() {
		t.Errorf("CtxAPIKeyID = %q, want %q", capturer.APIKeyID, expectedKeyID.String())
	}
	if capturer.UserID != expectedUserID.String() {
		t.Errorf("CtxUserID = %q, want %q", capturer.UserID, expectedUserID.String())
	}
}

func TestGatewayAuth_LowercaseXRequestID_200(t *testing.T) {
	plaintextKey := "dt-lowercase-request-id-key"
	expectedKeyID := uuid.New()
	expectedUserID := uuid.New()

	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, hash string) (*domain.APIKey, error) {
			if hash == keyhash.Hash(plaintextKey, "test-hmac-secret-32-bytes-!!!!") {
				return &domain.APIKey{
					ID:     expectedKeyID,
					UserID: expectedUserID,
					Status: domain.APIKeyStatusActive,
				}, nil
			}
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	capturer := &gatewayContextCapturer{}
	mw := GatewayAuth(application)
	handler := mw(capturer)

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintextKey)
	req.Header.Set("x-request-id", "lowercase-req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if capturer.RequestID != "lowercase-req-1" {
		t.Errorf("CtxRequestID = %q, want %q", capturer.RequestID, "lowercase-req-1")
	}
}

func TestGatewayAuth_ErrorResponseContentTypeJSON(t *testing.T) {
	repo := &mockAPIKeyRepo{}
	application := appWithMockRepo(repo)
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if ct := w.Header().Get("Content-Type"); !containsJSONContentType(ct) {
		t.Errorf("Content-Type = %q, expected JSON content type", ct)
	}
}

func containsJSONContentType(ct string) bool {
	return ct == "application/json" || len(ct) > 16 && ct[:16] == "application/json"
}

func TestGatewayAuth_ContextValuesNotSetOnAuthFailure(t *testing.T) {
	repo := &mockAPIKeyRepo{
		findByHashFn: func(ctx context.Context, keyHash string) (*domain.APIKey, error) {
			return nil, nil
		},
	}
	application := appWithMockRepo(repo)
	var capturedCtx context.Context
	mw := GatewayAuth(application)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCtx = r.Context()
		t.Error("next handler should not be called when auth fails")
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer invalid-key")
	req.Header.Set("X-Request-ID", "req-1")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if capturedCtx != nil {
		t.Error("next handler was called despite auth failure")
	}
}

// --- AdminAuth tests ---

func TestAdminAuth_AdminRole_Passes(t *testing.T) {
	capturer := &contextCapturer{}
	middleware := AdminAuth()
	handler := middleware(capturer)

	ctx := context.WithValue(context.Background(), CtxRoleKey, "admin")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil).WithContext(ctx)
	req = setUserInContextForAuth(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}

func TestAdminAuth_UserRole_Forbidden(t *testing.T) {
	middleware := AdminAuth()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	ctx := context.WithValue(context.Background(), CtxRoleKey, "user")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil).WithContext(ctx)
	req = setUserInContextForAuth(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if !containsJSONContentType(ct) {
		t.Errorf("Content-Type = %q, expected JSON content type", ct)
	}
}

func TestAdminAuth_MissingRole_Forbidden(t *testing.T) {
	middleware := AdminAuth()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil)
	req = setUserInContextForAuth(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAdminAuth_EmptyRole_Forbidden(t *testing.T) {
	middleware := AdminAuth()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	ctx := context.WithValue(context.Background(), CtxRoleKey, "")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil).WithContext(ctx)
	req = setUserInContextForAuth(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAdminAuth_InvalidRoleString_Forbidden(t *testing.T) {
	middleware := AdminAuth()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	ctx := context.WithValue(context.Background(), CtxRoleKey, "superadmin")
	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil).WithContext(ctx)
	req = setUserInContextForAuth(req, uuid.New().String())
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestAdminAuth_ErrorResponseFormat(t *testing.T) {
	middleware := AdminAuth()
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/models", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if resp["error"] != "Admin access required" {
		t.Errorf("error = %q, want %q", resp["error"], "Admin access required")
	}
}

// setUserInContextForAuth injects user ID into request context.
func setUserInContextForAuth(req *http.Request, userID string) *http.Request {
	ctx := req.Context()
	ctx = context.WithValue(ctx, CtxUserID, userID)
	return req.WithContext(ctx)
}

func TestConsoleAuth_DisabledUser_Rejected(t *testing.T) {
	// Arrange: user exists but is banned — a still-valid JWT must be
	// rejected immediately (stateless-token revocation).
	secret := "test-jwt-disabled-secret-32-byte!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	token := generateTestJWT(t, userID, "disabled@test.com", "Disabled", secret)

	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: "disabled@test.com", DisplayName: "Disabled", Role: "user", Status: domain.UserStatusBanned}))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for banned user", w.Code)
	}
}

func TestConsoleAuth_DeletedUser_Rejected(t *testing.T) {
	// Arrange: user row no longer exists — token must be rejected.
	secret := "test-jwt-deleted-secret-32-byte!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	token := generateTestJWT(t, userID, "gone@test.com", "Gone", secret)

	middleware := ConsoleAuth(consoleTestApp(cfg, nil))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/console/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401 for deleted user", w.Code)
	}
}

func TestConsoleAuth_RoleFromDBOverridesToken(t *testing.T) {
	// Arrange: token carries role "admin" but the DB says "user" — the
	// live role must win so a demoted admin loses admin access immediately.
	secret := "test-jwt-role-secret-32-byte!!"
	cfg := configWithJWTSecret(secret)
	userID := uuid.New()
	token, _ := jwtutil.GenerateToken(userID, "demoted@test.com", "Demoted", "admin", "personal", "", "", secret, 1)

	var gotRole string
	middleware := ConsoleAuth(consoleTestApp(cfg, &domain.User{ID: userID, Email: "demoted@test.com", DisplayName: "Demoted", Role: "user", Status: domain.UserStatusActive}))
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRole, _ = jwtutil.RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if gotRole != "user" {
		t.Errorf("role from context = %q, want %q (DB role must override token)", gotRole, "user")
	}
}
