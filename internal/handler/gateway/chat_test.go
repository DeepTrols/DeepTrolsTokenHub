package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/quota"
	"github.com/deeptrols/api/internal/repository/usage"
	"github.com/deeptrols/api/internal/service/billing"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Compile-time interface checks.
var (
	_ gw.Executor             = (*mockExecutor)(nil)
	_ model.Repository        = (*mockModelRepo)(nil)
	_ model.PricingRepository = (*mockPricingRepo)(nil)
)

// ============================================================================
// mockExecutor -- mocks gw.Executor for chat handler tests
// ============================================================================

type mockExecutor struct {
	mu        sync.Mutex
	executeFn func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error)
	// call tracking
	executeCalled int
	lastBody      map[string]any
}

func (m *mockExecutor) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
	m.mu.Lock()
	m.executeCalled++
	m.lastBody = body
	m.mu.Unlock()
	if m.executeFn != nil {
		return m.executeFn(ctx, baseURL, apiKey, upstreamModel, body)
	}
	return nil, fmt.Errorf("mockExecutor: executeFn not set")
}

// ============================================================================
// mockWalletRepo -- mocks wallet.Repository for Charger
// ============================================================================

type mockWalletRepo struct {
	mu           sync.Mutex
	findByUserFn func(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error)
	reserveFn    func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error)
	commitFn     func(ctx context.Context, txID uuid.UUID) error
	settleFn     func(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error
	releaseFn    func(ctx context.Context, txID uuid.UUID) error
	// call tracking
	reserveCalled   int
	commitCalled    int
	settleCalled    int
	releaseCalled   int
	lastReserveAmt  decimal.Decimal
	lastReserveKey  string
	lastCommitTxID  uuid.UUID
	lastSettleTxID  uuid.UUID
	lastSettleAmt   decimal.Decimal
	lastReleaseTxID uuid.UUID
}

func (m *mockWalletRepo) FindByUser(ctx context.Context, userID uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
	if m.findByUserFn != nil {
		return m.findByUserFn(ctx, userID, tenantID)
	}
	return nil, nil
}
func (m *mockWalletRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Wallet, error) {
	return nil, nil
}
func (m *mockWalletRepo) Create(ctx context.Context, wallet *domain.Wallet) error { return nil }
func (m *mockWalletRepo) Reserve(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	m.mu.Lock()
	m.reserveCalled++
	m.lastReserveAmt = amount
	m.lastReserveKey = idempotencyKey
	m.mu.Unlock()
	if m.reserveFn != nil {
		return m.reserveFn(ctx, walletID, amount, idempotencyKey)
	}
	return nil, fmt.Errorf("mockWalletRepo: reserveFn not set")
}
func (m *mockWalletRepo) Commit(ctx context.Context, txID uuid.UUID) error {
	m.mu.Lock()
	m.commitCalled++
	m.lastCommitTxID = txID
	m.mu.Unlock()
	if m.commitFn != nil {
		return m.commitFn(ctx, txID)
	}
	return nil
}
func (m *mockWalletRepo) Settle(ctx context.Context, txID uuid.UUID, finalAmount decimal.Decimal) error {
	m.mu.Lock()
	m.settleCalled++
	m.lastSettleTxID = txID
	m.lastSettleAmt = finalAmount
	m.mu.Unlock()
	if m.settleFn != nil {
		return m.settleFn(ctx, txID, finalAmount)
	}
	return nil
}
func (m *mockWalletRepo) Release(ctx context.Context, txID uuid.UUID) error {
	m.mu.Lock()
	m.releaseCalled++
	m.lastReleaseTxID = txID
	m.mu.Unlock()
	if m.releaseFn != nil {
		return m.releaseFn(ctx, txID)
	}
	return nil
}
func (m *mockWalletRepo) ListTransactions(ctx context.Context, walletID uuid.UUID, limit, offset int) ([]domain.WalletTransaction, error) {
	return nil, nil
}

