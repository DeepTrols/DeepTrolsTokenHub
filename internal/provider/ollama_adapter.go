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

// OllamaAdapter executes OpenAI chat requests against an Ollama /api/chat
// endpoint. Ollama speaks a near-OpenAI
// messages format; only the endpoint, response envelope and usage counters
// differ.
type OllamaAdapter struct {
	client *http.Client
}

func NewOllamaAdapter() *OllamaAdapter {
	return &OllamaAdapter{client: &http.Client{Timeout: 300 * time.Second}}
}

var _ gw.Executor = (*OllamaAdapter)(nil)

func (o *OllamaAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return o.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body, extraHeaders...)
}

// ExecuteEndpoint converts the OpenAI chat body to an Ollama chat request and
// normalizes the Ollama response (message + prompt_eval_count/eval_count) into
// the OpenAI chat shape the gateway's billing pipeline expects.
func (o *OllamaAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	if upstreamModel == "" {
		return nil, fmt.Errorf("ollama: model required")
	}

	ollamaReq := map[string]any{
		"model":    upstreamModel,
		"messages": ollamaMessages(body["messages"]),
		"stream":   false,
	}
	if options := ollamaOptions(body); len(options) > 0 {
		ollamaReq["options"] = options
	}
	reqBytes, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	url := strings.TrimRight(strings.TrimSpace(baseURL), "/") + "/api/chat"
	start := time.Now()
	respBytes, _, statusCode, _, err := o.doRaw(ctx, apiKey, url, reqBytes, mergeHeaders(extraHeaders...))
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	var ollamaResp struct {
		Error   string `json:"error"`
		Message *struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		Done            bool `json:"done"`
		PromptEvalCount int  `json:"prompt_eval_count"`
		EvalCount       int  `json:"eval_count"`
	}
	if err := json.Unmarshal(respBytes, &ollamaResp); err != nil {
		return nil, fmt.Errorf("ollama: parse response: %w", err)
	}
	if ollamaResp.Error != "" {
		return nil, fmt.Errorf("ollama: upstream error: %s", ollamaResp.Error)
	}

	content := ""
	if ollamaResp.Message != nil {
		content = ollamaResp.Message.Content
	}
	bodyMap := map[string]any{
		"id":      "chatcmpl-ollama",
		"object":  "chat.completion",
		"model":   upstreamModel,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage": map[string]any{
			"prompt_tokens":     ollamaResp.PromptEvalCount,
			"completion_tokens": ollamaResp.EvalCount,
			"total_tokens":      ollamaResp.PromptEvalCount + ollamaResp.EvalCount,
		},
	}
	usage := &usageparser.NormalizedUsage{
		InputTokens:  int64(ollamaResp.PromptEvalCount),
		OutputTokens: int64(ollamaResp.EvalCount),
		TotalTokens:  int64(ollamaResp.PromptEvalCount + ollamaResp.EvalCount),
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
		ProviderReqID: "ollama-" + upstreamModel,
		DurationMs:    durationMs,
	}, nil
}

func (o *OllamaAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.RawResponse, error) {
	return nil, fmt.Errorf("ollama: raw endpoint not supported")
}

func (o *OllamaAdapter) ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return nil, fmt.Errorf("ollama: multipart endpoint not supported")
}

// ollamaMessages maps OpenAI chat messages to Ollama messages, keeping system
// roles (Ollama supports them natively) and flattening content blocks to text.
func ollamaMessages(raw any) []any {
	messages, ok := raw.([]any)
	if !ok {
		return []any{}
	}
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		mm, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := mm["role"].(string)
		if role == "" {
			role = "user"
		}
		content := contentText(mm["content"])
		if content == "" {
			continue
		}
		out = append(out, map[string]any{"role": role, "content": content})
	}
	return out
}

func contentText(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case []any:
		var b strings.Builder
		for _, item := range c {
			if mm, ok := item.(map[string]any); ok {
				if t, ok := mm["text"].(string); ok {
					b.WriteString(t)
				}
			}
		}
		return b.String()
	default:
		return ""
	}
}

func ollamaOptions(body map[string]any) map[string]any {
	out := map[string]any{}
	if t, ok := body["temperature"].(float64); ok {
		out["temperature"] = t
	}
	if p, ok := body["top_p"].(float64); ok {
		out["top_p"] = p
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (o *OllamaAdapter) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte, extraHeaders map[string]string) ([]byte, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("ollama: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := o.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("ollama: upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), nil
}
