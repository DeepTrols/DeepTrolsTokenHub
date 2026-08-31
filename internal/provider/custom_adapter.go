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
	gw "github.com/deeptrols/api/internal/service/gateway"
)

// CustomChannelAdapter forwards requests to an admin-defined arbitrary
// endpoint: the instance config
// `custom_override` may set method/url/headers/body templates with the
// placeholders {model}, {api_key}, {base_url} and {request_body}.
type CustomChannelAdapter struct {
	client *http.Client
	cfg    map[string]any
}

func NewCustomChannelAdapter(cfg map[string]any) *CustomChannelAdapter {
	return &CustomChannelAdapter{client: &http.Client{Timeout: 120 * time.Second}, cfg: cfg}
}

var _ gw.Executor = (*CustomChannelAdapter)(nil)

func (c *CustomChannelAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return c.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body, extraHeaders...)
}

type customOverride struct {
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

func (c *CustomChannelAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	var ov customOverride
	if raw, ok := c.cfg["custom_override"]; ok {
		if s, ok := raw.(string); ok {
			_ = json.Unmarshal([]byte(s), &ov)
		} else if m, ok := raw.(map[string]any); ok {
			b, _ := json.Marshal(m)
			_ = json.Unmarshal(b, &ov)
		}
	}
	if ov.Method == "" {
		ov.Method = http.MethodPost
	}

	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("custom: marshal request: %w", err)
	}
	reqBodyStr := string(reqBody)

	url := ov.URL
	if url == "" {
		url = strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/v1/" + strings.TrimPrefix(endpoint, "/")
	}
	url = replaceCustomTokens(url, map[string]string{
		"{model}":        upstreamModel,
		"{api_key}":      apiKey,
		"{base_url}":     strings.TrimRight(baseURL, "/"),
		"{request_body}": reqBodyStr,
	})

	outBody := []byte(reqBodyStr)
	if ov.Body != "" {
		outBody = []byte(replaceCustomTokens(ov.Body, map[string]string{
			"{model}":        upstreamModel,
			"{api_key}":      apiKey,
			"{base_url}":     strings.TrimRight(baseURL, "/"),
			"{request_body}": reqBodyStr,
		}))
	}

	start := time.Now()
	respBytes, _, statusCode, _, err := c.doRaw(ctx, apiKey, ov.Method, url, outBody, ov.Headers, mergeHeaders(extraHeaders...))
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	respBody := map[string]any{}
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &respBody); err != nil {
			respBody = map[string]any{"raw": string(respBytes)}
		}
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

func (c *CustomChannelAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.RawResponse, error) {
	return nil, fmt.Errorf("custom: raw endpoint not supported")
}

func (c *CustomChannelAdapter) ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return nil, fmt.Errorf("custom: multipart endpoint not supported")
}

// replaceCustomTokens replaces {token} placeholders; missing tokens are left
// as-is so malformed templates fail visibly upstream rather than silently.
func replaceCustomTokens(s string, tokens map[string]string) string {
	out := s
	for k, v := range tokens {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func (c *CustomChannelAdapter) doRaw(ctx context.Context, apiKey, method, url string, reqBytes []byte, customHeaders map[string]string, extraHeaders map[string]string) ([]byte, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("custom: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range customHeaders {
		req.Header.Set(k, replaceCustomTokens(v, map[string]string{
			"{api_key}": apiKey,
			"{model}":   "",
		}))
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("custom: upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), nil
}
