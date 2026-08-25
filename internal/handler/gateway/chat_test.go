package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/config"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/guardrails"
	"github.com/deeptrols/api/internal/handler/middleware"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/repository/budget"
	"github.com/deeptrols/api/internal/repository/model"
	"github.com/deeptrols/api/internal/repository/testutil"
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
	mu                sync.Mutex
	executeFn         func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error)
	executeEndpointFn func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error)
	executeRawFn      func(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.RawResponse, error)
	// call tracking
	executeCalled         int
	executeEndpointCalled int
	executeRawCalled      int
	lastBody              map[string]any
	lastEndpoint          string
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

// ExecuteEndpoint is the generic endpoint counterpart of Execute. It prefers
// the endpoint-specific mock fn and falls back to the chat executeFn so
// existing tests keep working unchanged.
func (m *mockExecutor) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
	m.mu.Lock()
	m.executeEndpointCalled++
	m.lastEndpoint = endpoint
	m.lastBody = body
	m.mu.Unlock()
	if m.executeEndpointFn != nil {
		return m.executeEndpointFn(ctx, baseURL, apiKey, upstreamModel, endpoint, body)
	}
	if m.executeFn != nil {
		return m.executeFn(ctx, baseURL, apiKey, upstreamModel, body)
	}
	return nil, fmt.Errorf("mockExecutor: executeEndpointFn not set")
}

