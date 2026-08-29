package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deeptrols/api/internal/app"
	"github.com/deeptrols/api/internal/domain"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

func responsesChatApp(t *testing.T, upstreamURL string, walletRepo *mockWalletRepo, usageRepo *mockUsageRepo, viaChat bool) (*app.App, uuid.UUID, uuid.UUID) {
	t.Helper()
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
			return []domain.ChannelInstance{{
				ID: instanceID, ChannelID: channelID, BaseURL: upstreamURL, ProviderRoute: "deepseek-chat",
				Config: map[string]any{"responses_via_chat": viaChat, "api_key": "test-key"},
				Status: domain.InstanceStatusActive,
			}}, nil
		},
	}
	pricingRepo := &mockPricingRepo{
		findByModelFn: func(ctx context.Context, mid uuid.UUID, tenantID *uuid.UUID) ([]domain.ModelPricing, error) {
			return makePricingEntries(), nil
		},
	}
	txID := uuid.New()
	walletRepo.findByUserFn = func(ctx context.Context, uid uuid.UUID, tenantID *uuid.UUID) (*domain.Wallet, error) {
		return &domain.Wallet{ID: uuid.New(), UserID: uid, Balance: decimal.NewFromFloat(100.0), Frozen: decimal.Zero}, nil
	}
	walletRepo.reserveFn = func(ctx context.Context, walletID uuid.UUID, amount decimal.Decimal, idempotencyKey string) (*domain.WalletTransaction, error) {
		return &domain.WalletTransaction{ID: txID, WalletID: walletID, Amount: amount, IdempotencyKey: idempotencyKey, TxType: domain.WalletTxReserve}, nil
	}

	application := newTestApp(nil, modelRepo, channelRepo, pricingRepo, walletRepo, usageRepo)
	return application, userID, apiKeyID
}

func responsesRequest(t *testing.T, application *app.App, userID, apiKeyID uuid.UUID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-responses")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()
	HandleResponses(application).ServeHTTP(w, req)
	return w
}

func TestHandleResponsesViaChat_NonStreaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected chat/completions upstream call, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"chatcmpl-r","object":"chat.completion","model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"echo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := responsesChatApp(t, upstream.URL, walletRepo, usageRepo, true)
	application.HttpClient = upstream.Client()

	w := responsesRequest(t, application, userID, apiKeyID, `{"model":"deepseek-chat","input":"hi"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{`"object":"response"`, `"type":"message"`, `"text":"echo"`, `"input_tokens":1`, `"output_tokens":1`} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q\n%s", want, body)
		}
	}
	if walletRepo.reserveCalled == 0 || walletRepo.settleCalled == 0 {
		t.Errorf("expected reserve+settle, got reserve=%d settle=%d", walletRepo.reserveCalled, walletRepo.settleCalled)
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusCompleted)
	}
}

func TestHandleResponsesViaChat_Streaming(t *testing.T) {
	upstream := claudeStreamUpstream(t) // OpenAI SSE chunks: role → text → finish+usage
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := responsesChatApp(t, upstream.URL, walletRepo, usageRepo, true)
	application.HttpClient = upstream.Client()

	w := responsesRequest(t, application, userID, apiKeyID, `{"model":"deepseek-chat","stream":true,"input":"hi"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	respBody := w.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.content_part.added",
		"event: response.output_text.delta",
		"event: response.output_text.done",
		"event: response.content_part.done",
		"event: response.output_item.done",
		"event: response.completed",
		`"delta":"Hello"`,
	} {
		if !strings.Contains(respBody, want) {
			t.Errorf("response missing %q", want)
		}
	}
	if strings.Contains(respBody, "[DONE]") {
		t.Error("Responses stream must not contain the OpenAI chat [DONE] marker")
	}
	if got := strings.Count(respBody, "event: response.completed"); got != 1 {
		t.Errorf("expected exactly one response.completed, got %d\n%s", got, respBody)
	}

	if walletRepo.reserveCalled == 0 || walletRepo.settleCalled == 0 || walletRepo.releaseCalled > 0 {
		t.Errorf("billing mismatch: reserve=%d settle=%d release=%d", walletRepo.reserveCalled, walletRepo.settleCalled, walletRepo.releaseCalled)
	}
	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	if usageRepo.lastUsageLog.UsageSource != domain.UsageSourceFinalChunk {
		t.Errorf("UsageSource = %s, want %s", usageRepo.lastUsageLog.UsageSource, domain.UsageSourceFinalChunk)
	}
}

func TestHandleResponses_WithoutFlag_ForwardsToResponsesEndpoint(t *testing.T) {
	var upstreamPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"resp_1","object":"response","model":"deepseek-chat","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"direct"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := responsesChatApp(t, upstream.URL, walletRepo, usageRepo, false)
	application.HttpClient = upstream.Client()

	w := responsesRequest(t, application, userID, apiKeyID, `{"model":"deepseek-chat","input":"hi"}`)

	if upstreamPath != "/v1/responses" {
		t.Fatalf("expected forward to /v1/responses, got %q", upstreamPath)
	}
	if !strings.Contains(w.Body.String(), `"id":"resp_1"`) {
		t.Fatalf("expected the direct upstream Responses payload, got %s", w.Body.String())
	}
}
