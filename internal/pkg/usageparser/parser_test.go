package usageparser

import "testing"

func TestParseOpenAIUsage_Success(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.InputTokens != 100 {
		t.Errorf("InputTokens = %d, want 100", nu.InputTokens)
	}
	if nu.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d, want 50", nu.OutputTokens)
	}
	if nu.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150", nu.TotalTokens)
	}
}

func TestParseOpenAIUsage_NoUsage(t *testing.T) {
	raw := map[string]any{"choices": []any{}}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.HasUsage() {
		t.Error("should have no usage")
	}
}

func TestParseOpenAIUsage_CachedTokens(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
			"prompt_tokens_details": map[string]any{
				"cached_tokens": float64(30),
			},
			"completion_tokens_details": map[string]any{
				"reasoning_tokens": float64(10),
			},
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.CacheReadTokens != 30 {
		t.Errorf("CacheReadTokens = %d, want 30", nu.CacheReadTokens)
	}
	if nu.ReasoningTokens != 10 {
		t.Errorf("ReasoningTokens = %d, want 10", nu.ReasoningTokens)
	}
}

func TestParseAnthropicUsage_Success(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"input_tokens":                float64(200),
			"output_tokens":               float64(100),
			"cache_read_input_tokens":     float64(50),
			"cache_creation_input_tokens": float64(25),
		},
	}
	nu, err := ParseAnthropicUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200", nu.InputTokens)
	}
	if nu.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", nu.OutputTokens)
	}
	if nu.CacheReadTokens != 50 {
		t.Errorf("CacheReadTokens = %d, want 50", nu.CacheReadTokens)
	}
	if nu.CacheWriteTokens != 25 {
		t.Errorf("CacheWriteTokens = %d, want 25", nu.CacheWriteTokens)
	}
	if nu.TotalTokens != 350 {
		t.Errorf("TotalTokens = %d, want 350", nu.TotalTokens)
	}
}

func TestParseAnthropicUsage_NoUsage(t *testing.T) {
	raw := map[string]any{"content": []any{}}
	nu, err := ParseAnthropicUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.HasUsage() {
		t.Error("should have no usage")
	}
}

func TestNormalizedUsage_HasUsage(t *testing.T) {
	tests := []struct {
		name string
		nu   *NormalizedUsage
		want bool
	}{
		{"empty", &NormalizedUsage{}, false},
		{"input only", &NormalizedUsage{InputTokens: 10}, true},
		{"output only", &NormalizedUsage{OutputTokens: 10}, true},
		{"cache only", &NormalizedUsage{CacheReadTokens: 5}, true},
		{"reasoning only", &NormalizedUsage{ReasoningTokens: 3}, true},
		{"image only", &NormalizedUsage{ImageCount: 1}, true},
		{"audio only", &NormalizedUsage{AudioSeconds: 1}, true},
		{"total only", &NormalizedUsage{TotalTokens: 100}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nu.HasUsage(); got != tt.want {
				t.Errorf("HasUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizedUsage_ToJSON(t *testing.T) {
	nu := &NormalizedUsage{InputTokens: 100, OutputTokens: 50, TotalTokens: 150}
	json := nu.ToJSON()
	if json["input_tokens"] != float64(100) {
		t.Errorf("input_tokens = %v, want 100", json["input_tokens"])
	}
}

// ============================================================================
// RED tests: DeepSeek cache fields, cache write, clamps, total fallback.
// ============================================================================

func TestParseOpenAIUsage_DeepSeekCacheHitTokens(t *testing.T) {
	// DeepSeek reports cache hits at the top level of usage, not inside
	// prompt_tokens_details. Missing this field silently bills the cached
	// input at the full input price (cache price is ~1/30 of input).
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":           float64(100),
			"completion_tokens":       float64(50),
			"total_tokens":            float64(150),
			"prompt_cache_hit_tokens": float64(30),
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.CacheReadTokens != 30 {
		t.Errorf("CacheReadTokens = %d, want 30", nu.CacheReadTokens)
	}
}

func TestParseOpenAIUsage_CacheWriteTokens(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
			"prompt_tokens_details": map[string]any{
				"cached_tokens":      float64(30),
				"cache_write_tokens": float64(40),
			},
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.CacheWriteTokens != 40 {
		t.Errorf("CacheWriteTokens = %d, want 40", nu.CacheWriteTokens)
	}
}

func TestParseOpenAIUsage_ClampsNegativeUsage(t *testing.T) {
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(-5),
			"completion_tokens": float64(-3),
			"total_tokens":      float64(-8),
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.InputTokens != 0 {
		t.Errorf("InputTokens = %d, want 0 (negative clamped)", nu.InputTokens)
	}
	if nu.OutputTokens != 0 {
		t.Errorf("OutputTokens = %d, want 0 (negative clamped)", nu.OutputTokens)
	}
	if nu.TotalTokens != 0 {
		t.Errorf("TotalTokens = %d, want 0 (negative clamped)", nu.TotalTokens)
	}
}

func TestParseOpenAIUsage_ClampsCachedToPrompt(t *testing.T) {
	// Upstream claiming more cached tokens than prompt tokens is malformed;
	// charging it would double-bill the same input.
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
			"total_tokens":      float64(150),
			"prompt_tokens_details": map[string]any{
				"cached_tokens": float64(150),
			},
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.CacheReadTokens != 100 {
		t.Errorf("CacheReadTokens = %d, want 100 (clamped to prompt)", nu.CacheReadTokens)
	}
}

func TestParseOpenAIUsage_TotalTokensFallback(t *testing.T) {
	// Some providers omit total_tokens; the parser must derive it from the
	// token dimensions so quota settlement is never zero by accident.
	raw := map[string]any{
		"usage": map[string]any{
			"prompt_tokens":     float64(100),
			"completion_tokens": float64(50),
		},
	}
	nu, err := ParseOpenAIUsage(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if nu.TotalTokens != 150 {
		t.Errorf("TotalTokens = %d, want 150 (derived from input+output)", nu.TotalTokens)
	}
}