// ExecuteEndpointRaw is the raw-response counterpart used by the audio
// endpoint tests. It prefers the endpoint-specific mock fn and fails loudly
// when a test forgets to stub it.
func (m *mockExecutor) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.RawResponse, error) {
	m.mu.Lock()
	m.executeRawCalled++
	m.lastEndpoint = endpoint
	m.lastBody = body
	m.mu.Unlock()
	if m.executeRawFn != nil {
		return m.executeRawFn(ctx, baseURL, apiKey, upstreamModel, endpoint, body)
	}
	return nil, fmt.Errorf("mockExecutor: executeRawFn not set")
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
func (m *mockWalletRepo) Transfer(ctx context.Context, fromWalletID, toWalletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
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
	findByCodeFn  func(ctx context.Context, code string) (*domain.Model, error)
	tenantModelFn func(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error)
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
	if m.tenantModelFn != nil {
		return m.tenantModelFn(ctx, tenantID, modelCode)
	}
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
func (m *mockChannelRepo) EnterCooldown(ctx context.Context, id uuid.UUID, until time.Time) error {
	return nil
}
func (m *mockChannelRepo) ClearCooldown(ctx context.Context, id uuid.UUID) error {
	return nil
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

// makePricingEntries returns standard pricing entries for token-based billing
// (unit price per 1M tokens).
func makePricingEntries() []domain.ModelPricing {
	return []domain.ModelPricing{
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "input",
			UnitName:         "1M tokens",
			UnitPrice:        "0.000015",
			UpstreamCost:     "0.000010",
			Currency:         "CNY",
			IsActive:         true,
		},
		{
			ID:               uuid.New(),
			ModelID:          uuid.Nil,
			PricingDimension: "output",
			UnitName:         "1M tokens",
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
	if usageRepo.lastUsageLog.WalletCharged.IsZero() {
		t.Error("WalletCharged should be non-zero after a successful settle")
	}
	if !usageRepo.lastUsageLog.WalletCharged.Equal(usageRepo.lastUsageLog.FinalCost) {
		t.Errorf("WalletCharged = %s, want FinalCost %s", usageRepo.lastUsageLog.WalletCharged, usageRepo.lastUsageLog.FinalCost)
	}
}

// ============================================================================
// Test: settle underfunded -- evidence must record the shortfall, never a
// silent commit of the reserved amount.
// ============================================================================

func TestHandleNonStreamingChat_SettleUnderfunded_MarksEvidence(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := map[string]any{
				"id":      "chatcmpl-underfunded",
				"object":  "chat.completion",
				"created": int64(1700000000),
				"model":   "gpt-4o",
				"choices": []any{
					map[string]any{
						"index":         float64(0),
						"message":       map[string]any{"role": "assistant", "content": "long"},
						"finish_reason": "stop",
					},
				},
				// 100k output tokens: far beyond the estimate-based hold.
				"usage": map[string]any{
					"prompt_tokens":     float64(20),
					"completion_tokens": float64(100000),
					"total_tokens":      float64(100020),
				},
			}
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusOK,
				Body:          respBody,
				Usage:         usage,
				UsageSource:   usageparser.SourceUpstream,
				ProviderReqID: "chatcmpl-underfunded",
				DurationMs:    500,
			}, nil
		},
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
		tenantModelFn: func(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error) {
			return &domain.TenantModel{ModelID: modelID, IsListed: true, AllowPayg: true}, nil
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
			// Output price is high enough that the 100k-token final cost
			// exceeds the minimum hold, exercising the underfunded path.
			return []domain.ModelPricing{
				{ID: uuid.New(), ModelID: mid, PricingDimension: "input", UnitName: "token",
					UnitPrice: "0.000015", UpstreamCost: "0.000010", Currency: "CNY", IsActive: true},
				{ID: uuid.New(), ModelID: mid, PricingDimension: "output", UnitName: "token",
					UnitPrice: "0.0002", UpstreamCost: "0.0001", Currency: "CNY", IsActive: true},
			}, nil
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
		settleFn: func(ctx context.Context, tID uuid.UUID, amount decimal.Decimal) error {
			// Simulate "wallet cannot cover final cost larger than reserve".
			return fmt.Errorf("insufficient funds to cover final cost %s", amount)
		},
		commitFn: func(ctx context.Context, tID uuid.UUID) error { return nil },
	}
	usageRepo := &mockUsageRepo{}
	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if walletRepo.settleCalled == 0 {
		t.Error("settle should have been attempted")
	}
	if walletRepo.commitCalled == 0 {
		t.Error("commit fallback should have been called after settle failure")
	}
	if walletRepo.releaseCalled > 0 {
		t.Error("release should NOT be called when the request succeeded")
	}

	log := waitForUsageLog(t, usageRepo)
	if log.ErrorCode != "undercharged" {
		t.Errorf("ErrorCode = %q, want %q", log.ErrorCode, "undercharged")
	}
	if log.ErrorMessage == "" {
		t.Error("ErrorMessage should describe the shortfall")
	}
	if log.WalletCharged.IsZero() {
		t.Error("WalletCharged should equal the committed reserved amount")
	}
	if !log.WalletCharged.LessThan(log.FinalCost) {
		t.Errorf("WalletCharged %s should be less than FinalCost %s", log.WalletCharged, log.FinalCost)
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
// Test: Non-streaming model not found -- 404
// ============================================================================

func TestHandleNonStreamingChat_ModelNotFound_Returns404(t *testing.T) {
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

	// Assert -- HTTP 404
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
	}
}

func TestHandleNonStreamingChat_ModelNotActive_Returns400(t *testing.T) {
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
			return &domain.Model{ID: uuid.New(), Code: code, Status: domain.ModelStatusInactive}, nil
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
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 400
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
}

func TestHandleNonStreamingChat_TenantNotAllowed_Returns403(t *testing.T) {
	// Arrange
	userID := uuid.New()
	apiKeyID := uuid.New()
	tenantID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor should not be called when routing fails")
			return nil, nil
		},
	}
	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: uuid.New(), Code: code, Status: domain.ModelStatusActive}, nil
		},
		tenantModelFn: func(ctx context.Context, tid uuid.UUID, modelCode string) (*domain.TenantModel, error) {
			return &domain.TenantModel{IsListed: false}, nil
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
	req = setAuthContext(req, userID, apiKeyID)
	// A non-empty tenant is required to reach the tenant_models gate.
	ctx := context.WithValue(req.Context(), middleware.CtxTenantID, tenantID.String())
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Act
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// Assert -- HTTP 403
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusForbidden, w.Body.String())
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
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceFinalChunk {
		t.Errorf("Streaming UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceFinalChunk)
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Streaming Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusCompleted)
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded for streaming")
	}
	if usageRepo.lastEvidence.StatusCode != http.StatusOK {
		t.Errorf("evidence status = %d, want %d", usageRepo.lastEvidence.StatusCode, http.StatusOK)
	}
}

// ============================================================================
// Test: Streaming truncated stream -- partial usage log + release (no [DONE])
// ============================================================================