func (m *mockWalletRepo) TopUp(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
	return nil, nil
}

// ============================================================================
// mockUsageRepo -- mocks usage.Repository for Logger
// ============================================================================

type mockUsageRepo struct {
	mu                       sync.Mutex
	createUsageLogFn         func(ctx context.Context, log *domain.UsageLog) error
	createChargeLinesFn      func(ctx context.Context, lines []domain.ChargeLine) error
	createProviderEvidenceFn func(ctx context.Context, evidence *domain.ProviderEvidence) error
	// captured records
	lastUsageLog    *domain.UsageLog
	lastChargeLines []domain.ChargeLine
	lastEvidence    *domain.ProviderEvidence
}

func (m *mockUsageRepo) CreateUsageLog(ctx context.Context, log *domain.UsageLog) error {
	m.mu.Lock()
	m.lastUsageLog = log
	m.mu.Unlock()
	if m.createUsageLogFn != nil {
		return m.createUsageLogFn(ctx, log)
	}
	return nil
}
func (m *mockUsageRepo) CreateChargeLines(ctx context.Context, lines []domain.ChargeLine) error {
	m.mu.Lock()
	m.lastChargeLines = lines
	m.mu.Unlock()
	if m.createChargeLinesFn != nil {
		return m.createChargeLinesFn(ctx, lines)
	}
	return nil
}
func (m *mockUsageRepo) CreateProviderEvidence(ctx context.Context, evidence *domain.ProviderEvidence) error {
	m.mu.Lock()
	m.lastEvidence = evidence
	m.mu.Unlock()
	if m.createProviderEvidenceFn != nil {
		return m.createProviderEvidenceFn(ctx, evidence)
	}
	return nil
}
func (m *mockUsageRepo) FindByRequestID(ctx context.Context, requestID string) (*domain.UsageLog, error) {
	return nil, nil
}
func (m *mockUsageRepo) ListByUser(ctx context.Context, userID uuid.UUID, filter usage.UsageFilter) ([]domain.UsageLog, int, error) {
	return nil, 0, nil
}
func (m *mockUsageRepo) ListByAPIKey(ctx context.Context, apiKeyID uuid.UUID, filter usage.UsageFilter) ([]domain.UsageLog, int, error) {
	return nil, 0, nil
}

// ============================================================================
// mockPricingRepo -- mocks model.PricingRepository for Pricer
// ============================================================================

type mockPricingRepo struct {
	findByModelFn func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error)
}

func (m *mockPricingRepo) FindByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
	if m.findByModelFn != nil {
		return m.findByModelFn(ctx, modelID, tenantID)
	}
	return nil, nil
}

// ============================================================================
// mockModelRepo -- mocks model.Repository for Router
// ============================================================================

type mockModelRepo struct {
	findByCodeFn func(ctx context.Context, code string) (*domain.Model, error)
}

func (m *mockModelRepo) FindByCode(ctx context.Context, code string) (*domain.Model, error) {
	if m.findByCodeFn != nil {
		return m.findByCodeFn(ctx, code)
	}
	return nil, fmt.Errorf("model not found")
}
func (m *mockModelRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Model, error) {
	return nil, nil
}
func (m *mockModelRepo) ListActive(ctx context.Context) ([]domain.Model, error) { return nil, nil }
func (m *mockModelRepo) ListByTenant(ctx context.Context, tenantID *uuid.UUID) ([]model.TenantModelView, error) {
	return nil, nil
}
func (m *mockModelRepo) GetTenantModel(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error) {
	return nil, nil
}

// ============================================================================
// mockChannelRepo -- mocks channel.Repository for Router
// ============================================================================

type mockChannelRepo struct {
	listByModelFn   func(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error)
	listInstancesFn func(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error)
}

