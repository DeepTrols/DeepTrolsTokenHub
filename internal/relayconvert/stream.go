package relayconvert

import "strings"

// openAIStreamDelta is an OpenAI chat.completion.chunk fragment.
type openAIStreamDelta struct {
	Role         string `json:"role,omitempty"`
	Content      string `json:"content,omitempty"`
	FinishReason string `json:"finish_reason,omitempty"`
}

type openAIChoicesChunk struct {
	Index        int               `json:"index"`
	Delta        openAIStreamDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason,omitempty"`
}

type oaiStreamChunk struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []openAIChoicesChunk `json:"choices"`
}

// ClaudeStreamToOpenAIEvent maps one Anthropic Messages SSE event to an OpenAI
// chat.completion.chunk (text path). Returns ok=false for events that produce no
// chunk (ping, content_block_stop) and done=true on message_stop.
func ClaudeStreamToOpenAIEvent(evt map[string]any) (chunk map[string]any, ok, done bool) {
	typ, _ := evt["type"].(string)
	get := func(key string) string {
		s, _ := evt[key].(string)
		return s
	}
	id, model := "chatcmpl-claude", get("model")
	if m, okm := evt["message"].(map[string]any); okm {
		if v, _ := m["id"].(string); v != "" {
			id = v
		}
		if v, _ := m["model"].(string); v != "" {
			model = v
		}
	}
	chunkFactory := func() map[string]any {
		return map[string]any{
			"id": id, "object": "chat.completion.chunk", "created": 0, "model": model,
			"choices": []map[string]any{},
		}
	}
	choice := func(c openAIChoicesChunk) map[string]any {
		return map[string]any{"index": c.Index, "delta": c.Delta, "finish_reason": c.FinishReason}
	}

	switch typ {
	case "message_start":
		usage := map[string]any{}
		if m, okm := evt["message"].(map[string]any); okm {
			if u, oku := m["usage"].(map[string]any); oku {
				usage = u
			}
		}
		ch := chunkFactory()
		ch["choices"] = []map[string]any{choice(openAIChoicesChunk{Index: 0, Delta: openAIStreamDelta{Role: "assistant"}})}
		if u := mergeClaudeUsage(usage); len(u) > 0 {
			ch["usage"] = u
		}
		return ch, true, false
	case "content_block_delta":
		delta, _ := evt["delta"].(map[string]any)
		dtype, _ := delta["type"].(string)
		if dtype == "text_delta" {
			text, _ := delta["text"].(string)
			ch := chunkFactory()
			ch["choices"] = []map[string]any{choice(openAIChoicesChunk{Index: 0, Delta: openAIStreamDelta{Content: text}})}
			return ch, true, false
		}
		return nil, false, false
	case "message_delta":
		delta, _ := evt["delta"].(map[string]any)
		stop, _ := delta["stop_reason"].(string)
		fr := finishReason(stop)
		usage := map[string]any{}
		if u, oku := evt["usage"].(map[string]any); oku {
			usage = u
		}
		ch := chunkFactory()
		ch["choices"] = []map[string]any{choice(openAIChoicesChunk{Index: 0, Delta: openAIStreamDelta{}, FinishReason: &fr})}
		if u := mergeClaudeUsage(usage); len(u) > 0 {
			ch["usage"] = u
		}
		return ch, true, false
	case "message_stop":
		return nil, false, true
	default:
		return nil, false, false
	}
}

// mergeClaudeUsage maps Anthropic usage keys to OpenAI usage keys.
func mergeClaudeUsage(u map[string]any) map[string]any {
	if len(u) == 0 {
		return nil
	}
	out := map[string]any{}
	if v, ok := u["input_tokens"]; ok {
		out["prompt_tokens"] = v
	}
	if v, ok := u["output_tokens"]; ok {
		out["completion_tokens"] = v
	}
	if len(out) == 0 {
		return nil
	}
	out["total_tokens"] = numOrZero(out["prompt_tokens"]) + numOrZero(out["completion_tokens"])
	return out
}

func numOrZero(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func isClaudeEventData(line string) bool {
	return strings.HasPrefix(line, "data:") && !strings.Contains(line, "event:")
}
