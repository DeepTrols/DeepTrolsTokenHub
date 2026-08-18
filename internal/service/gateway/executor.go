package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// Executor is the interface for executing model requests against upstream.
type Executor interface {
	Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*ExecuteResponse, error)
	// ExecuteEndpoint forwards a request to an arbitrary OpenAI-compatible
	// endpoint (e.g. "embeddings", "images/generations", "audio/speech").
	// The endpoint is relative to /v1 and must not include a leading slash.
	ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*ExecuteResponse, error)
	// ExecuteEndpointRaw forwards a request to an endpoint whose response is
	// not JSON (e.g. audio/speech returns binary audio). The raw body is
	// returned as-is together with the upstream content type.
	ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*RawResponse, error)
}

// LiteLLMExecutor forwards requests to a LiteLLM proxy or provider endpoint.
// It is stateless — base URL and API key are passed per call so different
// channel instances can route to different endpoints.
type LiteLLMExecutor struct {
	client *http.Client
}

func NewLiteLLMExecutor() *LiteLLMExecutor {
	return &LiteLLMExecutor{
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

type ExecuteResponse struct {
	StatusCode    int
	Body          map[string]any
	Usage         *usageparser.NormalizedUsage
	UsageSource   usageparser.Source
	ProviderReqID string
	DurationMs    int
}

// RawResponse is the upstream response for endpoints that return non-JSON
// payloads (e.g. audio/speech binary audio).
type RawResponse struct {
	StatusCode    int
	ContentType   string
	Body          []byte
	ProviderReqID string
	DurationMs    int
}

// maxRawResponseBytes caps non-JSON upstream bodies (audio files). Higher
// than the JSON cap because TTS clips can be several megabytes.
const maxRawResponseBytes = 50 << 20

// Execute sends a chat completion request to the given upstream endpoint.
func (e *LiteLLMExecutor) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*ExecuteResponse, error) {
	return e.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body)
}

// ExecuteEndpoint forwards a JSON body to the given upstream /v1 endpoint.
func (e *LiteLLMExecutor) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*ExecuteResponse, error) {
	reqBytes, url, err := buildUpstreamRequest(baseURL, upstreamModel, endpoint, body)
	if err != nil {
		return nil, err
	}
	respBytes, _, statusCode, _, durationMs, err := e.doRaw(ctx, apiKey, url, reqBytes)
	if err != nil {
		return nil, err
	}

	var respBody map[string]any
	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	usage, err := usageparser.ParseOpenAIUsage(respBody)
	source := usageparser.SourceUpstream
	if err != nil || !usage.HasUsage() {
		usage = &usageparser.NormalizedUsage{}
		source = usageparser.SourceEstimated
	}

	providerReqID := ""
	if id, ok := respBody["id"].(string); ok {
		providerReqID = id
	}

	return &ExecuteResponse{
		StatusCode:    statusCode,
		Body:          respBody,
		Usage:         usage,
		UsageSource:   source,
		ProviderReqID: providerReqID,
		DurationMs:    durationMs,
	}, nil
}

// ExecuteEndpointRaw forwards a request to an endpoint whose response is not
// JSON (e.g. audio/speech binary audio). The raw body is returned as-is with
// the upstream content type so the handler can stream it back to the client.
func (e *LiteLLMExecutor) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*RawResponse, error) {
	reqBytes, url, err := buildUpstreamRequest(baseURL, upstreamModel, endpoint, body)
	if err != nil {
		return nil, err
	}
	respBytes, contentType, statusCode, providerReqID, durationMs, err := e.doRaw(ctx, apiKey, url, reqBytes)
	if err != nil {
		return nil, err
	}
	return &RawResponse{
		StatusCode:    statusCode,
		ContentType:   contentType,
		Body:          respBytes,
		ProviderReqID: providerReqID,
		DurationMs:    durationMs,
	}, nil
}

// buildUpstreamRequest marshals the body with the upstream model name and
// constructs the /v1 URL. The body is shallow-copied so the caller's map is
// never mutated.
func buildUpstreamRequest(baseURL, upstreamModel, endpoint string, body map[string]any) ([]byte, string, error) {
	reqBody := make(map[string]any, len(body)+1)
	for k, v := range body {
		reqBody[k] = v
	}
	reqBody["model"] = upstreamModel

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", fmt.Errorf("marshal request: %w", err)
	}
	url := fmt.Sprintf("%s/v1/%s", strings.TrimSuffix(baseURL, "/v1"), strings.TrimPrefix(endpoint, "/"))
	return reqBytes, url, nil
}

// doRaw performs the HTTP POST and returns the raw response body, content
// type, status code, provider request id and duration. Response bodies are
// capped so a misbehaving upstream cannot exhaust memory.
func (e *LiteLLMExecutor) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte) ([]byte, string, int, string, int, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", 0, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, "", 0, "", 0, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", 0, err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode, resp.Header.Get("x-request-id"), int(time.Since(start).Milliseconds()), nil
}

// readLimited reads up to maxBytes+1 and fails with a clear error when the
// upstream response exceeds the cap — instead of silently truncating and
// failing later with a confusing JSON parse error.
func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("upstream response exceeds %d byte limit", maxBytes)
	}
	return data, nil
}