func (m *mockChannelRepo) ListByModel(ctx context.Context, modelID uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
	if m.listByModelFn != nil {
		return m.listByModelFn(ctx, modelID, tenantID)
	}
	return nil, fmt.Errorf("no channels")
}
func (m *mockChannelRepo) FindByID(ctx context.Context, id uuid.UUID) (*domain.Channel, error) {
	return nil, nil
}
func (m *mockChannelRepo) ListInstances(ctx context.Context, channelID uuid.UUID) ([]domain.ChannelInstance, error) {
	if m.listInstancesFn != nil {
		return m.listInstancesFn(ctx, channelID)
	}
	return nil, fmt.Errorf("no instances")
}
func (m *mockChannelRepo) UpdateHealth(ctx context.Context, id uuid.UUID, score int, status domain.HealthStatus) error {
	return nil
}
func (m *mockChannelRepo) UpdateInstanceLoad(ctx context.Context, id uuid.UUID, load int) error {
	return nil
}
func (m *mockChannelRepo) FindRoutePolicy(ctx context.Context, tenantID *uuid.UUID, modelID uuid.UUID, userLevel string) (*domain.RoutePolicy, error) {
	return nil, nil
}

// ============================================================================
// Test helpers
// ============================================================================

// newTestApp builds a mock App with all services wired for testing.
func newTestApp(
	executor gw.Executor,
	modelRepo *mockModelRepo,
	channelRepo *mockChannelRepo,
	pricingRepo *mockPricingRepo,
	walletRepo *mockWalletRepo,
	usageRepo *mockUsageRepo,
) *app.App {
	cfg := &config.Config{
		LiteLLM: config.LiteLLMConfig{
			BaseURL:   "http://localhost:9999",
			MasterKey: "test-master-key",
		},
	}

	return &app.App{
		Config:     cfg,
		Executor:   executor,
		Router:     gw.NewRouter(modelRepo, channelRepo),
		Pricer:     billing.NewPricer(pricingRepo),
		Charger:    billing.NewCharger(walletRepo),
		Logger:     billing.NewLogger(usageRepo),
		Wallets:    walletRepo,
		HttpClient: &http.Client{},
	}
}

// setAuthContext sets the standard auth context values on the request.
func setAuthContext(r *http.Request, userID, apiKeyID uuid.UUID) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.CtxUserID, userID.String())
	ctx = context.WithValue(ctx, middleware.CtxAPIKeyID, apiKeyID.String())
	ctx = context.WithValue(ctx, middleware.CtxTenantID, "")
	ctx = context.WithValue(ctx, middleware.CtxRequestID, "test-request-id")
	return r.WithContext(ctx)
}

// makePricingEntries returns standard pricing entries for token-based billing.
func makePricingEntries() []domain.ModelPricing {
	return []domain.ModelPricing{
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "input",
			UnitName:         "token",
			UnitPrice:        "0.000015",
			UpstreamCost:     "0.000010",
			Currency:         "CNY",
			IsActive:         true,
		},
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "output",
			UnitName:         "token",
			UnitPrice:        "0.000060",
			UpstreamCost:     "0.000040",
			Currency:         "CNY",
			IsActive:         true,
		},
	}
}

// validRequestBody returns a minimal valid chat completion request body.
func validRequestBody() map[string]any {
	return map[string]any{
		"model": "gpt-4o",
		"messages": []any{
			map[string]any{"role": "user", "content": "Hello, world! This is a test message with some tokens to count."},
		},
	}
}

