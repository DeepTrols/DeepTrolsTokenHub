package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// ServiceConfig holds cache tuning parameters.
type ServiceConfig struct {
	TTL            time.Duration
	AcceptedModels map[string]bool
}

// Service provides hash-based response caching. Identical requests
// (model + messages + key params) hit cache and skip the upstream call.
type Service struct {
	client *redis.Client
	cfg    ServiceConfig
}

// New creates a Service. nil client = caching silently disabled.
func New(client *redis.Client, cfg ServiceConfig) *Service {
	if client == nil {
		return &Service{cfg: cfg}
	}
	return &Service{client: client, cfg: cfg}
}

// IsEnabled returns true when ready to cache. A nil receiver is treated as
// disabled (defensive against typed-nil interface conversions).
func (s *Service) IsEnabled() bool { return s != nil && s.client != nil }

// IsModelAccepted returns true when the model may be cached.
func (s *Service) IsModelAccepted(model string) bool {
	if s == nil {
		return false
	}
	if len(s.cfg.AcceptedModels) == 0 {
		return true
	}
	return s.cfg.AcceptedModels[model]
}

// BuildKey returns a deterministic SHA-256 key from model + body params.
// scope (e.g. tenant:user) is mixed into the key so cached responses are
// never shared across tenants or users.
func BuildKey(model string, body map[string]any, scope ...string) string {
	payload := map[string]any{
		"model":    model,
		"messages": body["messages"],
		"scope":    scope,
	}
	for _, p := range []string{
		"temperature", "top_p", "top_k",
		"max_tokens", "max_completion_tokens",
		"seed", "stop", "tools", "tool_choice",
		"response_format", "frequency_penalty", "presence_penalty",
	} {
		if v, ok := body[p]; ok {
			payload[p] = v
		}
	}
	raw, _ := json.Marshal(payload)
	h := sha256.Sum256(raw)
	return "cache:response:" + hex.EncodeToString(h[:])
}

// CachedResponse is what we store and retrieve.
type CachedResponse struct {
	StatusCode   int    `json:"status_code"`
	Body         string `json:"body"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Model        string `json:"model"`
}

// Get looks up a cached response. Returns nil, nil on miss.
func (s *Service) Get(ctx context.Context, key string) (*CachedResponse, error) {
	if !s.IsEnabled() {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache get: %w", err)
	}
	var cr CachedResponse
	if err := json.Unmarshal([]byte(raw), &cr); err != nil {
		return nil, fmt.Errorf("cache unmarshal: %w", err)
	}
	return &cr, nil
}

// Set stores a response. TTL defaults to 1h.
func (s *Service) Set(ctx context.Context, key string, cr *CachedResponse) error {
	if !s.IsEnabled() {
		return nil
	}
	raw, err := json.Marshal(cr)
	if err != nil {
		return fmt.Errorf("cache marshal: %w", err)
	}
	ttl := s.cfg.TTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	return s.client.Set(ctx, key, raw, ttl).Err()
}
