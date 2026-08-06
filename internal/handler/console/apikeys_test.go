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
	"github.com/deeptrols/api/internal/pkg/keyhash"
	"github.com/deeptrols/api/internal/repository/apikey"
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"golang.org/x/crypto/bcrypt"
)

// appForAPIKeyTest creates a minimal App with repos wired for API key tests.
func appForAPIKeyTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-for-apikey-32-byte",
			ExpiryHours: 24,
		},
		Encryption: config.EncryptionConfig{
			Key: "test-encryption-key-32bytes-done", // must be exactly 32 bytes for AES-256
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		APIKeys: apikey.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForAPIKeyTest creates a user with bcrypt hash.
func seedUserForAPIKeyTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
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
		t.Fatalf("seedUserForAPIKeyTest: create: %v", err)
	}
	return u
}

// setUserInAPIKeyContext adds a user ID to the request context.
func setUserInAPIKeyContext(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	return r.WithContext(ctx)
}

// =============================================================================
// HandleListAPIKeys Tests
// =============================================================================

func TestHandleListAPIKeys_NoAuth(t *testing.T) {
	a := appForAPIKeyTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/console/api-keys", nil)
	w := httptest.NewRecorder()

	handler := HandleListAPIKeys(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListAPIKeys_EmptyList(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "empty-keys@example.com", "pass123", "Empty Keys User")

	req := httptest.NewRequest(http.MethodGet, "/api/console/api-keys", nil)
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListAPIKeys(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	data, ok := resp["data"].([]interface{})
	if !ok {
		t.Fatal("data field is not an array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty array, got %d items", len(data))
	}
}

func TestHandleListAPIKeys_ReturnsUserKeys(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "keys-owner@example.com", "pass123", "Keys Owner")

	// Seed API keys directly via repository
	now := time.Now().UTC()
	key1 := domain.APIKey{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash1",
		MaskedKey:       "dt-sk-****abc1",
		Name:            "Key One",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	key2 := domain.APIKey{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash2",
		MaskedKey:       "dt-sk-****def2",
		Name:            "Key Two",
		Status:          domain.APIKeyStatusDisabled,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := a.APIKeys.Create(context.Background(), &key1); err != nil {
		t.Fatalf("create key1: %v", err)
	}
	if err := a.APIKeys.Create(context.Background(), &key2); err != nil {
		t.Fatalf("create key2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/console/api-keys", nil)
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListAPIKeys(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []apiKeyResponse `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	// Verify key fields
	foundKey1, foundKey2 := false, false
	for _, k := range resp.Data {
		if k.ID == key1.ID.String() {
			foundKey1 = true
			if k.Name != key1.Name {
				t.Errorf("key1 name = %s, want %s", k.Name, key1.Name)
			}
			if k.Status != string(key1.Status) {
				t.Errorf("key1 status = %s, want %s", k.Status, key1.Status)
			}
			if k.MaskedKey != key1.MaskedKey {
				t.Errorf("key1 masked_key = %s, want %s", k.MaskedKey, key1.MaskedKey)
			}
		}
		if k.ID == key2.ID.String() {
			foundKey2 = true
			if k.Status != string(key2.Status) {
				t.Errorf("key2 status = %s, want %s", k.Status, key2.Status)
			}
		}
	}
	if !foundKey1 {
		t.Error("key1 not found in response")
	}
	if !foundKey2 {
		t.Error("key2 not found in response")
	}
}

func TestHandleListAPIKeys_OnlyReturnsOwnKeys(t *testing.T) {
	a := appForAPIKeyTest(t)
	userA := seedUserForAPIKeyTest(t, a, "userA-keys@example.com", "passA", "User A")
	userB := seedUserForAPIKeyTest(t, a, "userB-keys@example.com", "passB", "User B")

	// Create keys for both users
	keyA := domain.APIKey{
		ID:              uuid.New(),
		UserID:          userA.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-user-a",
		MaskedKey:       "dt-sk-****aaaa",
		Name:            "A's Key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	keyB := domain.APIKey{
		ID:              uuid.New(),
		UserID:          userB.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-user-b",
		MaskedKey:       "dt-sk-****bbbb",
		Name:            "B's Key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &keyA); err != nil {
		t.Fatalf("create keyA: %v", err)
	}
	if err := a.APIKeys.Create(context.Background(), &keyB); err != nil {
		t.Fatalf("create keyB: %v", err)
	}

	// Request as user A
	req := httptest.NewRequest(http.MethodGet, "/api/console/api-keys", nil)
	req = setUserInAPIKeyContext(req, userA.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListAPIKeys(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []apiKeyResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 key for user A, got %d", len(resp.Data))
	}
	if resp.Data[0].Name != "A's Key" {
		t.Errorf("got key name = %s, want 'A's Key'", resp.Data[0].Name)
	}
}

// =============================================================================
// HandleCreateAPIKey Tests
// =============================================================================

func TestHandleCreateAPIKey_NoAuth(t *testing.T) {
	a := appForAPIKeyTest(t)

	body := map[string]string{"name": "Test Key"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateAPIKey_InvalidBody(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "badbody@example.com", "pass", "Bad Body")

	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateAPIKey_Success(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "create-key@example.com", "pass", "Create Key User")

	body := map[string]interface{}{
		"name": "My New Key",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Plaintext string `json:"plaintext"`
		KeyPrefix string `json:"key_prefix"`
		MaskedKey string `json:"masked_key"`
		Warning   string `json:"warning"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Validate response
	if resp.ID == "" {
		t.Fatal("id is empty")
	}
	if resp.Name != "My New Key" {
		t.Errorf("name = %s, want 'My New Key'", resp.Name)
	}
	if resp.Plaintext == "" {
		t.Fatal("plaintext is empty")
	}
	if !bytes.HasPrefix([]byte(resp.Plaintext), []byte("dt-sk-")) {
		t.Errorf("plaintext does not start with 'dt-sk-': %s", resp.Plaintext)
	}
	if resp.MaskedKey == "" {
		t.Fatal("masked_key is empty")
	}
	if resp.Warning == "" {
		t.Fatal("warning is empty")
	}
	if resp.KeyPrefix != "dt-sk-" {
		t.Errorf("key_prefix = %s, want 'dt-sk-'", resp.KeyPrefix)
	}

	// Verify the plaintext is NOT stored in DB (only HMAC-SHA256 hash is stored)
	expectedHash := keyhash.Hash(resp.Plaintext, a.Config.Encryption.Key)
	storedKey, err := a.APIKeys.FindByID(context.Background(), uuid.MustParse(resp.ID))
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if storedKey.KeyHash != expectedHash {
		t.Errorf("stored hash = %s, want %s", storedKey.KeyHash, expectedHash)
	}
	if storedKey.MaskedKey == "" {
		t.Fatal("stored masked_key is empty")
	}
	if storedKey.UserID != seedUser.ID {
		t.Errorf("stored key UserID = %s, want %s", storedKey.UserID, seedUser.ID)
	}
}

func TestHandleCreateAPIKey_WithOptionalFields(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "opt-fields@example.com", "pass", "Optional Fields")

	body := map[string]interface{}{
		"name":             "Restricted Key",
		"allowed_models":   []string{"gpt-4o", "claude-sonnet"},
		"source_whitelist": []string{"192.168.0.0/16", "10.0.0.0/8"},
		"monthly_limit":    "100.50",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct{ ID string }
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify stored fields
	storedKey, err := a.APIKeys.FindByID(context.Background(), uuid.MustParse(resp.ID))
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if len(storedKey.AllowedModels) != 2 {
		t.Errorf("allowed_models length = %d, want 2", len(storedKey.AllowedModels))
	}
	if len(storedKey.SourceWhitelist) != 2 {
		t.Errorf("source_whitelist length = %d, want 2", len(storedKey.SourceWhitelist))
	}
	if !storedKey.MonthlyLimit.Equal(decimal.RequireFromString("100.50")) {
		t.Errorf("monthly_limit = %s, want 100.50", storedKey.MonthlyLimit.String())
	}
}

func TestHandleCreateAPIKey_EmptyName(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "empty-name@example.com", "pass", "Empty Name")

	body := map[string]string{"name": ""}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/console/api-keys", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleUpdateAPIKey Tests
// =============================================================================

func TestHandleUpdateAPIKey_NoAuth(t *testing.T) {
	a := appForAPIKeyTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/console/api-keys/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleUpdateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdateAPIKey_NotFound(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "update-notfound@example.com", "pass", "Not Found")

	nonExistentID := uuid.New().String()
	body := map[string]string{"name": "Updated Name"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/api-keys/"+nonExistentID, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleUpdateAPIKey_WrongOwner(t *testing.T) {
	a := appForAPIKeyTest(t)
	userA := seedUserForAPIKeyTest(t, a, "update-ownerA@example.com", "passA", "Owner A")
	userB := seedUserForAPIKeyTest(t, a, "update-ownerB@example.com", "passB", "Owner B")

	// Create key for user A
	key := domain.APIKey{
		ID:              uuid.New(),
		UserID:          userA.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-owner-a",
		MaskedKey:       "dt-sk-****owner",
		Name:            "A's Key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// User B tries to update user A's key
	body := map[string]string{"name": "Stolen Key"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/api-keys/"+key.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", key.ID.String())
	req = setUserInAPIKeyContext(req, userB.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleUpdateAPIKey_Success(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "update-success@example.com", "pass", "Success")

	// Create key
	key := domain.APIKey{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-update-success",
		MaskedKey:       "dt-sk-****orig",
		Name:            "Original Name",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// Update
	body := map[string]interface{}{
		"name":          "Updated Name",
		"monthly_limit": "200.00",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/console/api-keys/"+key.ID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", key.ID.String())
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify update in DB
	updated, err := a.APIKeys.FindByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if updated.Name != "Updated Name" {
		t.Errorf("name = %s, want 'Updated Name'", updated.Name)
	}
	if !updated.MonthlyLimit.Equal(decimal.RequireFromString("200.00")) {
		t.Errorf("monthly_limit = %s, want 200.00", updated.MonthlyLimit.String())
	}
}

// =============================================================================
// HandleDeleteAPIKey Tests
// =============================================================================

func TestHandleDeleteAPIKey_NoAuth(t *testing.T) {
	a := appForAPIKeyTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/console/api-keys/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleDeleteAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteAPIKey_NotFound(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "delete-notfound@example.com", "pass", "Not Found Delete")

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/console/api-keys/"+nonExistentID, nil)
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleDeleteAPIKey_WrongOwner(t *testing.T) {
	a := appForAPIKeyTest(t)
	userA := seedUserForAPIKeyTest(t, a, "delete-ownerA@example.com", "passA", "Owner A")
	userB := seedUserForAPIKeyTest(t, a, "delete-ownerB@example.com", "passB", "Owner B")

	key := domain.APIKey{
		ID:              uuid.New(),
		UserID:          userA.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-delete-owner",
		MaskedKey:       "dt-sk-****del",
		Name:            "A's Delete Key",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/console/api-keys/"+key.ID.String(), nil)
	req = chiRouteCtx(req, "id", key.ID.String())
	req = setUserInAPIKeyContext(req, userB.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleDeleteAPIKey_Success_SoftDelete(t *testing.T) {
	a := appForAPIKeyTest(t)
	seedUser := seedUserForAPIKeyTest(t, a, "delete-success@example.com", "pass", "Success Delete")

	key := domain.APIKey{
		ID:              uuid.New(),
		UserID:          seedUser.ID,
		KeyPrefix:       "dt-sk-",
		KeyHash:         "hash-delete-success",
		MaskedKey:       "dt-sk-****sd",
		Name:            "To Be Deleted",
		Status:          domain.APIKeyStatusActive,
		OverLimitAction: domain.OverLimitBlock,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := a.APIKeys.Create(context.Background(), &key); err != nil {
		t.Fatalf("create key: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/console/api-keys/"+key.ID.String(), nil)
	req = chiRouteCtx(req, "id", key.ID.String())
	req = setUserInAPIKeyContext(req, seedUser.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteAPIKey(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Verify soft-delete: status should be "revoked" and revoked_at should be set
	deleted, err := a.APIKeys.FindByID(context.Background(), key.ID)
	if err != nil {
		t.Fatalf("FindByID after delete: %v", err)
	}
	if deleted.Status != domain.APIKeyStatusRevoked {
		t.Errorf("status = %s, want %s", deleted.Status, domain.APIKeyStatusRevoked)
	}
	if deleted.RevokedAt == nil {
		t.Error("revoked_at should not be nil after soft delete")
	}
}

// =============================================================================
// Chi URL param routing helpers for tests
// =============================================================================

// chiRouteCtx sets a chi URL param on the request context.
func chiRouteCtx(r *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}