func TestHandleStreamingChat_TruncatedStream_LogsPartialAndReleases(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	// Upstream hijacks the connection, forwards one chunk, then resets the TCP
	// connection so the client sees a read error instead of a clean EOF.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("hijack not supported")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			return
		}
		buf.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\n\r\n")
		chunk := `{"id":"chatcmpl-trunc","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`
		fmt.Fprintf(buf, "data: %s\n\n", chunk)
		buf.Flush()
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.SetLinger(0) // RST instead of FIN
		}
		conn.Close()
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
	req.Header.Set("X-Request-ID", "test-stream-trunc")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	// Partial stream was forwarded, but no [DONE] may be sent.
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "data:") {
		t.Error("response should contain the forwarded SSE chunk")
	}
	if strings.Contains(respBody, "[DONE]") {
		t.Error("truncated stream must NOT send [DONE] (invariant #5)")
	}

	if walletRepo.reserveCalled == 0 {
		t.Error("reserve should have been called before upstream")
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on truncated stream")
	}
	if walletRepo.settleCalled > 0 {
		t.Error("settle should NOT be called on truncated stream")
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded for truncated stream (timed out)")
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusPartial {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusPartial)
	}
	if usageRepo.lastUsageLog.ErrorCode != "stream_interrupted" {
		t.Errorf("ErrorCode = %s, want stream_interrupted", usageRepo.lastUsageLog.ErrorCode)
	}
	if !usageRepo.lastUsageLog.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0", usageRepo.lastUsageLog.WalletCharged)
	}
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceEstimated {
		t.Errorf("UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceEstimated)
	}
}

// ============================================================================
// Test: Streaming upstream HTTP error -- failed usage log with evidence
// ============================================================================

func TestHandleStreamingChat_UpstreamHTTPError_LogsFailed(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": {"message": "provider exploded"}}`))
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
	req.Header.Set("X-Request-ID", "test-stream-http-err")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on upstream HTTP error")
	}
	if walletRepo.settleCalled > 0 {
		t.Error("settle should NOT be called on upstream HTTP error")
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded for upstream HTTP error (timed out)")
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusFailed)
	}
	if usageRepo.lastUsageLog.ErrorCode != "upstream_http_error" {
		t.Errorf("ErrorCode = %s, want upstream_http_error", usageRepo.lastUsageLog.ErrorCode)
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if usageRepo.lastEvidence.StatusCode != http.StatusInternalServerError {
		t.Errorf("evidence status = %d, want %d", usageRepo.lastEvidence.StatusCode, http.StatusInternalServerError)
	}
	if usageRepo.lastEvidence.ErrorMessage == "" {
		t.Error("evidence error_message should be populated on upstream HTTP error")
	}
	if !usageRepo.lastUsageLog.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0", usageRepo.lastUsageLog.WalletCharged)
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
// Test: Streaming upstream connection error -- failed usage log
// ============================================================================

func TestHandleStreamingChat_UpstreamConnectionError_LogsFailed(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	// Grab a port with nothing listening: connection refused on client.Do.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	deadURL := "http://" + ln.Addr().String()
	ln.Close()

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
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: deadURL, ProviderRoute: "gpt-4o", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
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

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-stream-conn-err")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on upstream connection error")
	}
	if walletRepo.settleCalled > 0 {
		t.Error("settle should NOT be called on upstream connection error")
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded for upstream connection error (timed out)")
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusFailed)
	}
	if usageRepo.lastUsageLog.ErrorCode != "upstream_error" {
		t.Errorf("ErrorCode = %s, want upstream_error", usageRepo.lastUsageLog.ErrorCode)
	}
	if !usageRepo.lastUsageLog.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0", usageRepo.lastUsageLog.WalletCharged)
	}
}

// ============================================================================
// Test: Streaming upstream HTTP error body is capped at 1 MB
// ============================================================================

func TestHandleStreamingChat_UpstreamHTTPError_BodyTruncated(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write(bytes.Repeat([]byte("x"), 2*1024*1024))
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
	req.Header.Set("X-Request-ID", "test-stream-big-err")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if w.Body.Len() > 1<<20 {
		t.Errorf("response body length = %d, want <= 1 MiB (capped)", w.Body.Len())
	}

	deadline := time.Now().Add(300 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded for large upstream error (timed out)")
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusFailed)
	}
}

// ============================================================================
// Test: Streaming model not found -- 404
// ============================================================================

func TestHandleStreamingChat_ModelNotFound_Returns404(t *testing.T) {
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

	// Assert -- HTTP 404
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusNotFound, w.Body.String())
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

type fakeGuardrailPolicySource struct {
	policies []guardrails.Policy
}

func (f *fakeGuardrailPolicySource) LoadPolicies(context.Context) ([]guardrails.Policy, error) {
	return f.policies, nil
}

