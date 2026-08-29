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

// anthropicAPIVersion is the Messages API version header Anthropic requires.
const anthropicAPIVersion = "2023-06-01"

// AnthropicAdapter executes OpenAI chat requests against an Anthropic Messages
// API endpoint (new-api anthropic channel adapter parity). The OpenAI request
// is converted to Claude format and the Claude response converted back, so the
// gateway's billing pipeline sees a normal OpenAI chat completion.
type AnthropicAdapter struct {
	client *http.Client
}

func NewAnthropicAdapter() *AnthropicAdapter {
	return &AnthropicAdapter{client: &http.Client{Timeout: 120 * time.Second}}
}

var _ gw.Executor = (*AnthropicAdapter)(nil)

func (a *AnthropicAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return a.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body, extraHeaders...)
}

// ExecuteEndpoint converts the OpenAI chat body to an Anthropic Messages
// request, calls `POST {base}/v1/messages` with x-api-key / anthropic-version
// headers, and converts the Claude response back to the OpenAI chat shape.
func (a *AnthropicAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	if upstreamModel == "" {
		return nil, fmt.Errorf("anthropic: model required")
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}
	var oai relayconvert.OpenAIChatRequest
	if err := json.Unmarshal(b, &oai); err != nil {
		return nil, fmt.Errorf("anthropic: parse request: %w", err)
	}
	reqBytes, err := json.Marshal(relayconvert.OpenAIToClaudeRequest(&oai))
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal claude request: %w", err)
	}

	url := anthropicMessagesURL(baseURL)
	start := time.Now()
	respBytes, _, statusCode, _, err := a.doRaw(ctx, apiKey, url, reqBytes, mergeHeaders(extraHeaders...))
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	var claudeResp relayconvert.ClaudeResponse
	if err := json.Unmarshal(respBytes, &claudeResp); err != nil {
		return nil, fmt.Errorf("anthropic: parse response: %w", err)
	}
	oaiResp := relayconvert.ClaudeToOpenAIChatResponse(&claudeResp)
	bodyMap, err := structToMap(oaiResp)
	if err != nil {
		return nil, err
	}
	usage := &usageparser.NormalizedUsage{
		InputTokens:  int64(claudeResp.Usage.InputTokens),
		OutputTokens: int64(claudeResp.Usage.OutputTokens),
		TotalTokens:  int64(claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens),
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
		ProviderReqID: "anthropic-" + claudeResp.ID,
		DurationMs:    durationMs,
	}, nil
}

func (a *AnthropicAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.RawResponse, error) {
	return nil, fmt.Errorf("anthropic: raw endpoint not supported")
}

func (a *AnthropicAdapter) ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return nil, fmt.Errorf("anthropic: multipart endpoint not supported")
}

// anthropicMessagesURL builds `{base}/v1/messages`, tolerating bases with or
// without the /v1 suffix.
func anthropicMessagesURL(baseURL string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	base = strings.TrimSuffix(base, "/v1")
	return base + "/v1/messages"
}

func (a *AnthropicAdapter) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte, extraHeaders map[string]string) ([]byte, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("anthropic: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if apiKey != "" {
		// Anthropic authenticates with x-api-key, not Bearer.
		req.Header.Set("x-api-key", apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("anthropic: upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), nil
}