// validResponseBody returns a typical OpenAI chat completion response with usage.
func validResponseBody() map[string]any {
	return map[string]any{
		"id":      "chatcmpl-test123",
		"object":  "chat.completion",
		"created": int64(1700000000),
		"model":   "gpt-4o",
		"choices": []any{
			map[string]any{
				"index": float64(0),
				"message": map[string]any{
					"role":    "assistant",
					"content": "Hello! How can I help you?",
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     float64(20),
			"completion_tokens": float64(15),
			"total_tokens":      float64(35),
		},
	}
}

// ============================================================================
// Test: Non-streaming success -- pricer called, reserve called, commit called, log has non-zero costs
// ============================================================================

func TestHandleNonStreamingChat_Success(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusOK,
				Body:          respBody,
				Usage:         usage,
				UsageSource:   usageparser.SourceUpstream,
				ProviderReqID: "chatcmpl-test123",
				DurationMs:    500,
			}, nil
		},
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{
				ID:     modelID,
				Code:   code,
				Status: domain.ModelStatusActive,
			}, nil
		},
	}

	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{
				ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive,
				HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10,
			}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{
				ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1",
				ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive,
			}}, nil
		},
	}

	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}

	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-request-id")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP response
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Assert -- executor was called
	if executor.executeCalled == 0 {
		t.Error("executor was not called")
	}

	// Assert -- reserve was called
	if walletRepo.reserveCalled == 0 {
		t.Error("reserve was not called")
	}
	if walletRepo.lastReserveAmt.LessThanOrEqual(decimal.Zero) {
		t.Errorf("reserve amount is zero or negative: %s", walletRepo.lastReserveAmt)
	}
	if walletRepo.lastReserveKey != "test-request-id" {
		t.Errorf("reserve idempotency key = %q, want %q", walletRepo.lastReserveKey, "test-request-id")
	}

	// Assert -- settle was called (not release); settle charges the REAL cost
	if walletRepo.settleCalled == 0 {
		t.Error("settle was not called after successful upstream")
	}
	if walletRepo.releaseCalled > 0 {
		t.Error("release was called on successful upstream (should not)")
	}
	if !walletRepo.lastSettleAmt.IsPositive() {
		t.Errorf("settle amount should be positive, got %s", walletRepo.lastSettleAmt)
	}

	// Assert -- log has non-zero costs (wait briefly for goroutine)
	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	if usageRepo.lastUsageLog.ListCost.Equal(decimal.Zero) {
		t.Error("ListCost should be non-zero in usage log")
	}
	if usageRepo.lastUsageLog.FinalCost.Equal(decimal.Zero) {
		t.Error("FinalCost should be non-zero in usage log")
	}
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceUpstream {
		t.Errorf("UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceUpstream)
	}
}

// ============================================================================
// Test: Non-streaming upstream failure -- reserve then release
// ============================================================================

func TestHandleNonStreamingChat_UpstreamFailure_Releases(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			return nil, fmt.Errorf("upstream connection refused")
		},
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-req")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 502 (upstream failure = Bad Gateway with failover exhausted)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}

	// Assert -- reserve was called
	if walletRepo.reserveCalled == 0 {
		t.Error("reserve should have been called before upstream")
	}

	// Assert -- release was called (not commit)
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on upstream failure")
	}
	if walletRepo.commitCalled > 0 {
		t.Error("commit was called on upstream failure (should not)")
	}
}

// ============================================================================
// Test: Non-streaming insufficient balance -- 402 before upstream
// ============================================================================

func TestHandleNonStreamingChat_InsufficientBalance_Returns402(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor should not be called when reserve fails")
			return nil, nil
		},
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.Zero, Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return nil, fmt.Errorf("insufficient balance")
		},
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-req")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 402
	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusPaymentRequired, w.Body.String())
	}

	// Assert -- executor was NOT called (we fail before upstream)
	if executor.executeCalled > 0 {
		t.Error("executor should not have been called when reserve fails")
	}
}

// ============================================================================
// Test: Non-streaming model not routable -- 503
// ============================================================================

func TestHandleNonStreamingChat_ModelNotRoutable_Returns503(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor should not be called when routing fails")
			return nil, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return nil, fmt.Errorf("model not found")
		},
	}
	channelRepo := &mockChannelRepo{}
	pricingRepo := &mockPricingRepo{}
	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-req")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 503
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

// ============================================================================
// Test: Streaming success with usage -- reserve, stream, usage extracted, commit, log
// ============================================================================