// TestHandleChatCompletions_GuardrailBlocked verifies the Phase 1 outbound
// guardrail wiring: a blocking keyword yields 400 guardrail_blocked before any
// routing or upstream execution.
func TestHandleChatCompletions_GuardrailBlocked(t *testing.T) {
	pool := testutil.SetupPool(t)
	testutil.TruncateAll(t, pool)

	policy, err := guardrails.NormalizePolicy(guardrails.Policy{
		ID:            "p-block",
		Name:          "敏感拦截",
		Status:        guardrails.StatusActive,
		ConfigVersion: guardrails.CurrentConfigVersion,
		DetectionItems: []guardrails.DetectionItem{
			{
				ID: "i1", PolicyID: "p-block", Name: "keyword", DetectorType: guardrails.DetectorPattern,
				Action: guardrails.ActionBlock, ConfigVersion: guardrails.CurrentConfigVersion,
				Config: map[string]any{"keywords": []any{"机密"}},
			},
		},
		Bindings: []guardrails.Binding{
			{
				ID: "b1", PolicyID: "p-block", ScopeType: guardrails.ScopeAllProjects,
				Checkpoint: guardrails.CheckpointBeforeProvider, Protocol: guardrails.ProtocolAll,
				ConfigVersion: guardrails.CurrentConfigVersion,
			},
		},
	})
	if err != nil {
		t.Fatalf("normalize policy: %v", err)
	}

	application := &app.App{
		Pool:               pool,
		Config:             &config.Config{},
		Guardrails:         guardrails.NewEngine(nil),
		GuardrailsPolicies: &fakeGuardrailPolicySource{policies: []guardrails.Policy{policy}},
	}

	body := bytes.NewBufferString(`{"model":"deepseek-chat","messages":[{"role":"user","content":"这是机密内容"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req = setAuthContext(req, uuid.New(), uuid.New())
	w := httptest.NewRecorder()

	HandleChatCompletions(application).ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "guardrail_blocked") {
		t.Errorf("body = %s, want guardrail_blocked", w.Body.String())
	}

	var auditCount int
	if err := pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM audit_logs WHERE action = 'guardrail_blocked'`).Scan(&auditCount); err != nil {
		t.Fatalf("audit query: %v", err)
	}
	if auditCount != 1 {
		t.Errorf("guardrail_blocked audits = %d, want 1", auditCount)
	}
}

func TestChatFragments_ExtractsText(t *testing.T) {
	fragments := chatFragments(map[string]any{
		"messages": []any{
			map[string]any{"role": "user", "content": "hello"},
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "text", "text": "part one"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "x"}},
			}},
			map[string]any{"role": "user", "content": ""},
		},
	})
	if len(fragments) != 2 {
		t.Fatalf("fragments = %d, want 2", len(fragments))
	}
	if fragments[0].Text != "hello" || fragments[1].Text != "part one" {
		t.Errorf("fragments = %+v", fragments)
	}
}

type fakeChatBudgetRepo struct {
	budget *domain.Budget
}

func (f *fakeChatBudgetRepo) FindMonthly(context.Context, uuid.UUID) (*domain.Budget, error) {
	if f.budget == nil {
		return nil, budget.ErrNotFound
	}
	return f.budget, nil
}

func (f *fakeChatBudgetRepo) AccrueSpend(context.Context, uuid.UUID, decimal.Decimal) error {
	return nil
}

