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
	"github.com/deeptrols/api/internal/repository/testutil"
	"github.com/deeptrols/api/internal/repository/user"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// appForChannelsTest creates a minimal App with a pool for channel tests.
func appForChannelsTest(t *testing.T) *app.App {
	t.Helper()
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:      "test-jwt-secret-channel-32-bytes",
			ExpiryHours: 24,
		},
	}

	return &app.App{
		Pool:    pool,
		Config:  cfg,
		Users:   user.NewPostgresRepository(pool),
		Healthy: true,
	}
}

// seedUserForChannelsTest creates a user with bcrypt hash.
func seedUserForChannelsTest(t *testing.T, a *app.App, email, password, displayName string) *domain.User {
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
		UserType:     domain.UserTypePersonal,
		Status:       domain.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := a.Users.Create(context.Background(), u); err != nil {
		t.Fatalf("seedUserForChannelsTest: create: %v", err)
	}
	return u
}

// seedModelForChannelsTest creates a model and returns its ID.
func seedModelForChannelsTest(t *testing.T, a *app.App, code, provider string) uuid.UUID {
	t.Helper()
	modelID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO models (id, code, provider, category, display_name, status, release_stage, created_at, updated_at)
		 VALUES ($1, $2, $3, 'chat', $2, 'active', 'GA', $4, $4)`,
		modelID, code, provider, now,
	)
	if err != nil {
		t.Fatalf("seedModelForChannelsTest: %v", err)
	}
	return modelID
}

// setAdminCtx adds user_id and role="admin" to the request context.
func setAdminCtx(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "admin")
	return r.WithContext(ctx)
}

// setUserCtx adds user_id without admin role to the request context.
func setUserCtx(r *http.Request, userID string) *http.Request {
	ctx := r.Context()
	ctx = context.WithValue(ctx, jwtutil.CtxUserIDKey, userID)
	ctx = context.WithValue(ctx, jwtutil.CtxRoleKey, "user")
	return r.WithContext(ctx)
}

// chiRouteMultiCtx adds multiple URL params to the request context.
func chiRouteMultiCtx(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, rctx)
	return r.WithContext(ctx)
}

// seedChannelForTest inserts a channel and optionally an instance, returns channel_id.
func seedChannelForTest(t *testing.T, a *app.App, name string, modelID uuid.UUID) uuid.UUID {
	t.Helper()
	channelID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO channels (id, name, model_id, pool_type, status, health_score, health_status, weight, max_concurrency, created_at, updated_at)
		 VALUES ($1, $2, $3, 'shared', 'active', 100, 'healthy', 100, 10, $4, $4)`,
		channelID, name, modelID, now,
	)
	if err != nil {
		t.Fatalf("seedChannelForTest: %v", err)
	}
	return channelID
}

// seedChannelWithInstance inserts a channel with an instance.
func seedChannelWithInstance(t *testing.T, a *app.App, name string, modelID uuid.UUID) (uuid.UUID, uuid.UUID) {
	t.Helper()
	channelID := seedChannelForTest(t, a, name, modelID)
	instanceID := uuid.New()
	now := time.Now().UTC()
	_, err := a.Pool.Exec(context.Background(),
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, current_load, max_load, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', 'https://api.openai.com/v1', 0, 10, '{}', 'active', $3, $3)`,
		instanceID, channelID, now,
	)
	if err != nil {
		t.Fatalf("seedChannelWithInstance: %v", err)
	}
	return channelID, instanceID
}

// =============================================================================
// HandleListChannels Tests
// =============================================================================

func TestHandleListChannels_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	w := httptest.NewRecorder()

	handler := HandleListChannels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleListChannels_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-notadmin@example.com", "pass123", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListChannels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleListChannels_EmptyList(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-empty@example.com", "pass", "Admin Empty")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListChannels(a)
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