func TestHandleStreamingChat_SuccessWithUsage(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	// Create an upstream test server that returns SSE stream with usage in final chunk.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		chunk1 := `{"id":"chatcmpl-001","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`
		chunk2 := `{"id":"chatcmpl-001","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`
		chunk3 := `{"id":"chatcmpl-001","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":15,"total_tokens":35}}`

		fmt.Fprintf(w, "data: %s\n\n", chunk1)
		flusher.Flush()
		fmt.Fprintf(w, "data: %s\n\n", chunk2)
		flusher.Flush()
		fmt.Fprintf(w, "data: %s\n\n", chunk3)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: upstream.URL, ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-stream-req")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- 200 OK with SSE content type
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Assert -- reserve was called
	if walletRepo.reserveCalled == 0 {
		t.Error("reserve should have been called before upstream streaming")
	}

	// Assert -- settle was called (not release)
	if walletRepo.settleCalled == 0 {
		t.Error("settle should have been called after successful stream")
	}
	if walletRepo.releaseCalled > 0 {
		t.Error("release was called on successful stream (should not)")
	}

	// Assert -- response body contains SSE data lines
	respBody := w.Body.String()
	if !strings.Contains(respBody, "data:") {
		t.Error("response should contain SSE data lines")
	}
	if !strings.Contains(respBody, "[DONE]") {
		t.Error("response should contain [DONE] signal")
	}

	// Assert -- log has non-zero costs
	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded for streaming (timed out)")
	}
	if usageRepo.lastUsageLog.ListCost.Equal(decimal.Zero) {
		t.Error("ListCost should be non-zero in streaming usage log")
	}
	if usageRepo.lastUsageLog.FinalCost.Equal(decimal.Zero) {
		t.Error("FinalCost should be non-zero in streaming usage log")
	}
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceUpstream {
		t.Errorf("Streaming UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceUpstream)
	}
}

// ============================================================================
// Test: Streaming no usage fallback -- estimated tag
// ============================================================================

func TestHandleStreamingChat_NoUsageInStream_EstimatedTag(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		chunk1 := `{"id":"chatcmpl-002","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`
		chunk2 := `{"id":"chatcmpl-002","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hi"},"finish_reason":"stop"}]}`

		fmt.Fprintf(w, "data: %s\n\n", chunk1)
		flusher.Flush()
		fmt.Fprintf(w, "data: %s\n\n", chunk2)
		flusher.Flush()
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: upstream.URL, ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-no-usage")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceEstimated {
		t.Errorf("UsageSource = %s, want %s for stream without usage", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceEstimated)
	}
}

// ============================================================================
// Test: Streaming upstream error -- release
// ============================================================================

