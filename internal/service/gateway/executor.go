package gateway

import (
	"context"

	"github.com/deeptrols/api/internal/pkg/usageparser"
)

// Executor is the interface for executing model requests against upstream.
// The OpenAI-compatible implementation lives in internal/provider
// (OpenAICompatAdapter); this interface keeps handlers decoupled from the
// concrete adapter.
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