func TestHandleListChannels_ReturnsChannels(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-list@example.com", "pass", "Admin List")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	ch1, _ := seedChannelWithInstance(t, a, "OpenAI Prod", modelID)
	ch2, _ := seedChannelWithInstance(t, a, "OpenAI Staging", modelID)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListChannels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data  []channelResponse `json:"data"`
		Total int               `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 channels, got %d", len(resp.Data))
	}
	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}

	foundCh1, foundCh2 := false, false
	for _, ch := range resp.Data {
		switch ch.ID {
		case ch1.String():
			foundCh1 = true
			if ch.Name != "OpenAI Prod" {
				t.Errorf("ch1 name = %s, want 'OpenAI Prod'", ch.Name)
			}
			if ch.InstanceCount != 1 {
				t.Errorf("ch1 instance_count = %d, want 1", ch.InstanceCount)
			}
		case ch2.String():
			foundCh2 = true
			if ch.Name != "OpenAI Staging" {
				t.Errorf("ch2 name = %s, want 'OpenAI Staging'", ch.Name)
			}
			if ch.InstanceCount != 1 {
				t.Errorf("ch2 instance_count = %d, want 1", ch.InstanceCount)
			}
		}
	}
	if !foundCh1 {
		t.Errorf("channel %s not found in response", ch1)
	}
	if !foundCh2 {
		t.Errorf("channel %s not found in response", ch2)
	}
}

func TestHandleListChannels_ChannelWithoutInstances(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-noinst@example.com", "pass", "Admin No Instance")
	modelID := seedModelForChannelsTest(t, a, "claude-sonnet", "anthropic")

	chID := seedChannelForTest(t, a, "No Instance Channel", modelID)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels", nil)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleListChannels(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data []channelResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != chID.String() {
		t.Errorf("id = %s, want %s", resp.Data[0].ID, chID.String())
	}
	if resp.Data[0].InstanceCount != 0 {
		t.Errorf("instance_count = %d, want 0", resp.Data[0].InstanceCount)
	}
}

// =============================================================================
// HandleGetChannel Tests
// =============================================================================

func TestHandleGetChannel_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleGetChannel_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-get-nonadmin@example.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/some-id", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleGetChannel_NotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-get-404@example.com", "pass", "Admin Get 404")

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/"+nonExistentID, nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteCtx(req, "id", nonExistentID)
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleGetChannel_InvalidID(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-get-badid@example.com", "pass", "Admin Bad ID")

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/not-a-uuid", nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteCtx(req, "id", "not-a-uuid")
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleGetChannel_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-get@example.com", "pass", "Admin Get")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID, instID := seedChannelWithInstance(t, a, "OpenAI Prod", modelID)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/"+chID.String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteCtx(req, "id", chID.String())
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data channelDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID != chID.String() {
		t.Errorf("id = %s, want %s", resp.Data.ID, chID.String())
	}
	if resp.Data.Name != "OpenAI Prod" {
		t.Errorf("name = %s, want 'OpenAI Prod'", resp.Data.Name)
	}
	if resp.Data.ModelID != modelID.String() {
		t.Errorf("model_id = %s, want %s", resp.Data.ModelID, modelID.String())
	}
	if resp.Data.Status != "active" {
		t.Errorf("status = %s, want 'active'", resp.Data.Status)
	}

	if len(resp.Data.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(resp.Data.Instances))
	}
	if resp.Data.Instances[0].ID != instID.String() {
		t.Errorf("instance id = %s, want %s", resp.Data.Instances[0].ID, instID.String())
	}
}

func TestHandleGetChannel_WithMultipleInstances(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-get-multi@example.com", "pass", "Admin Get Multi")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Multi Instance", modelID)
	now := time.Now().UTC()
	ctx := context.Background()

	inst1ID := uuid.New()
	inst2ID := uuid.New()

	_, err := a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, current_load, max_load, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', 'https://api.openai.com/v1', 0, 10, '{}', 'active', $3, $3)`,
		inst1ID, chID, now,
	)
	if err != nil {
		t.Fatalf("seed instance 1: %v", err)
	}
	_, err = a.Pool.Exec(ctx,
		`INSERT INTO channel_instances (id, channel_id, instance_type, base_url, current_load, max_load, config, status, created_at, updated_at)
		 VALUES ($1, $2, 'openai', 'https://api2.openai.com/v1', 0, 20, '{}', 'active', $3, $3)`,
		inst2ID, chID, now,
	)
	if err != nil {
		t.Fatalf("seed instance 2: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/channels/"+chID.String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteCtx(req, "id", chID.String())
	w := httptest.NewRecorder()

	handler := HandleGetChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Data channelDetailResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(resp.Data.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(resp.Data.Instances))
	}
}

// =============================================================================
// HandleCreateChannel Tests
// =============================================================================

