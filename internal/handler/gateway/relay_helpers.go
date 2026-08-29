package gateway

import (
	"encoding/json"

	"github.com/deeptrols/api/internal/relayconvert"
)

func structToMap(v any) map[string]any {
	b, _ := json.Marshal(v)
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return map[string]any{}
	}
	return m
}

// parseOpenAIResponseInto hydrates an OpenAIChatResponse from an upstream body
// map (OpenAI chat completion shape).
func parseOpenAIResponseInto(body map[string]any, out *relayconvert.OpenAIChatResponse) {
	if body == nil {
		return
	}
	if id, _ := body["id"].(string); id != "" {
		out.ID = id
	}
	if model, _ := body["model"].(string); model != "" {
		out.Model = model
	}
	if choices, ok := body["choices"].([]any); ok && len(choices) > 0 {
		if c, ok := choices[0].(map[string]any); ok {
			var choice relayconvert.OAIChoice
			choice.Index = 0
			if m, ok := c["message"].(map[string]any); ok {
				choice.Message.Role, _ = m["role"].(string)
				choice.Message.Content, _ = m["content"].(string)
				if tcs, ok := m["tool_calls"].([]any); ok {
					for _, tc := range tcs {
						if tcm, ok := tc.(map[string]any); ok {
							call := relayconvert.OpenAIToolCall{}
							call.ID, _ = tcm["id"].(string)
							call.Type, _ = tcm["type"].(string)
							if fn, ok := tcm["function"].(map[string]any); ok {
								call.Function.Name, _ = fn["name"].(string)
								call.Function.Arguments, _ = fn["arguments"].(string)
							}
							choice.Message.ToolCalls = append(choice.Message.ToolCalls, call)
						}
					}
				}
			}
			choice.FinishReason, _ = c["finish_reason"].(string)
			out.Choices = append(out.Choices, choice)
		}
	}
	if u, ok := body["usage"].(map[string]any); ok {
		out.Usage.PromptTokens = intNum(u["prompt_tokens"])
		out.Usage.CompletionTokens = intNum(u["completion_tokens"])
		out.Usage.TotalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	}
}

func intNum(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}
