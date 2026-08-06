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

// Executor is the interface for executing chat completion requests against upstream.
type Executor interface {
	Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*ExecuteResponse, error)
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

// Execute sends a chat completion request to the given upstream endpoint.
func (e *LiteLLMExecutor) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any) (*ExecuteResponse, error) {
	start := time.Now()

	// Shallow copy to avoid mutating the caller's body map.
	reqBody := make(map[string]any, len(body)+1)
	for k, v := range body {
		reqBody[k] = v
	}
	reqBody["model"] = upstreamModel

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", strings.TrimSuffix(baseURL, "/v1"))
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := readLimited(resp.Body, 10<<20) // 10 MB cap, truncation detected
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
		StatusCode:    resp.StatusCode,
		Body:          respBody,
		Usage:         usage,
		UsageSource:   source,
		ProviderReqID: providerReqID,
		DurationMs:    int(time.Since(start).Milliseconds()),
	}, nil
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
