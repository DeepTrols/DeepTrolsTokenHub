package gateway

import (
	"context"
	"strings"

	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// Executor is the interface for executing model requests against upstream.
// The OpenAI-compatible implementation lives in internal/provider
// (OpenAICompatAdapter); this interface keeps handlers decoupled from the
// concrete adapter.
type Executor interface {
	Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*ExecuteResponse, error)
	// ExecuteEndpoint forwards a request to an arbitrary OpenAI-compatible
	// endpoint (e.g. "embeddings", "images/generations", "audio/speech").
	// The endpoint is relative to /v1 and must not include a leading slash.
	ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*ExecuteResponse, error)
	// ExecuteEndpointRaw forwards a request to an endpoint whose response is
	// not JSON (e.g. audio/speech returns binary audio). The raw body is
	// returned as-is together with the upstream content type.
	ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*RawResponse, error)
	// ExecuteEndpointMultipart forwards a multipart/form-data request (e.g.
	// audio/transcriptions, images/edits). File parts carry raw bytes; field
	// parts are stringified. The JSON-or-text response is returned as an
	// ExecuteResponse with Body either a decoded JSON object or {"text": ...}.
	ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]MultipartFile, extraHeaders ...map[string]string) (*ExecuteResponse, error)
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

// MultipartFile is a single file part of a multipart upstream request.
type MultipartFile struct {
	FileName    string
	ContentType string
	Content     []byte
}

// CustomHeadersFromConfig extracts the optional custom_headers block from a
// channel instance config ({"X-Header": "value", ...}) into a map for the
// executor. Values are stringified; non-string values are skipped.
func CustomHeadersFromConfig(cfg map[string]any) map[string]string {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg["custom_headers"]
	if !ok || raw == nil {
		return nil
	}
	obj, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(obj))
	for k, v := range obj {
		s, ok := v.(string)
		if !ok {
			continue
		}
		key := strings.TrimSpace(k)
		if key != "" {
			out[key] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
