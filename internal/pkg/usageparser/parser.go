package usageparser

import (
	"encoding/json"
)

// NormalizedUsage represents platform-standardized usage dimensions.
// Cross-protocol normalization: OpenAI ↔ Anthropic ↔ Gemini.
type NormalizedUsage struct {
	InputTokens      int64 `json:"input_tokens,omitempty"`
	OutputTokens     int64 `json:"output_tokens,omitempty"`
	CacheReadTokens  int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int64 `json:"cache_write_tokens,omitempty"`
	ReasoningTokens  int64 `json:"reasoning_tokens,omitempty"`
	ImageCount       int64 `json:"image_count,omitempty"`
	AudioSeconds     int64 `json:"audio_seconds,omitempty"`
	TTSCharacters    int64 `json:"tts_characters,omitempty"`
	VideoUnits       int64 `json:"video_units,omitempty"`
	TotalTokens      int64 `json:"total_tokens,omitempty"`
}

// Source indicates where the usage data originated.
type Source string

const (
	SourceUpstream   Source = "upstream"
	SourceFinalChunk Source = "final_chunk"
	SourceEstimated  Source = "estimated"
)

// ParseOpenAIUsage extracts normalized usage from an OpenAI response body.
func ParseOpenAIUsage(raw map[string]any) (*NormalizedUsage, error) {
	nu := &NormalizedUsage{}

	usage, ok := raw["usage"].(map[string]any)
	if !ok {
		return nu, nil
	}

	if v, ok := usage["prompt_tokens"].(float64); ok {
		nu.InputTokens = int64(v)
	}
	if v, ok := usage["completion_tokens"].(float64); ok {
		nu.OutputTokens = int64(v)
	}
	if v, ok := usage["total_tokens"].(float64); ok {
		nu.TotalTokens = int64(v)
	}

	// Check for cached tokens in prompt_tokens_details
	if details, ok := usage["prompt_tokens_details"].(map[string]any); ok {
		if v, ok := details["cached_tokens"].(float64); ok {
			nu.CacheReadTokens = int64(v)
		}
	}
	if details, ok := usage["completion_tokens_details"].(map[string]any); ok {
		if v, ok := details["reasoning_tokens"].(float64); ok {
			nu.ReasoningTokens = int64(v)
		}
	}

	return nu, nil
}

// ParseAnthropicUsage extracts normalized usage from an Anthropic response body.
func ParseAnthropicUsage(raw map[string]any) (*NormalizedUsage, error) {
	nu := &NormalizedUsage{}

	usage, ok := raw["usage"].(map[string]any)
	if !ok {
		return nu, nil
	}

	if v, ok := usage["input_tokens"].(float64); ok {
		nu.InputTokens = int64(v)
	}
	if v, ok := usage["output_tokens"].(float64); ok {
		nu.OutputTokens = int64(v)
	}
	if v, ok := usage["cache_read_input_tokens"].(float64); ok {
		nu.CacheReadTokens = int64(v)
	}
	if v, ok := usage["cache_creation_input_tokens"].(float64); ok {
		nu.CacheWriteTokens = int64(v)
	}

	nu.TotalTokens = nu.InputTokens + nu.OutputTokens + nu.CacheReadTokens

	return nu, nil
}

// ToJSON marshals normalized usage to a JSON-compatible map.
func (nu *NormalizedUsage) ToJSON() map[string]any {
	var result map[string]any
	data, _ := json.Marshal(nu)
	json.Unmarshal(data, &result)
	return result
}

// HasUsage returns true if at least one usage dimension is non-zero.
func (nu *NormalizedUsage) HasUsage() bool {
	return nu.InputTokens > 0 || nu.OutputTokens > 0 || nu.CacheReadTokens > 0 ||
		nu.CacheWriteTokens > 0 || nu.ReasoningTokens > 0 || nu.ImageCount > 0 ||
		nu.AudioSeconds > 0 || nu.TTSCharacters > 0 || nu.VideoUnits > 0 || nu.TotalTokens > 0
}