func TestHandleCreateChannel_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	body := map[string]interface{}{
		"name":            "Test Channel",
		"model_id":        uuid.New().String(),
		"pool_type":       "shared",
		"weight":          100,
		"max_concurrency": 10,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleCreateChannel_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create-nonadmin@example.com", "pass", "Not Admin")

	body := map[string]interface{}{"name": "Test", "model_id": uuid.New().String()}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleCreateChannel_InvalidBody(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create-badbody@example.com", "pass", "Admin Bad Body")

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateChannel_MissingRequiredFields(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create-missing@example.com", "pass", "Admin Missing")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty name", map[string]interface{}{"name": "", "model_id": modelID.String()}},
		{"missing model_id", map[string]interface{}{"name": "Test"}},
		{"invalid model_id", map[string]interface{}{"name": "Test", "model_id": "not-a-uuid"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = setAdminCtx(req, user.ID.String())
			w := httptest.NewRecorder()

			handler := HandleCreateChannel(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleCreateChannel_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create@example.com", "pass", "Admin Create")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	body := map[string]interface{}{
		"name":            "OpenAI Shared Channel",
		"model_id":        modelID.String(),
		"pool_type":       "shared",
		"weight":          100,
		"max_concurrency": 10,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data channelResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID == "" {
		t.Fatal("id is empty")
	}
	if resp.Data.Name != "OpenAI Shared Channel" {
		t.Errorf("name = %s, want 'OpenAI Shared Channel'", resp.Data.Name)
	}
	if resp.Data.ModelID != modelID.String() {
		t.Errorf("model_id = %s, want %s", resp.Data.ModelID, modelID.String())
	}
	if resp.Data.PoolType != "shared" {
		t.Errorf("pool_type = %s, want 'shared'", resp.Data.PoolType)
	}
	if resp.Data.Weight != 100 {
		t.Errorf("weight = %d, want 100", resp.Data.Weight)
	}
	if resp.Data.MaxConcurrency != 10 {
		t.Errorf("max_concurrency = %d, want 10", resp.Data.MaxConcurrency)
	}
	if resp.Data.Status != "active" {
		t.Errorf("status = %s, want 'active'", resp.Data.Status)
	}

	// Verify database state
	var count int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM channels WHERE name = $1`, "OpenAI Shared Channel",
	).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 channel in DB, got %d (err: %v)", count, err)
	}
}

func TestHandleCreateChannel_WithDefaults(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create-defaults@example.com", "pass", "Admin Defaults")
	modelID := seedModelForChannelsTest(t, a, "claude-sonnet", "anthropic")

	body := map[string]interface{}{
		"name":     "Minimal Channel",
		"model_id": modelID.String(),
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data channelResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify defaults
	if resp.Data.PoolType != "shared" {
		t.Errorf("pool_type default = %s, want 'shared'", resp.Data.PoolType)
	}
	if resp.Data.Weight != 100 {
		t.Errorf("weight default = %d, want 100", resp.Data.Weight)
	}
	if resp.Data.MaxConcurrency != 10 {
		t.Errorf("max_concurrency default = %d, want 10", resp.Data.MaxConcurrency)
	}
}

func TestHandleCreateChannel_ModelNotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-create-nomodel@example.com", "pass", "Admin No Model")

	body := map[string]interface{}{
		"name":     "Bad Channel",
		"model_id": uuid.New().String(),
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleCreateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// =============================================================================
// HandleUpdateChannel Tests
// =============================================================================

func TestHandleUpdateChannel_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleUpdateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUpdateChannel_NotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-upd-404@example.com", "pass", "Admin Upd 404")

	nonExistentID := uuid.New().String()
	body := map[string]string{"name": "Updated"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/"+nonExistentID, bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleUpdateChannel_InvalidID(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-upd-badid@example.com", "pass", "Admin Upd BadID")

	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleUpdateChannel_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-upd@example.com", "pass", "Admin Update")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Original Name", modelID)

	body := map[string]interface{}{
		"name":            "Updated Name",
		"weight":          50,
		"max_concurrency": 5,
		"pool_type":       "dedicated",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/"+chID.String(), bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Data channelResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.Name != "Updated Name" {
		t.Errorf("name = %s, want 'Updated Name'", resp.Data.Name)
	}
	if resp.Data.Weight != 50 {
		t.Errorf("weight = %d, want 50", resp.Data.Weight)
	}
	if resp.Data.MaxConcurrency != 5 {
		t.Errorf("max_concurrency = %d, want 5", resp.Data.MaxConcurrency)
	}
	if resp.Data.PoolType != "dedicated" {
		t.Errorf("pool_type = %s, want 'dedicated'", resp.Data.PoolType)
	}

	// Verify DB state
	var name string
	var weight int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT name, weight FROM channels WHERE id = $1`, chID,
	).Scan(&name, &weight)
	if err != nil {
		t.Fatalf("query channel: %v", err)
	}
	if name != "Updated Name" {
		t.Errorf("db name = %s, want 'Updated Name'", name)
	}
	if weight != 50 {
		t.Errorf("db weight = %d, want 50", weight)
	}
}

func TestHandleUpdateChannel_InvalidBody(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-upd-bad@example.com", "pass", "Admin Update Bad")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	req := httptest.NewRequest(http.MethodPut, "/api/admin/channels/"+chID.String(), bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleUpdateChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =============================================================================
// HandleDeleteChannel Tests
// =============================================================================

func TestHandleDeleteChannel_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleDeleteChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleDeleteChannel_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-del-nonadmin@example.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/some-id", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleDeleteChannel_NotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-del-404@example.com", "pass", "Admin Del 404")

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+nonExistentID, nil)
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleDeleteChannel_InvalidID(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-del-badid@example.com", "pass", "Admin Del BadID")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/not-a-uuid", nil)
	req = chiRouteCtx(req, "id", "not-a-uuid")
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleDeleteChannel_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-del@example.com", "pass", "Admin Delete")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID, _ := seedChannelWithInstance(t, a, "To Be Deleted", modelID)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+chID.String(), nil)
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleDeleteChannel(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "deleted" {
		t.Errorf("status = %s, want 'deleted'", resp["status"])
	}

	// Verify soft-delete
	var status string
	err := a.Pool.QueryRow(context.Background(),
		`SELECT status FROM channels WHERE id = $1`, chID,
	).Scan(&status)
	if err != nil {
		t.Fatalf("query channel: %v", err)
	}
	if status != "inactive" {
		t.Errorf("channel status = %s, want 'inactive'", status)
	}
}

// =============================================================================
// HandleAddInstance Tests
// =============================================================================

func TestHandleAddInstance_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	body := map[string]interface{}{"instance_type": "openai", "base_url": "https://api.openai.com/v1", "max_load": 10}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/some-id/instances", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleAddInstance_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-nonadmin@example.com", "pass", "Not Admin")

	body := map[string]interface{}{"instance_type": "openai", "base_url": "https://api.openai.com/v1", "max_load": 10}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/some-id/instances", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
}

