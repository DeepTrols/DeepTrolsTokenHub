package relayconvert

import (
	"strings"
	"time"
)

// ResponsesStreamState tracks OpenAI chat.completion.chunk SSE → OpenAI
// Responses SSE conversion (oai_chat → oai_responses).
// The terminal sequence is deferred until both the finish reason and usage are
// observed so clients never see a premature response.completed.
type ResponsesStreamState struct {
	ID      string
	Model   string
	Created int64
	Done    bool

	finishReason  string
	usage         map[string]any
	nextIndex     int
	textStarted   bool
	textIndex     int
	text          strings.Builder
	reasonStarted bool
	reasonIndex   int
	reason        strings.Builder
	toolsByIndex  map[int]*responsesToolState
	toolOrder     []*responsesToolState
	itemOrder     []string
}

type responsesToolState struct {
	outputIndex int
	id          string
	name        string
	args        strings.Builder
}

// OpenAIStreamChunkToResponsesEvents converts one OpenAI chat.completion.chunk
// into zero or more OpenAI Responses SSE events. A nil/empty result means the
// chunk produced no client-visible output (usage-only tail or after terminal).
func OpenAIStreamChunkToResponsesEvents(chunk map[string]any, state *ResponsesStreamState) []map[string]any {
	if state == nil {
		state = &ResponsesStreamState{}
	}
	if state.Done {
		return nil
	}

	var events []map[string]any

	// First chunk opens the response envelope.
	if state.ID == "" {
		state.ID, _ = chunk["id"].(string)
		state.Model, _ = chunk["model"].(string)
		state.Created = time.Now().Unix()
		events = append(events, map[string]any{
			"type": "response.created",
			"response": map[string]any{
				"id":         state.ID,
				"object":     "response",
				"created_at": state.Created,
				"status":     "in_progress",
				"model":      state.Model,
				"output":     []any{},
				"usage":      nil,
			},
		})
	}

	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				events = append(events, responsesDeltaEvents(delta, state)...)
			}
			if fr, ok := choice["finish_reason"].(string); ok && fr != "" && state.finishReason == "" {
				state.finishReason = fr
			}
		}
	}

	if u, ok := chunk["usage"].(map[string]any); ok && len(u) > 0 {
		state.usage = u
	}

	if state.finishReason != "" && len(state.usage) > 0 {
		events = append(events, state.finalizeEvents()...)
		state.Done = true
	}
	return events
}

// responsesDeltaEvents maps one OpenAI stream delta to Responses output-item
// events (reasoning summary, output text, function call arguments).
func responsesDeltaEvents(delta map[string]any, state *ResponsesStreamState) []map[string]any {
	var events []map[string]any

	if rc, _ := delta["reasoning_content"].(string); rc != "" {
		if !state.reasonStarted {
			state.reasonStarted = true
			state.reasonIndex = state.nextIndex
			state.nextIndex++
			state.itemOrder = append(state.itemOrder, "reasoning")
			events = append(events, map[string]any{
				"type":         "response.output_item.added",
				"output_index": state.reasonIndex,
				"item": map[string]any{
					"type":    "reasoning",
					"id":      "rs_" + state.ID,
					"status":  "in_progress",
					"summary": []any{},
				},
			})
		}
		state.reason.WriteString(rc)
		events = append(events, map[string]any{
			"type":          "response.reasoning_summary.delta",
			"output_index":  state.reasonIndex,
			"summary_index": 0,
			"item_id":       "rs_" + state.ID,
			"delta":         rc,
		})
	}

	if content, _ := delta["content"].(string); content != "" {
		if !state.textStarted {
			state.textStarted = true
			state.textIndex = state.nextIndex
			state.nextIndex++
			state.itemOrder = append(state.itemOrder, "text")
			events = append(events, map[string]any{
				"type":         "response.output_item.added",
				"output_index": state.textIndex,
				"item": map[string]any{
					"type":    "message",
					"id":      "msg_" + state.ID,
					"status":  "in_progress",
					"role":    "assistant",
					"content": []any{},
				},
			})
			events = append(events, map[string]any{
				"type":          "response.content_part.added",
				"output_index":  state.textIndex,
				"content_index": 0,
				"item_id":       "msg_" + state.ID,
				"content": map[string]any{
					"type":        "output_text",
					"text":        "",
					"annotations": []any{},
				},
			})
		}
		state.text.WriteString(content)
		events = append(events, map[string]any{
			"type":          "response.output_text.delta",
			"output_index":  state.textIndex,
			"content_index": 0,
			"item_id":       "msg_" + state.ID,
			"delta":         content,
		})
	}

	if tcs, ok := delta["tool_calls"].([]any); ok {
		if state.toolsByIndex == nil {
			state.toolsByIndex = map[int]*responsesToolState{}
		}
		for _, tcAny := range tcs {
			tc, _ := tcAny.(map[string]any)
			oi, _ := tc["index"].(float64)
			oi64 := int(oi)
			tool := state.toolsByIndex[oi64]
			if tool == nil {
				tool = &responsesToolState{}
				tool.id, _ = tc["id"].(string)
				if fn, ok := tc["function"].(map[string]any); ok {
					tool.name, _ = fn["name"].(string)
				}
				if tool.id == "" {
					tool.id = state.ID + "_call_" + itoa(oi64)
				}
				tool.outputIndex = state.nextIndex
				state.nextIndex++
				state.toolsByIndex[oi64] = tool
				state.toolOrder = append(state.toolOrder, tool)
				state.itemOrder = append(state.itemOrder, "tool")
				events = append(events, map[string]any{
					"type":         "response.output_item.added",
					"output_index": tool.outputIndex,
					"item": map[string]any{
						"type":      "function_call",
						"id":        tool.id,
						"call_id":   tool.id,
						"name":      tool.name,
						"arguments": "",
						"status":    "in_progress",
					},
				})
			}
			if fn, ok := tc["function"].(map[string]any); ok {
				if args, ok := fn["arguments"].(string); ok && args != "" {
					tool.args.WriteString(args)
					events = append(events, map[string]any{
						"type":         "response.function_call_arguments.delta",
						"output_index": tool.outputIndex,
						"item_id":      tool.id,
						"delta":        args,
					})
				}
			}
		}
	}

	return events
}

