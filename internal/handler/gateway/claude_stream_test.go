package gateway

import (
	"context"
	"fmt"
	"net"
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

// claudeStreamUpstream builds an httptest upstream that emits OpenAI SSE
// chunks for /v1/chat/completions (role → text deltas → finish+usage → [DONE]).
func claudeStreamUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, _ := w.(http.Flusher)

		chunks := []string{
			`{"id":"chatcmpl-c1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			`{"id":"chatcmpl-c1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-c1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":" world"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-c1","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":15,"total_tokens":35}}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func claudeStreamApp(t *testing.T, upstreamURL string, walletRepo *mockWalletRepo, usageRepo *mockUsageRepo) (*app.App, uuid.UUID, uuid.UUID) {
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
			return []domain.ChannelInstance{{ID: instanceID, ChannelID: channelID, BaseURL: upstreamURL, ProviderRoute: "deepseek-chat", Config: map[string]any{}, Status: domain.InstanceStatusActive}}, nil
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

func claudeStreamRequest(t *testing.T, application *app.App, userID, apiKeyID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"deepseek-chat","stream":true,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "test-claude-stream")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()
	HandleClaudeMessages(application).ServeHTTP(w, req)
	return w
}

func TestHandleClaudeMessagesStreaming_Success(t *testing.T) {
	upstream := claudeStreamUpstream(t)
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := claudeStreamApp(t, upstream.URL, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	w := claudeStreamRequest(t, application, userID, apiKeyID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}

	respBody := w.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
		"text_delta",
		"end_turn",
	} {
		if !strings.Contains(respBody, want) {
			t.Errorf("response missing %q", want)
		}
	}
	if strings.Contains(respBody, "[DONE]") {
		t.Error("Claude stream must not contain the OpenAI [DONE] marker")
	}
	if got := strings.Count(respBody, "event: message_stop"); got != 1 {
		t.Errorf("expected exactly one message_stop, got %d\n%s", got, respBody)
	}

	// Billing: reserve before upstream, settle after a clean stream, no release.
	if walletRepo.reserveCalled == 0 {
		t.Error("reserve should have been called before upstream streaming")
	}
	if walletRepo.settleCalled == 0 {
		t.Error("settle should have been called after a successful stream")
	}
	if walletRepo.releaseCalled > 0 {
		t.Error("release should not be called on a successful stream")
	}

	// Evidence chain: completed log with final-chunk usage and non-zero cost.
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
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusCompleted {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusCompleted)
	}
	if usageRepo.lastUsageLog.ListCost.Equal(decimal.Zero) {
		t.Error("ListCost should be non-zero")
	}
	if usageRepo.lastEvidence == nil {
		t.Fatal("provider evidence was never recorded")
	}
}

func TestHandleClaudeMessagesStreaming_TruncatedStream_NoMessageStop(t *testing.T) {
	// Upstream sends the message start and one delta, then resets the TCP
	// connection so the stream ends abruptly (invariant #5: no message_stop).
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
		chunks := []string{
			`{"id":"chatcmpl-trunc","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}`,
			`{"id":"chatcmpl-trunc","object":"chat.completion.chunk","model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(buf, "data: %s\n\n", c)
			buf.Flush()
		}
		if tc, ok := conn.(*net.TCPConn); ok {
			tc.SetLinger(0) // RST instead of FIN
		}
		conn.Close()
	}))
	defer upstream.Close()

	walletRepo := &mockWalletRepo{releaseFn: func(ctx context.Context, tID uuid.UUID) error { return nil }}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := claudeStreamApp(t, upstream.URL, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	w := claudeStreamRequest(t, application, userID, apiKeyID)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	respBody := w.Body.String()
	if !strings.Contains(respBody, "event: content_block_delta") {
		t.Error("partial content should still be forwarded")
	}
	if strings.Contains(respBody, "event: message_stop") {
		t.Error("truncated stream must NOT emit message_stop (invariant #5)")
	}

	// The reserved hold must be released and no settle may happen.
	if walletRepo.releaseCalled == 0 {
		t.Error("release should be called after a truncated stream")
	}
	if walletRepo.settleCalled > 0 {
		t.Error("settle should not be called on a truncated stream")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for usageRepo.lastUsageLog == nil && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if usageRepo.lastUsageLog == nil {
		t.Fatal("usage log was never recorded (timed out)")
	}
	// Chunks were delivered before the interruption: the evidence chain must
	// record the stream as partial, never as a clean success.
	if usageRepo.lastUsageLog.Status != domain.UsageLogStatusPartial {
		t.Errorf("Status = %s, want %s", usageRepo.lastUsageLog.Status, domain.UsageLogStatusPartial)
	}
}

func TestHandleClaudeMessages_NonStreamingReturnsJSON(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"echo-1","choices":[{"index":0,"message":{"role":"assistant","content":"echo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	defer upstream.Close()

	walletRepo := &mockWalletRepo{}
	usageRepo := &mockUsageRepo{}
	application, userID, apiKeyID := claudeStreamApp(t, upstream.URL, walletRepo, usageRepo)
	application.HttpClient = upstream.Client()

	body := `{"model":"deepseek-chat","stream":false,"messages":[{"role":"user","content":"Hello"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = setAuthContext(req, userID, apiKeyID)
	w := httptest.NewRecorder()
	HandleClaudeMessages(application).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", w.Code, http.StatusOK, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"type":"message"`) {
		t.Fatalf("expected a Claude message JSON response, got %s", w.Body.String())
	}
	if strings.Contains(w.Body.String(), "event:") {
		t.Fatal("non-streaming request must not produce SSE events")
	}
}