// TestHandleChatCompletions_BudgetExceeded verifies the tenant budget gate
// returns 429 before any wallet/upstream work.
func TestHandleChatCompletions_BudgetExceeded(t *testing.T) {
	modelID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	tenantID := uuid.New()

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
		tenantModelFn: func(ctx context.Context, tenantID uuid.UUID, modelCode string) (*domain.TenantModel, error) {
			return &domain.TenantModel{ModelID: modelID, IsListed: true, AllowPayg: true}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, _ *uuid.UUID) ([]domain.Channel, error) {
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
		findByModelFn: func(ctx context.Context, mid uuid.UUID, _ *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, &mockWalletRepo{}, &mockUsageRepo{})
	limit, _ := decimal.NewFromString("0.00000000000001")
	application.BudgetChecker = billing.NewBudgetChecker(&fakeChatBudgetRepo{
		budget: &domain.Budget{
			TenantID: tenantID, LimitAmount: limit, SpentAmount: decimal.Zero,
			Status: domain.BudgetStatusActive,
		},
	})

	body := bytes.NewBufferString(`{"model":"gpt-4o","messages":[{"role":"user","content":"hello"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req = setAuthContext(req, uuid.New(), uuid.New())
	req = req.WithContext(context.WithValue(req.Context(), middleware.CtxTenantID, tenantID.String()))
	w := httptest.NewRecorder()

	HandleChatCompletions(application).ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429; body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "budget_exceeded") {
		t.Errorf("body = %s, want budget_exceeded", w.Body.String())
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

	// Verify the hold amount is derived from pricing (prices are per 1M tokens,
	// so a small prompt yields a small but positive hold).
	expectedMin, _ := decimal.NewFromString("0.000000001")
	if walletRepo.lastReserveAmt.LessThan(expectedMin) {
		t.Errorf("reserve amount %s should be >= minimum %s", walletRepo.lastReserveAmt, expectedMin)
	}
}

func TestHandleNonStreamingChat_PricingIncomplete_RejectsBeforeUpstream(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor should not be called when pricing is incomplete")
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
	// Only output pricing exists: the estimate also needs input -> 422.
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return []domain.ModelPricing{{
				ID: uuid.New(), ModelID: mid, PricingDimension: "output", UnitName: "1M tokens",
				UnitPrice: "0.03", UpstreamCost: "0.015", Currency: "CNY", IsActive: true,
			}}, nil
		},
	}
	walletRepo := &mockWalletRepo{
		findByUserFn: func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
			return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
		},
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()

	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusUnprocessableEntity, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "pricing_incomplete") {
		t.Errorf("body = %s, want pricing_incomplete error", w.Body.String())
	}
	if executor.executeCalled != 0 {
		t.Error("executor should not be called")
	}
	if walletRepo.reserveCalled != 0 {
		t.Error("reserve should not be called")
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

// ============================================================================
// Non-streaming failure evidence (invariant #4/#5): every failed upstream
// attempt must leave a usage_log (failed, zero cost) plus provider evidence,
// and the wallet hold must always be released -- even on client disconnect.
// ============================================================================

// newNonStreamFailureEnv builds the standard mock app for non-streaming
// failure tests. When multiChannel is true the router exposes two candidates
// (baseURL 9999 then 8888) so failover paths can be exercised.
func newNonStreamFailureEnv(executor gw.Executor, multiChannel bool) (*app.App, *mockWalletRepo, *mockUsageRepo) {
	return newNonStreamEnv(executor, multiChannel, "")
}

// newNonStreamEnv builds a non-streaming chat test environment. instanceProvider
// is written into the channel instance config's "provider" key (empty = no
// provider in config, exercising the "direct" fallback).
func newNonStreamEnv(executor gw.Executor, multiChannel bool, instanceProvider string) (*app.App, *mockWalletRepo, *mockUsageRepo) {
	channelID1 := uuid.New()
	channelID2 := uuid.New()
	instanceID1 := uuid.New()
	instanceID2 := uuid.New()
	modelID := uuid.New()

	instanceConfig := map[string]any{}
	if instanceProvider != "" {
		instanceConfig = map[string]any{"provider": instanceProvider}
	}

	modelRepo := &mockModelRepo{
		findByCodeFn: func(ctx context.Context, code string) (*domain.Model, error) {
			return &domain.Model{ID: modelID, Code: code, Status: domain.ModelStatusActive}, nil
		},
	}
	channelRepo := &mockChannelRepo{
		listByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.Channel, error) {
			channels := []domain.Channel{
				{ID: channelID1, ModelID: modelID, Name: "ch1", Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 100, MaxConcurrency: 10},
			}
			if multiChannel {
				channels = append(channels, domain.Channel{ID: channelID2, ModelID: modelID, Name: "ch2", Status: domain.ChannelStatusActive, HealthScore: 100, HealthStatus: domain.HealthStatusHealthy, Weight: 90, MaxConcurrency: 10})
			}
			return channels, nil
		},
		listInstancesFn: func(ctx context.Context, cid uuid.UUID) ([]domain.ChannelInstance, error) {
			if multiChannel && cid == channelID2 {
				return []domain.ChannelInstance{{ID: instanceID2, ChannelID: channelID2, BaseURL: "http://localhost:8888/v1", ProviderRoute: "gpt-4o", Config: instanceConfig, Status: domain.InstanceStatusActive}}, nil
			}
			return []domain.ChannelInstance{{ID: instanceID1, ChannelID: channelID1, BaseURL: "http://localhost:9999/v1", ProviderRoute: "gpt-4o", Config: instanceConfig, Status: domain.InstanceStatusActive}}, nil
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
	return application, walletRepo, usageRepo
}

// newNonStreamChatRequest builds a valid non-streaming chat request with auth
// context and a unique request ID.
func newNonStreamChatRequest(userID, apiKeyID uuid.UUID, body map[string]any) *http.Request {
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-ns-"+uuid.New().String())
	return setAuthContext(req, userID, apiKeyID)
}

// waitForUsageLog polls the mock usage repo until the async logging goroutine
// has recorded a usage log (or fails the test after a deadline).
func waitForUsageLog(t *testing.T, usageRepo *mockUsageRepo) *domain.UsageLog {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	return usageRepo.lastUsageLog
}

func TestHandleNonStreamingChat_UpstreamHTTPError_LogsFailed(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusInternalServerError,
				Body:          map[string]any{"error": map[string]any{"message": "provider exploded"}},
				ProviderReqID: "upstream-req-1",
				DurationMs:    42,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newNonStreamFailureEnv(executor, false)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusBadGateway, w.Body.String())
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on upstream HTTP error")
	}
	if walletRepo.settleCalled > 0 {
		t.Error("settle should NOT be called on upstream HTTP error")
	}

	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", log.Status, domain.UsageLogStatusFailed)
	}
	if log.ErrorCode != "upstream_http_error" {
		t.Errorf("ErrorCode = %s, want upstream_http_error", log.ErrorCode)
	}
	if log.ProviderRequestID != "upstream-req-1" {
		t.Errorf("ProviderRequestID = %s, want the upstream x-request-id", log.ProviderRequestID)
	}
	if log.UsageSource != domain.UsageSourceEstimated {
		t.Errorf("UsageSource = %s, want %s", log.UsageSource, domain.UsageSourceEstimated)
	}
	if !log.WalletCharged.IsZero() {
		t.Errorf("WalletCharged = %s, want 0", log.WalletCharged)
	}
	if !log.FinalCost.IsZero() {
		t.Errorf("FinalCost = %s, want 0", log.FinalCost)
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if usageRepo.lastEvidence.StatusCode != http.StatusInternalServerError {
		t.Errorf("evidence status = %d, want %d", usageRepo.lastEvidence.StatusCode, http.StatusInternalServerError)
	}
	if usageRepo.lastEvidence.ErrorMessage == "" {
		t.Error("evidence error_message should be populated on upstream HTTP error")
	}
}

func TestHandleNonStreamingChat_UpstreamConnectionError_LogsFailed(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			return nil, fmt.Errorf("upstream connection refused")
		},
	}
	application, walletRepo, usageRepo := newNonStreamFailureEnv(executor, false)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if walletRepo.releaseCalled == 0 {
		t.Error("release should have been called on connection error")
	}

	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", log.Status, domain.UsageLogStatusFailed)
	}
	if log.ErrorCode != "upstream_error" {
		t.Errorf("ErrorCode = %s, want upstream_error", log.ErrorCode)
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if usageRepo.lastEvidence.ErrorMessage == "" {
		t.Error("evidence error_message should carry the connection error")
	}
}

func TestHandleNonStreamingChat_FailoverAllFailed_LogsSingleFailure(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			if strings.Contains(baseURL, "9999") {
				return &gw.ExecuteResponse{StatusCode: http.StatusBadGateway, Body: map[string]any{"error": "first down"}}, nil
			}
			return nil, fmt.Errorf("second channel down")
		},
	}
	application, walletRepo, usageRepo := newNonStreamFailureEnv(executor, true)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if executor.executeCalled != 2 {
		t.Errorf("executeCalled = %d, want 2 (both candidates tried)", executor.executeCalled)
	}
	if walletRepo.releaseCalled != 2 {
		t.Errorf("releaseCalled = %d, want 2 (one per failed attempt)", walletRepo.releaseCalled)
	}

	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s", log.Status, domain.UsageLogStatusFailed)
	}
	if !strings.Contains(log.ErrorMessage, "all 2 candidates failed") {
		t.Errorf("ErrorMessage = %q, want it to mention 'all 2 candidates failed'", log.ErrorMessage)
	}
	// Evidence reflects the LAST attempt (connection error -> no status code).
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if usageRepo.lastEvidence.StatusCode != 0 {
		t.Errorf("evidence status = %d, want 0 (last attempt was a connection error)", usageRepo.lastEvidence.StatusCode)
	}
}

func TestHandleNonStreamingChat_ClientCancel_LogsFailedDetached(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			return nil, ctx.Err()
		},
	}
	application, walletRepo, usageRepo := newNonStreamFailureEnv(executor, false)

	var releaseWithCancelledCtx bool
	walletRepo.releaseFn = func(ctx context.Context, tID uuid.UUID) error {
		if ctx.Err() != nil {
			releaseWithCancelledCtx = true
		}
		return nil
	}

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if walletRepo.releaseCalled == 0 {
		t.Error("release should still be called when the client disconnected")
	}
	if releaseWithCancelledCtx {
		t.Error("release must use a detached context when the request context is cancelled")
	}
	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusFailed {
		t.Errorf("Status = %s, want %s (evidence must survive disconnect)", log.Status, domain.UsageLogStatusFailed)
	}
	if log.ErrorCode != "client_disconnected" {
		t.Errorf("ErrorCode = %s, want client_disconnected", log.ErrorCode)
	}
}

func TestHandleNonStreamingChat_ErrorBodyTruncated(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			return &gw.ExecuteResponse{
				StatusCode: http.StatusInternalServerError,
				Body:       map[string]any{"error": map[string]any{"message": strings.Repeat("x", 2<<20)}},
				DurationMs: 10,
			}, nil
		},
	}
	application, _, usageRepo := newNonStreamFailureEnv(executor, false)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusFailed {
		t.Fatalf("Status = %s, want %s", log.Status, domain.UsageLogStatusFailed)
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	raw, err := json.Marshal(usageRepo.lastEvidence.ResponseBody)
	if err != nil {
		t.Fatalf("marshal evidence response body: %v", err)
	}
	if len(raw) > maxUpstreamErrorBody {
		t.Errorf("evidence response body = %d bytes, want <= %d", len(raw), maxUpstreamErrorBody)
	}
}

func TestHandleNonStreamingChat_Success_NoFailureLog(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-ok", DurationMs: 12}, nil
		},
	}
	application, _, usageRepo := newNonStreamFailureEnv(executor, false)

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	log := waitForUsageLog(t, usageRepo)
	if log.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Status = %s, want %s (success must not be recorded as failed)", log.Status, domain.UsageLogStatusCompleted)
	}
}

// ============================================================================
// Provider attribution in evidence (LiteLLM removal)
// ============================================================================

func TestUpstreamProvider_FromInstanceConfig(t *testing.T) {
	got := upstreamProvider(&gw.RouteResult{
		Instance: &domain.ChannelInstance{Config: map[string]any{"provider": "deepseek"}},
	})
	if got != "deepseek" {
		t.Errorf("upstreamProvider = %q, want %q", got, "deepseek")
	}
}

func TestUpstreamProvider_FallbackDirect(t *testing.T) {
	got := upstreamProvider(&gw.RouteResult{
		Instance: &domain.ChannelInstance{Config: map[string]any{}},
	})
	if got != "direct" {
		t.Errorf("upstreamProvider = %q, want %q", got, "direct")
	}
}

func TestUpstreamProvider_NilRoute(t *testing.T) {
	if got := upstreamProvider(nil); got != "unknown" {
		t.Errorf("upstreamProvider(nil) = %q, want %q", got, "unknown")
	}
}

func TestHandleNonStreamingChat_EvidenceProviderFromInstanceConfig(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-provider-1", DurationMs: 12}, nil
		},
	}
	application, _, usageRepo := newNonStreamEnv(executor, false, "deepseek")

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	waitForUsageLog(t, usageRepo)
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if got := usageRepo.lastEvidence.Provider; got != "deepseek" {
		t.Errorf("evidence provider = %q, want %q (must not be litellm)", got, "deepseek")
	}
}

func TestHandleNonStreamingChat_EvidenceProviderFallbackDirect(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			respBody := validResponseBody()
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{StatusCode: http.StatusOK, Body: respBody, Usage: usage,
				UsageSource: usageparser.SourceUpstream, ProviderReqID: "chatcmpl-direct-1", DurationMs: 12}, nil
		},
	}
	application, _, usageRepo := newNonStreamEnv(executor, false, "")

	req := newNonStreamChatRequest(userID, apiKeyID, validRequestBody())
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", validRequestBody())

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	waitForUsageLog(t, usageRepo)
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
	if got := usageRepo.lastEvidence.Provider; got != "direct" {
		t.Errorf("evidence provider = %q, want %q", got, "direct")
	}
}

// ============================================================================
// RED: an upstream success WITHOUT usage must never settle at zero cost.
// ============================================================================

func TestHandleNonStreamingChat_NoUsage_ChargesEstimate(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			// Upstream returned a successful response but no usage object.
			respBody := map[string]any{
				"id":      "chatcmpl-nousage",
				"object":  "chat.completion",
				"model":   "gpt-4o",
				"choices": []any{},
			}
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusOK,
				Body:          respBody,
				Usage:         &usageparser.NormalizedUsage{},
				UsageSource:   usageparser.SourceEstimated,
				ProviderReqID: "chatcmpl-nousage",
				DurationMs:    42,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newNonStreamEnv(executor, false, "")

	body := validRequestBody()
	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	// A successful call without usage must still charge the estimate: settling
	// zero would make every usage-less upstream response free.
	if walletRepo.settleCalled == 0 {
		t.Fatal("settle was not called")
	}
	if walletRepo.lastSettleAmt.LessThanOrEqual(decimal.Zero) {
		t.Errorf("settle amount = %s, want > 0 (usage-less success must not be free)", walletRepo.lastSettleAmt)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.FinalCost.LessThanOrEqual(decimal.Zero) {
		t.Errorf("usage log FinalCost = %s, want > 0", log.FinalCost)
	}
	if log.UsageSource != domain.UsageSourceEstimated {
		t.Errorf("UsageSource = %s, want %s", log.UsageSource, domain.UsageSourceEstimated)
	}
}

// ============================================================================
// RED: streaming requests must force stream_options.include_usage so the
// upstream actually reports a final usage chunk instead of forcing estimates.
// ============================================================================

func TestHandleStreamingChat_InjectsIncludeUsage(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	gotIncludeUsage := make(chan bool, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]any
		_ = json.NewDecoder(r.Body).Decode(&reqBody)
		opts, _ := reqBody["stream_options"].(map[string]any)
		include, _ := opts["include_usage"].(bool)
		gotIncludeUsage <- include

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-001\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-001\",\"object\":\"chat.completion.chunk\",\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":15,\"total_tokens\":35}}\n\n")
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
	req.Header.Set("X-Request-ID", "test-stream-include-usage")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	select {
	case include := <-gotIncludeUsage:
		if !include {
			t.Error("stream_options.include_usage = false, want true (upstream would not report usage)")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upstream handler never saw the request body")
	}
	// The injected usage must flow into the final-chunk billing path.
	waitForUsageLog(t, usageRepo)
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceFinalChunk {
		t.Errorf("UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceFinalChunk)
	}
}

// ============================================================================
// RED: an upstream failure that already consumed tokens (response carries a
// usage object, e.g. context-length exceeded) must still be billed for the
// consumption instead of vanishing as a zero-cost failure.
// ============================================================================

func TestHandleNonStreamingChat_UpstreamErrorWithUsage_ChargesUsage(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			// 400 with usage: upstream consumed input tokens before rejecting.
			respBody := map[string]any{
				"error": map[string]any{"message": "context_length_exceeded"},
				"usage": map[string]any{
					"prompt_tokens":     float64(100),
					"completion_tokens": float64(0),
					"total_tokens":      float64(100),
				},
			}
			usage, _ := usageparser.ParseOpenAIUsage(respBody)
			return &gw.ExecuteResponse{
				StatusCode:    http.StatusBadRequest,
				Body:          respBody,
				Usage:         usage,
				UsageSource:   usageparser.SourceUpstream,
				ProviderReqID: "chatcmpl-fail-usage",
				DurationMs:    42,
			}, nil
		},
	}
	application, walletRepo, usageRepo := newNonStreamEnv(executor, false, "")

	body := validRequestBody()
	req := newNonStreamChatRequest(userID, apiKeyID, body)
	w := httptest.NewRecorder()
	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	// The request itself still fails for the client (upstream error).
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusBadGateway, w.Body.String())
	}
	// But the consumed tokens must be settled, not released for free.
	if walletRepo.settleCalled == 0 {
		t.Fatal("settle was not called for a failure that consumed tokens")
	}
	if walletRepo.lastSettleAmt.LessThanOrEqual(decimal.Zero) {
		t.Errorf("settle amount = %s, want > 0 (upstream consumed tokens)", walletRepo.lastSettleAmt)
	}
	log := waitForUsageLog(t, usageRepo)
	if log.FinalCost.LessThanOrEqual(decimal.Zero) {
		t.Errorf("usage log FinalCost = %s, want > 0", log.FinalCost)
	}
	if log.UsageSource != domain.UsageSourceUpstream {
		t.Errorf("UsageSource = %s, want %s", log.UsageSource, domain.UsageSourceUpstream)
	}
}
