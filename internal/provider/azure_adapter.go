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

// azureDefaultAPIVersion mirrors new-api's AZURE_DEFAULT_API_VERSION.
const azureDefaultAPIVersion = "2025-04-01-preview"

// AzureAdapter executes OpenAI chat requests against an Azure OpenAI
// deployment endpoint (new-api ChannelTypeAzure parity). The body and response
// are OpenAI-compatible; only the URL shape (deployment path + api-version) and
// the api-key auth header differ.
type AzureAdapter struct {
	client           *http.Client
	apiVersion       string
	legacyDeployment bool
}

func NewAzureAdapter() *AzureAdapter {
	return &AzureAdapter{client: &http.Client{Timeout: 120 * time.Second}, apiVersion: azureDefaultAPIVersion}
}

// NewAzureAdapterWithConfig reads the Azure-specific instance config:
// azure_api_version (default 2025-04-01-preview) and azure_legacy_deployment
// (strips dots from deployment names for accounts created before 2025-05-10).
func NewAzureAdapterWithConfig(cfg map[string]any) *AzureAdapter {
	adapter := NewAzureAdapter()
	if v, _ := cfg["azure_api_version"].(string); v != "" {
		adapter.apiVersion = v
	}
	adapter.legacyDeployment = configBoolLike(cfg, "azure_legacy_deployment")
	return adapter
}

var _ gw.Executor = (*AzureAdapter)(nil)

func (a *AzureAdapter) Execute(ctx context.Context, baseURL, apiKey, upstreamModel string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return a.ExecuteEndpoint(ctx, baseURL, apiKey, upstreamModel, "chat/completions", body, extraHeaders...)
}

// ExecuteEndpoint builds `{base}/openai/deployments/{deployment}/{endpoint}
// ?api-version={version}`, authenticates with the api-key header and parses
// the OpenAI-shaped response.
func (a *AzureAdapter) ExecuteEndpoint(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	if upstreamModel == "" {
		return nil, fmt.Errorf("azure: deployment required")
	}
	deployment := upstreamModel
	if a.legacyDeployment {
		deployment = strings.ReplaceAll(deployment, ".", "")
	}
	reqBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("azure: marshal request: %w", err)
	}
	url := fmt.Sprintf("%s/openai/deployments/%s/%s?api-version=%s",
		strings.TrimRight(strings.TrimSpace(baseURL), "/"), deployment, strings.TrimPrefix(endpoint, "/"), a.apiVersion)

	start := time.Now()
	respBytes, _, statusCode, _, err := a.doRaw(ctx, apiKey, url, reqBytes, mergeHeaders(extraHeaders...))
	durationMs := int(time.Since(start).Milliseconds())
	if err != nil {
		return nil, err
	}

	var respBody map[string]any
	if err := json.Unmarshal(respBytes, &respBody); err != nil {
		return nil, fmt.Errorf("azure: parse response: %w", err)
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

func (a *AzureAdapter) ExecuteEndpointRaw(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, body map[string]any, extraHeaders ...map[string]string) (*gw.RawResponse, error) {
	return nil, fmt.Errorf("azure: raw endpoint not supported")
}

func (a *AzureAdapter) ExecuteEndpointMultipart(ctx context.Context, baseURL, apiKey, upstreamModel, endpoint string, fields map[string]any, files map[string]gw.MultipartFile, extraHeaders ...map[string]string) (*gw.ExecuteResponse, error) {
	return nil, fmt.Errorf("azure: multipart endpoint not supported")
}

// configBoolLike tolerates native booleans and JSON-string booleans.
func configBoolLike(cfg map[string]any, key string) bool {
	switch v := cfg[key].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || v == "1"
	}
	return false
}

func (a *AzureAdapter) doRaw(ctx context.Context, apiKey, url string, reqBytes []byte, extraHeaders map[string]string) ([]byte, string, int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("azure: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		// Azure OpenAI authenticates with api-key, not Bearer.
		req.Header.Set("api-key", apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("azure: upstream request failed: %w", err)
	}
	defer resp.Body.Close()
	respBytes, err := readLimited(resp.Body, maxRawResponseBytes)
	if err != nil {
		return nil, "", 0, "", err
	}
	return respBytes, resp.Header.Get("Content-Type"), resp.StatusCode,
		resp.Header.Get("x-request-id"), nil
}