func TestHandleAddInstance_ChannelNotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-noch@example.com", "pass", "Admin No Channel")

	body := map[string]interface{}{"instance_type": "openai", "base_url": "https://api.openai.com/v1", "max_load": 10}
	bodyBytes, _ := json.Marshal(body)

	nonExistentID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/"+nonExistentID+"/instances", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", nonExistentID)
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleAddInstance_InvalidBody(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-badbody@example.com", "pass", "Admin Bad Body")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/"+chID.String()+"/instances", bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleAddInstance_MissingRequiredFields(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-missing@example.com", "pass", "Admin Missing")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	tests := []struct {
		name string
		body map[string]interface{}
	}{
		{"empty instance_type", map[string]interface{}{"instance_type": "", "base_url": "https://api.example.com", "max_load": 10}},
		{"empty base_url", map[string]interface{}{"instance_type": "openai", "base_url": "", "max_load": 10}},
		{"zero max_load", map[string]interface{}{"instance_type": "openai", "base_url": "https://api.example.com", "max_load": 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/"+chID.String()+"/instances", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			req = chiRouteCtx(req, "id", chID.String())
			req = setAdminCtx(req, user.ID.String())
			w := httptest.NewRecorder()

			handler := HandleAddInstance(a)
			handler.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusBadRequest, w.Body.String())
			}
		})
	}
}

