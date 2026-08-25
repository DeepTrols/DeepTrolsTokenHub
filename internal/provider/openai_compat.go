package provider

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
	gw "github.com/deeptrols/api/internal/service/gateway"
)

// OpenAICompatAdapter forwards OpenAI-compatible requests to any domestic
// provider endpoint (DeepSeek / Qwen / 智谱 / Kimi / 豆包 / 混元 / 讯飞 / 文心 /
// 零一 / SiliconFlow). It is stateless: base URL and API key are passed per
// call so different channel instances route to different endpoints.
type OpenAICompatAdapter struct {
	client *http.Client
}

func NewOpenAICompatAdapter() *OpenAICompatAdapter {
	return &OpenAICompatAdapter{
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

var _ gw.Executor = (*OpenAICompatAdapter)(nil)

// maxRawResponseBytes caps non-JSON upstream bodies (audio files). Higher
// than the JSON cap because TTS clips can be several megabytes.
const maxRawResponseBytes = 50 << 20

// Execute sends a chat completion request to the given upstream endpoint.
func (e *OpenAICompatAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*gw.ExecuteResponse, error) {
	return e.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body)
}

// ExecuteEndpoint forwards a JSON body to the given upstream /v1 endpoint.
func (e *OpenAICompatAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.ExecuteResponse, error) {
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

	return &gw.ExecuteResponse{
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
func (e *OpenAICompatAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any) (*gw.RawResponse, error) {
	reqBytes, url, err := buildUpstreamRequest(baseURL, upstreamModel, endpoint, body)
	if err != nil {
		return nil, err
	}
	respBytes, contentType, statusCode, providerReqID, durationMs, err := e.doRaw(ctx, apiKey, url, reqBytes)
	if err != nil {
		return nil, err
	}
	return &gw.RawResponse{
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
func (e *OpenAICompatAdapter) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte) ([]byte, string, int, string, int, error) {
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
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), int(time.Since(start).Milliseconds()), nil
}

// readLimited reads at most maxBytes from r, refusing oversized bodies.
func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	limited := io.LimitReader(r, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("upstream response exceeds %d bytes", maxBytes)
	}
	return data, nil
}

// InferCategory derives a model catalog category from a discovered model ID
// and the provider template's declared capabilities. This is the deterministic
// capability-probe shortcut: no extra upstream calls, best-effort accuracy.
func InferCategory(modelID string, t Template) string {
	lower := strings.ToLower(modelID)
	switch {
	case strings.Contains(lower, "embedding") || strings.Contains(lower, "bge-") || strings.Contains(lower, "text-embedding"):
		return "embedding"
	case strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "audio") || strings.Contains(lower, "voice"):
		return "audio"
	case strings.Contains(lower, "video") || strings.Contains(lower, "seedance") || strings.Contains(lower, "cogvideo"):
		return "video"
	case strings.Contains(lower, "image") || strings.Contains(lower, "wanx") || strings.Contains(lower, "cogview") || strings.Contains(lower, "flux"):
		return "image"
	default:
		return "chat"
	}
}
