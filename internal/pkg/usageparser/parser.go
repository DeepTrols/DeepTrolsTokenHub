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

// ParseOpenAIUsage extracts normalized usage from an OpenAI-compatible
// response body. Domestic providers (DeepSeek, Qwen, GLM, Kimi, ...) all
// expose OpenAI-shaped usage; the parser tolerates their field-name variants
// by trying multiple candidate keys per dimension (first non-nil wins).
//
// Upstream-reported usage is untrusted: every dimension is clamped so a
// negative or overlapping count can never under-report usage or double-bill a
// request. TotalTokens falls back to the sum of the token dimensions when the
// provider omits it.
func ParseOpenAIUsage(raw map[string]any) (*NormalizedUsage, error) {
	nu := &NormalizedUsage{}

	usage, ok := raw["usage"].(map[string]any)
	if !ok {
		return nu, nil
	}

	// DeepSeek reports cache hits at the top level (prompt_cache_hit_tokens),
	// while OpenAI-compatible gateways usually nest them in prompt_tokens_details.
	inputDetails, _ := firstNonNil(usage["prompt_tokens_details"], usage["input_tokens_details"]).(map[string]any)
	outputDetails, _ := firstNonNil(usage["completion_tokens_details"], usage["output_tokens_details"]).(map[string]any)

	nu.InputTokens = int64FromAny(firstNonNil(usage["prompt_tokens"], usage["input_tokens"]))
	nu.OutputTokens = int64FromAny(firstNonNil(usage["completion_tokens"], usage["output_tokens"]))
	nu.CacheReadTokens = int64FromAny(firstNonNil(
		inputDetails["cached_tokens"],
		usage["prompt_cache_hit_tokens"], // DeepSeek
		usage["cached_tokens"],
		usage["cached_input_tokens"],
	))
	nu.CacheWriteTokens = int64FromAny(firstNonNil(
		inputDetails["cache_write_tokens"],
		usage["cache_write_input_tokens"],
	))
	nu.ReasoningTokens = int64FromAny(firstNonNil(
		outputDetails["reasoning_tokens"],
		usage["reasoning_tokens"],
	))
	nu.TotalTokens = int64FromAny(usage["total_tokens"])

	clampNormalizedUsage(nu)
	if nu.TotalTokens <= 0 {
		nu.TotalTokens = nu.InputTokens + nu.OutputTokens + nu.CacheReadTokens + nu.CacheWriteTokens
	}

	return nu, nil
}

// clampNormalizedUsage enforces the billable-usage invariants:
//   - no dimension may be negative;
//   - cached reads cannot exceed prompt tokens;
//   - cache writes cannot exceed the uncached portion of the prompt.
//
// It mutates nu in place so callers can never forget to apply it.
func clampNormalizedUsage(nu *NormalizedUsage) {
	nu.InputTokens = max64(nu.InputTokens, 0)
	nu.OutputTokens = max64(nu.OutputTokens, 0)
	nu.CacheReadTokens = clamp64(nu.CacheReadTokens, 0, nu.InputTokens)
	nu.CacheWriteTokens = clamp64(nu.CacheWriteTokens, 0, max64(nu.InputTokens-nu.CacheReadTokens, 0))
	nu.ReasoningTokens = max64(nu.ReasoningTokens, 0)
	nu.TotalTokens = max64(nu.TotalTokens, 0)
}

func firstNonNil(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}

func int64FromAny(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case float32:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	default:
		return 0
	}
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func clamp64(v, lo, hi int64) int64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// EstimateTextTokens estimates the token count of text for pre-call budget
// holds and fallback billing when upstream usage is unavailable. CJK
// characters are close to one token each; non-CJK text is approximated at
// four characters per token (matching the legacy charsPerToken heuristic).
func EstimateTextTokens(text string) int64 {
	var tokens int64
	var nonCJK int64
	for _, r := range text {
		if isCJK(r) {
			tokens++
			continue
		}
		nonCJK++
		if nonCJK >= 4 {
			tokens++
			nonCJK = 0
		}
	}
	return tokens
}

func isCJK(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3400 && r <= 0x4DBF) || // CJK Extension A
		(r >= 0xF900 && r <= 0xFAFF) || // Compatibility Ideographs
		(r >= 0x3040 && r <= 0x30FF) || // Hiragana + Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul
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