func TestHandleAddInstance_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-add@example.com", "pass", "Admin Add Instance")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	body := map[string]interface{}{
		"instance_type": "openai",
		"base_url":      "https://api.openai.com/v1",
		"max_load":      10,
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/"+chID.String()+"/instances", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	var resp struct {
		Data instanceResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp.Data.ID == "" {
		t.Fatal("instance id is empty")
	}
	if resp.Data.ChannelID != chID.String() {
		t.Errorf("channel_id = %s, want %s", resp.Data.ChannelID, chID.String())
	}
	if resp.Data.InstanceType != "openai" {
		t.Errorf("instance_type = %s, want 'openai'", resp.Data.InstanceType)
	}
	if resp.Data.BaseURL != "https://api.openai.com/v1" {
		t.Errorf("base_url = %s, want 'https://api.openai.com/v1'", resp.Data.BaseURL)
	}
	if resp.Data.MaxLoad != 10 {
		t.Errorf("max_load = %d, want 10", resp.Data.MaxLoad)
	}
	if resp.Data.Status != "active" {
		t.Errorf("status = %s, want 'active'", resp.Data.Status)
	}

	// Verify database
	var count int
	err := a.Pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM channel_instances WHERE channel_id = $1`, chID,
	).Scan(&count)
	if err != nil || count != 1 {
		t.Errorf("expected 1 instance in DB, got %d (err: %v)", count, err)
	}
}

func TestHandleAddInstance_WithConfig(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-inst-config@example.com", "pass", "Admin Config")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	body := map[string]interface{}{
		"instance_type": "azure",
		"base_url":      "https://my-resource.openai.azure.com",
		"max_load":      5,
		"config":        map[string]string{"deployment": "gpt-4o-deployment"},
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/admin/channels/"+chID.String()+"/instances", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	req = chiRouteCtx(req, "id", chID.String())
	req = setAdminCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleAddInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusCreated, w.Body.String())
	}

	// Verify config stored in DB
	var configJSON []byte
	err := a.Pool.QueryRow(context.Background(),
		`SELECT config FROM channel_instances WHERE channel_id = $1`, chID,
	).Scan(&configJSON)
	if err != nil {
		t.Fatalf("query config: %v", err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(configJSON, &cfg); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if cfg["deployment"] != "gpt-4o-deployment" {
		t.Errorf("config deployment = %s, want 'gpt-4o-deployment'", cfg["deployment"])
	}
}

// =============================================================================
// HandleRemoveInstance Tests
// =============================================================================

func TestHandleRemoveInstance_NoAuth(t *testing.T) {
	a := appForChannelsTest(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/some-id/instances/some-id", nil)
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleRemoveInstance_NotAdmin(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-nonadmin@example.com", "pass", "Not Admin")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/some-id/instances/some-id", nil)
	req = setUserCtx(req, user.ID.String())
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleRemoveInstance_ChannelNotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-noch@example.com", "pass", "Admin No Channel")

	chID := uuid.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+chID.String()+"/instances/"+uuid.New().String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": chID.String(), "instanceId": uuid.New().String()})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleRemoveInstance_InstanceNotFound(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-noinst@example.com", "pass", "Admin No Instance")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)
	nonExistentInstID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete,
		"/api/admin/channels/"+chID.String()+"/instances/"+nonExistentInstID.String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": chID.String(), "instanceId": nonExistentInstID.String()})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleRemoveInstance_InvalidChannelID(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-badch@example.com", "pass", "Admin Bad Channel")

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/not-a-uuid/instances/"+uuid.New().String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": "not-a-uuid", "instanceId": uuid.New().String()})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRemoveInstance_InvalidInstanceID(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-badinst@example.com", "pass", "Admin Bad Instance")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID := seedChannelForTest(t, a, "Test Channel", modelID)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+chID.String()+"/instances/not-a-uuid", nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": chID.String(), "instanceId": "not-a-uuid"})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleRemoveInstance_Success(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-ok@example.com", "pass", "Admin Remove Instance")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID, instID := seedChannelWithInstance(t, a, "Test Channel", modelID)

	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+chID.String()+"/instances/"+instID.String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": chID.String(), "instanceId": instID.String()})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp["status"] != "removed" {
		t.Errorf("status = %s, want 'removed'", resp["status"])
	}

	// Verify soft-delete
	var instStatus string
	err := a.Pool.QueryRow(context.Background(),
		`SELECT status FROM channel_instances WHERE id = $1`, instID,
	).Scan(&instStatus)
	if err != nil {
		t.Fatalf("query instance: %v", err)
	}
	if instStatus != "inactive" {
		t.Errorf("instance status = %s, want 'inactive'", instStatus)
	}
}

func TestHandleRemoveInstance_DuplicateRemoval(t *testing.T) {
	a := appForChannelsTest(t)
	user := seedUserForChannelsTest(t, a, "ch-ri-dup@example.com", "pass", "Admin Remove Dup")
	modelID := seedModelForChannelsTest(t, a, "gpt-4o", "openai")

	chID, instID := seedChannelWithInstance(t, a, "Test Channel", modelID)

	// Soft-delete instance first
	_, err := a.Pool.Exec(context.Background(),
		`UPDATE channel_instances SET status = 'inactive' WHERE id = $1`, instID,
	)
	if err != nil {
		t.Fatalf("soft delete instance: %v", err)
	}

	// Try to remove again
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/channels/"+chID.String()+"/instances/"+instID.String(), nil)
	req = setAdminCtx(req, user.ID.String())
	req = chiRouteMultiCtx(req, map[string]string{"id": chID.String(), "instanceId": instID.String()})
	w := httptest.NewRecorder()

	handler := HandleRemoveInstance(a)
	handler.ServeHTTP(w, req)

	// Should still return 200 (idempotent)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", w.Code, http.StatusOK, w.Body.String())
	}
}
