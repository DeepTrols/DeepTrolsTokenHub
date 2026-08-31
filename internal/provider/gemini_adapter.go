package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deeptrols/api/internal/pkg/usageparser"
	"github.com/deeptrols/api/internal/relayconvert"
	gw "github.com/deeptrols/api/internal/service/gateway"
)

// GeminiAdapter executes OpenAI chat requests against a Gemini native
// generateContent endpoint. It is
// stateless like OpenAICompatAdapter; base URL and key arrive per call.
type GeminiAdapter struct {
	client *http.Client
}

func NewGeminiAdapter() *GeminiAdapter {
	return &GeminiAdapter{client: &http.Client{Timeout: 120 * time.Second}}
}

var _ gw.Executor = (*GeminiAdapter)(nil)

// NewExecutorForConfig picks the channel executor by the instance's
// `upstream_format` config: "gemini" → GeminiAdapter, otherwise the
// OpenAI-compatible adapter.
func NewExecutorForConfig(cfg map[string]any) gw.Executor {
	if v, _ := cfg["upstream_format"].(string); strings.EqualFold(strings.TrimSpace(v), "gemini") {
		return NewGeminiAdapter()
	} else if strings.EqualFold(strings.TrimSpace(v), "anthropic") {
		return NewAnthropicAdapter()
	} else if strings.EqualFold(strings.TrimSpace(v), "ollama") {
		return NewOllamaAdapter()
	} else if strings.EqualFold(strings.TrimSpace(v), "azure") {
		return NewAzureAdapterWithConfig(cfg)
	} else if strings.EqualFold(strings.TrimSpace(v), "custom") {
		return NewCustomChannelAdapter(cfg)
	}
	return NewOpenAICompatAdapter()
}

func (g *GeminiAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return g.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body, extraHeaders...)
}

// ExecuteEndpoint converts the OpenAI chat body to a Gemini generateContent
// request, calls `POST {base}/v1beta/models/{model}:generateContent` and
// converts the Gemini response back to the OpenAI chat shape the gateway's
// billing pipeline expects.
func (g *GeminiAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	if upstreamModel == "" {
		return nil, fmt.Errorf("gemini: model required")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}
	var oai relayconvert.OpenAIChatRequest
	if err := json.Unmarshal(b, &oai); err != nil {
		return nil, fmt.Errorf("gemini: parse request: %w", err)
	}
	reqBytes, err := json.Marshal(relayconvert.OpenAIToGeminiRequest(&oai))
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal gemini request: %w", err)
	}

	url := geminiGenerateURL(baseURL, upstreamModel)
	start := time.Now()
	respBytes, _, statusCode, _, err := g.doRaw(ctx, apiKey, url, reqBytes, mergeHeaders(extraHeaders...))
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	var geminiResp relayconvert.GeminiResponse
	if err := json.Unmarshal(respBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("gemini: parse response: %w", err)
	}
	oaiResp := relayconvert.GeminiToOpenAIChatResponse(&geminiResp, upstreamModel)
	bodyMap, err := structToMap(oaiResp)
	if err != nil {
		return nil, err
	}
	usage := &usageparser.NormalizedUsage{
		InputTokens:  int64(geminiResp.UsageMetadata.PromptTokenCount),
		OutputTokens: int64(geminiResp.UsageMetadata.CandidatesTokenCount),
		TotalTokens:  int64(geminiResp.UsageMetadata.TotalTokenCount),
	}
	source := usageparser.SourceUpstream
	if usage.TotalTokens == 0 {
		source = usageparser.SourceEstimated
	}
	return &gw.ExecuteResponse{
		StatusCode:    statusCode,
		Body:          bodyMap,
		Usage:         usage,
		UsageSource:   source,
		ProviderReqID: "gemini-" + upstreamModel,
		DurationMs:    durationMs,
	}, nil
}

func (g *GeminiAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.RawResponse, error) {
	return nil, fmt.Errorf("gemini: raw endpoint not supported")
}

func (g *GeminiAdapter) ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return nil, fmt.Errorf("gemini: multipart endpoint not supported")
}

// geminiGenerateURL builds `{base}/v1beta/models/{model}:generateContent`,
// tolerating base URLs with or without /v1 or /v1beta suffixes.
func geminiGenerateURL(baseURL, model string) string {
	base := strings.TrimSpace(baseURL)
	base = strings.TrimRight(base, "/")
	base = strings.TrimSuffix(base, "/v1beta")
	base = strings.TrimSuffix(base, "/v1")
	return fmt.Sprintf("%s/v1beta/models/%s:generateContent", base, model)
}

func (g *GeminiAdapter) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte, extraHeaders map[string]string) ([]byte, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("gemini: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("gemini: upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), nil
}

func structToMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}