// finalizeEvents closes every open output item in start order and emits
// response.completed with the final output array and usage.
func (state *ResponsesStreamState) finalizeEvents() []map[string]any {
	var events []map[string]any
	output := make([]any, 0, len(state.itemOrder))
	toolCursor := 0

	for _, kind := range state.itemOrder {
		switch kind {
		case "reasoning":
			events = append(events, map[string]any{
				"type":          "response.reasoning_summary.done",
				"output_index":  state.reasonIndex,
				"summary_index": 0,
				"item_id":       "rs_" + state.ID,
				"summary":       []any{},
			})
			events = append(events, map[string]any{
				"type":          "response.content_part.done",
				"output_index":  state.reasonIndex,
				"content_index": 0,
				"item_id":       "rs_" + state.ID,
			})
			item := map[string]any{
				"type":    "reasoning",
				"id":      "rs_" + state.ID,
				"status":  "completed",
				"summary": []any{},
			}
			events = append(events, map[string]any{
				"type":         "response.output_item.done",
				"output_index": state.reasonIndex,
				"item":         item,
			})
			output = append(output, item)
		case "text":
			text := state.text.String()
			events = append(events, map[string]any{
				"type":          "response.output_text.done",
				"output_index":  state.textIndex,
				"content_index": 0,
				"item_id":       "msg_" + state.ID,
				"text":          text,
			})
			events = append(events, map[string]any{
				"type":          "response.content_part.done",
				"output_index":  state.textIndex,
				"content_index": 0,
				"item_id":       "msg_" + state.ID,
			})
			item := map[string]any{
				"type":   "message",
				"id":     "msg_" + state.ID,
				"status": "completed",
				"role":   "assistant",
				"content": []any{map[string]any{
					"type":        "output_text",
					"text":        text,
					"annotations": []any{},
				}},
			}
			events = append(events, map[string]any{
				"type":         "response.output_item.done",
				"output_index": state.textIndex,
				"item":         item,
			})
			output = append(output, item)
		case "tool":
			if toolCursor >= len(state.toolOrder) {
				continue
			}
			tool := state.toolOrder[toolCursor]
			toolCursor++
			args := tool.args.String()
			events = append(events, map[string]any{
				"type":         "response.function_call_arguments.done",
				"output_index": tool.outputIndex,
				"item_id":      tool.id,
				"arguments":    args,
			})
			item := map[string]any{
				"type":      "function_call",
				"id":        tool.id,
				"call_id":   tool.id,
				"name":      tool.name,
				"arguments": args,
				"status":    "completed",
			}
			events = append(events, map[string]any{
				"type":         "response.output_item.done",
				"output_index": tool.outputIndex,
				"item":         item,
			})
			output = append(output, item)
		}
	}

	status := "completed"
	switch state.finishReason {
	case "length", "content_filter":
		status = "incomplete"
	}
	events = append(events, map[string]any{
		"type": "response.completed",
		"response": map[string]any{
			"id":         state.ID,
			"object":     "response",
			"created_at": state.Created,
			"status":     status,
			"model":      state.Model,
			"output":     output,
			"usage": map[string]any{
				"input_tokens":  streamInt(state.usage["prompt_tokens"]),
				"output_tokens": streamInt(state.usage["completion_tokens"]),
				"total_tokens":  streamInt(state.usage["total_tokens"]),
			},
		},
	})
	return events
}

// ForceFinishEvents terminalizes an unterminated stream (clean upstream EOF
// without a finish/usage chunk) so clients never hang waiting for completion.
func (state *ResponsesStreamState) ForceFinishEvents() []map[string]any {
	if state.Done {
		return nil
	}
	if state.finishReason == "" {
		state.finishReason = "stop"
	}
	events := state.finalizeEvents()
	state.Done = true
	return events
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