func TestHandleStreamingChat_UpstreamError_Releases(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "provider error"}}`))
	}))
	defer upstream.Close()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: upstream.URL, ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-stream-err")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Assert
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	if walletRepo.reserveCalled == 0 {
		t.Error("reserve should have been called before upstream")
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on upstream error")
	}
	if walletRepo.commitCalled > 0 {
		t.Error("commit should NOT be called on upstream error")
	}
}

// ============================================================================
// Test: Streaming model not routable -- 503
// ============================================================================

func TestHandleStreamingChat_ModelNotRoutable_Returns503(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return nil, fmt.Errorf("model not found")
		},
	}
	channelRepo := &mockChannelRepo{}
	pricingRepo := &mockPricingRepo{}
	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-stream-route")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Assert
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusServiceUnavailable, w.Body.String())
	}
}

// ============================================================================
// Test: HandleChatCompletions dispatches correctly based on stream flag
// ============================================================================

func TestHandleChatCompletions_NonStream_DispatchesCorrectly(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-test", DurationMs: 300,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := map[string]any{
		"model":    "gpt-4o",
		"messages": []any{map[string]any{"role": "user", "content": "Hello"}},
		"stream":   false,
	}
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-dispatch")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	handler := HandleChatCompletions(application)

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if executor.executeCalled == 0 {
		t.Error("non-streaming executor should have been called")
	}
}

// ============================================================================
// Test: HandleChatCompletions non-POST returns 405
// ============================================================================

func TestHandleChatCompletions_NonPostMethod_Returns405(t *testing.T) {
	application := newTestApp(nil, &mockModelRepo{}, &mockChannelRepo{}, &mockPricingRepo{}, &mockWalletRepo{}, &mockUsageRepo{})

	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	handler := HandleChatCompletions(application)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// ============================================================================
// Test: HandleChatCompletions -- model name required
// ============================================================================

func TestHandleChatCompletions_MissingModel_Returns400(t *testing.T) {
	application := newTestApp(nil, &mockModelRepo{}, &mockChannelRepo{}, &mockPricingRepo{}, &mockWalletRepo{}, &mockUsageRepo{})

	body := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "Hi"}}}
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler := HandleChatCompletions(application)

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

// ============================================================================
// Test: Pricer-based hold amount uses model pricing (not hardcoded)
// ============================================================================

func TestHandleNonStreamingChat_HoldAmount_UsesPricer(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-test", DurationMs: 200,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}

	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-pricer-hold")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	// Verify the hold amount is not zero
	expectedMin, _ := decimal.NewFromString("0.0001")
	if walletRepo.lastReserveAmt.LessThan(expectedMin) {
		t.Errorf("reserve amount %s should be >= minimum %s", walletRepo.lastReserveAmt, expectedMin)
	}
}

// ============================================================================
// Test: Usage source tagged correctly for non-streaming
// ============================================================================

func TestHandleNonStreamingChat_UsageSourceTagged(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	tests := []struct {
		name       string
		execSource usageparser.Source
		expectTag  domain.UsageSource
	}{
		{"upstream source", usageparser.SourceUpstream, domain.UsageSourceUpstream},
		{"estimated source", usageparser.SourceEstimated, domain.UsageSourceEstimated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &mockExecutor{
				executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
					respBody := validResponseBody()
					usage, _ := usageparser.ParseOpenAIUsage(respBody)
					return &gw.ExecuteResponse{
						StatusCode: http.StatusOK, Body: respBody, Usage: usage,
						UsageSource: tt.execSource, ProviderReqID: "chatcmpl-test", DurationMs: 200,
					}, nil
				},
			}
			modelRepo := &mockModelRepo{
				findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
					return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
				},
			}
			channelRepo := &mockChannelRepo{
				listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
					return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
				},
				listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
					return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
				},
			}
			pricingRepo := &mockPricingRepo{
				findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
					return makePricingEntries(), nil
				},
			}
			txID := uuid.New()
			walletRepo := &mockWalletRepo{
				findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
					return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
				},
				reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
					return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
				},
				commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
			}
			usageRepo := &mockUsageRepo{}

			application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

			body := validRequestBody()
			respBodyBytes, _ := json.Marshal(body)
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Request-ID", "test-source-tag")
			req = setAuthContext(req, userID, apiKeyID)
			w := httptest.NewRecorder()

			HandleNonStreamingChat(w, req, application, "gpt-4o", body)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
			}

			deadline := time.Now().Add(200 * time.Millisecond)
			for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
				time.Sleep(5 * time.Millisecond)
			}
			if usageRepo.lastUsageLog == nil {
				t.Fatal("usage log timed out")
			}
			if usageRepo.lastUsageLog.UsageSource != tt.expectTag {
				t.Errorf("UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, tt.expectTag)
			}
		})
	}
}

// ============================================================================
// Test: No wallet should not panic (wallet is optional)
// ============================================================================

func TestHandleNonStreamingChat_NoWallet_DoesNotPanic(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-test", DurationMs: 200,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return nil, nil // no wallet
		},
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-no-wallet")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Should not panic — and must fail closed (no wallet = 402, no upstream call)
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusPaymentRequired, w.Body.String())
	}
	if executor.executeCalled > 0 {
		t.Error("upstream should not be called when the account has no wallet")
	}
}

// ============================================================================
// mockQuotaRepoForChat -- mocks quota.Repository for QuotaChecker chat tests
// ============================================================================

type mockQuotaRepoForChat struct {
	findAllocationFn func(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error)
	consumeFn        func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)
	restoreFn        func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error)

	consumeCalled int
	restoreCalled int
	lastConsumed  int64
	lastRestored  int64
}

func (m *mockQuotaRepoForChat) FindAllocation(ctx context.Context, userID, tenantID, modelID uuid.UUID) (*domain.QuotaAllocation, error) {
	if m.findAllocationFn != nil {
		return m.findAllocationFn(ctx, userID, tenantID, modelID)
	}
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepoForChat) Consume(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	m.consumeCalled++
	m.lastConsumed = amount
	if m.consumeFn != nil {
		return m.consumeFn(ctx, allocationID, amount, idempotencyKey)
	}
	return &domain.QuotaLedgerEntry{
		ID:           uuid.New(),
		AllocationID: allocationID,
		Action:       domain.QuotaActionConsume,
		Amount:       amount,
	}, nil
}

func (m *mockQuotaRepoForChat) Restore(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
	m.restoreCalled++
	m.lastRestored = amount
	if m.restoreFn != nil {
		return m.restoreFn(ctx, allocationID, amount, idempotencyKey)
	}
	return &domain.QuotaLedgerEntry{
		ID:           uuid.New(),
		AllocationID: allocationID,
		Action:       domain.QuotaActionRestore,
		Amount:       amount,
	}, nil
}

// The remaining interface methods are not exercised by gateway tests; stubs
// satisfy quota.Repository as the interface grows.

func (m *mockQuotaRepoForChat) FindPool(ctx context.Context, poolID uuid.UUID) (*domain.QuotaPool, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepoForChat) FindPoolsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaPool, error) {
	return nil, nil
}

func (m *mockQuotaRepoForChat) FindAllocationsByTenant(ctx context.Context, tenantID uuid.UUID) ([]domain.QuotaAllocation, error) {
	return nil, nil
}

func (m *mockQuotaRepoForChat) Allocate(ctx context.Context, poolID, userID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaAllocation, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepoForChat) FindLedgerByAllocation(ctx context.Context, allocationID uuid.UUID, limit, offset int) ([]domain.QuotaLedgerEntry, error) {
	return nil, nil
}

func (m *mockQuotaRepoForChat) UpdatePool(ctx context.Context, poolID uuid.UUID, totalAmount int64, unitName, dimension string) (*domain.QuotaPool, error) {
	return nil, quota.ErrNotFound
}

func (m *mockQuotaRepoForChat) DeletePool(ctx context.Context, poolID uuid.UUID) error {
	return quota.ErrNotFound
}

var _ quota.Repository = (*mockQuotaRepoForChat)(nil)

// ============================================================================
// Test: Non-streaming 429 when quota exceeded
// ============================================================================

func TestHandleNonStreamingChat_QuotaExceeded_Returns429(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor should not be called when quota exceeded")
			return nil, nil
		},
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			t.Error("reserve should not be called when quota exceeded")
			return nil, nil
		},
	}
	usageRepo := &mockUsageRepo{}

	// Quota repo: allocation with very little remaining (3 tokens vs 256+ estimated)
	quotaRepo := &mockQuotaRepoForChat{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 3,
				UsedAmount:      0,
			}, nil
		},
		// New reservation flow: Consume must atomically fail with insufficient quota.
		consumeFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return nil, quota.ErrInsufficientQuota
		},
	}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.QuotaChecker = billing.NewQuotaChecker(quotaRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-quota-429")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 429
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}

	// Executor should not have been called
	if executor.executeCalled > 0 {
		t.Error("executor should not have been called when quota exceeded")
	}
}

// ============================================================================
// Test: Non-streaming normal flow when quota sufficient
// ============================================================================

func TestHandleNonStreamingChat_QuotaSufficient_ProceedsNormally(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-test", DurationMs: 200,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	// Quota repo: allocation with plenty of remaining
	quotaRepo := &mockQuotaRepoForChat{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 1000000,
				UsedAmount:      0,
			}, nil
		},
	}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.QuotaChecker = billing.NewQuotaChecker(quotaRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-quota-suff")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 200, normal flow
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}

	// Quota consume should have been called (best-effort in background goroutine)
	deadline := time.Now().Add(200 * time.Millisecond)
	for quotaRepo.consumeCalled == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if quotaRepo.consumeCalled == 0 {
		t.Error("quota consume was not called after successful upstream")
	}
}

// ============================================================================
// Test: Non-streaming normal flow when no quota configured (QuotaChecker = nil)
// ============================================================================

func TestHandleNonStreamingChat_NoQuotaChecker_ProceedsNormally(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-test", DurationMs: 200,
			}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	// QuotaChecker is nil (no quota configured)
	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.QuotaChecker = nil

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-no-quota-ck")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act - should NOT panic when QuotaChecker is nil
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 200, normal flow
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
}

// ============================================================================
// Test: Streaming 429 when quota exceeded
// ============================================================================

func TestHandleStreamingChat_QuotaExceeded_Returns429(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{{ID: channelID, ModelID: modelID, Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 1, MaxConcurrency: 10}}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			t.Error("reserve should not be called when streaming quota exceeded")
			return nil, nil
		},
	}
	usageRepo := &mockUsageRepo{}

	// Quota repo: allocation with nearly exhausted quota
	quotaRepo := &mockQuotaRepoForChat{
		findAllocationFn: func(ctx context.Context, uid, tid, mid uuid.UUID) (*domain.QuotaAllocation, error) {
			return &domain.QuotaAllocation{
				ID:              uuid.New(),
				UserID:          uid,
				AllocatedAmount: 5,
				UsedAmount:      0,
			}, nil
		},
		// New reservation flow: atomic Consume fails with insufficient quota.
		consumeFn: func(ctx context.Context, allocationID uuid.UUID, amount int64, idempotencyKey string) (*domain.QuotaLedgerEntry, error) {
			return nil, quota.ErrInsufficientQuota
		},
	}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	application.QuotaChecker = billing.NewQuotaChecker(quotaRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-stream-quota-429")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 429
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusTooManyRequests, w.Body.String())
	}
}

func TestHandleNonStreamingChat_FailoverToSecondChannel(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID1 := uuid.New()
	channelID2 := uuid.New()
	instanceID1 := uuid.New()
	instanceID2 := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			if strings.Contains(baseURL, "9999") {
				return nil, fmt.Errorf("first channel down")
			}
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-failover", DurationMs: 150}, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			return []domain.Channel{
				{ID: channelID1, ModelID: modelID, Name: "ch1", Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 100, MaxConcurrency: 10},
				{ID: channelID2, ModelID: modelID, Name: "ch2", Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 90, MaxConcurrency: 10},
			}, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			if cid == channelID1 {
				return []domain.ChannelInstance{{ID: instanceID1, ChannelID: channelID1, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
			}
			return []domain.ChannelInstance{{ID: instanceID2, ChannelID: channelID2, BaseURL: "http://localhost:8888/v1", ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100), Frozen: decimal.Zero}, nil
		},
		reserveFn: func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
			return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
		},
		releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-failover")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after failover; body = %s", w.Code, w.Body.String())
	}
	// two attempts → two reserves, one release for the failed first attempt
	if walletRepo.reserveCalled != 2 {
		t.Errorf("reserveCalled = %d, want 2 (one per attempt)", walletRepo.reserveCalled)
	}
	if walletRepo.releaseCalled != 1 {
		t.Errorf("releaseCalled = %d, want 1 (failed first attempt)", walletRepo.releaseCalled)
	}
	if walletRepo.settleCalled != 1 {
		t.Errorf("settleCalled = %d, want 1 (successful second attempt)", walletRepo.settleCalled)
	}
	if executor.executeCalled != 2 {
		t.Errorf("executeCalled = %d, want 2", executor.executeCalled)
	}
}
