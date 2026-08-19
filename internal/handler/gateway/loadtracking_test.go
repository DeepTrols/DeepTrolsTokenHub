package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/deeptrols/api/internal/pkg/usageparser"
	gw "github.com/deeptrols/api/internal/service/gateway"
	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
)

// wireLoadTracker attaches a real Redis-backed LoadTracker (miniredis) to the
// app under test and returns it for assertions.
func wireLoadTracker(t *testing.T, application *app.App) *gw.LoadTracker {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	tracker := gw.NewLoadTracker(client, 60*time.Second)
	application.LoadTracker = tracker
	return tracker
}

func TestHandleNonStreamingChat_LoadHoldReleased_OnInsufficientBalance(t *testing.T) {
	userID := uuid.New()
	apiKeyID := uuid.New()
	channelID := uuid.New()
	instanceID := uuid.New()
	modelID := uuid.New()

	executor := &mockExecutor{
		executeFn: func(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
			t.Error("executor must not be called when balance is insufficient")
			return nil, fmt.Errorf("unexpected call")
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
	}
	usageRepo := &mockUsageRepo{}

	application := newTestApp(executor, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	tracker := wireLoadTracker(t, application)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-load-insufficient")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusPaymentRequired {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusPaymentRequired, w.Body.String())
	}
	if got, err := tracker.Load(context.Background(), instanceID); err != nil || got != 0 {
		t.Errorf("load after insufficient balance = %d, err=%v, want 0 (hold must be released)", got, err)
	}
}

func TestHandleNonStreamingChat_LoadHoldReleased_AfterSuccess(t *testing.T) {
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
				ProviderReqID: "chatcmpl-load-success",
				DurationMs:    5,
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
	tracker := wireLoadTracker(t, application)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-load-success")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleNonStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got, err := tracker.Load(context.Background(), instanceID); err != nil || got != 0 {
		t.Errorf("load after success = %d, err=%v, want 0 (hold must be released)", got, err)
	}
}

func TestHandleStreamingChat_LoadHoldReleased_AfterStreamEnds(t *testing.T) {
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
		chunk := `{"id":"chatcmpl-ss","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`
		fmt.Fprintf(w, "data: %s\n\n", chunk)
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
	tracker := wireLoadTracker(t, application)

	body := validRequestBody()
	respBodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(string(respBodyBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-load-stream")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()

	HandleStreamingChat(w, req, application, "gpt-4o", body)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if got, err := tracker.Load(context.Background(), instanceID); err != nil || got != 0 {
		t.Errorf("load after stream = %d, err=%v, want 0 (hold must be released)", got, err)
	}
}
