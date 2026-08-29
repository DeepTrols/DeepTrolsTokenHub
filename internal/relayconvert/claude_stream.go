package relayconvert

// ClaudeStreamState tracks the OpenAI SSE → Anthropic Messages SSE conversion
// across chunks. It mirrors new-api's ClaudeConvertInfo state machine: emit
// message_start once, open/close content blocks (text / thinking / tool_use),
// and defer the terminal sequence until both the finish reason and usage have
// been observed so an Anthropic client never sees a premature message_stop.
type ClaudeStreamState struct {
	Done      bool
	Started   bool
	MessageID string
	Model     string

	finishReason string
	usage        map[string]any
	nextIndex    int
	openBlocks   []claudeOpenBlock
	textOpen     bool
	thinkingOpen bool
	toolByIndex  map[int]*claudeToolState
}

type claudeOpenBlock struct {
	index int
	kind  string
}

type claudeToolState struct {
	index int
	id    string
	name  string
}

// OpenAIStreamChunkToClaudeEvents converts one OpenAI chat.completion.chunk
// into zero or more Anthropic Messages SSE events. A nil/empty result means
// the chunk produced no client-visible output (e.g. a usage-only tail after
// terminalization, or a stream that already reached message_stop).
func OpenAIStreamChunkToClaudeEvents(chunk map[string]any, state *ClaudeStreamState) []map[string]any {
	if state == nil {
		state = &ClaudeStreamState{}
	}
	if state.Done {
		return nil
	}

	var events []map[string]any

	// First chunk of the stream opens the Anthropic message envelope.
	if !state.Started {
		state.Started = true
		state.MessageID, _ = chunk["id"].(string)
		state.Model, _ = chunk["model"].(string)
		events = append(events, map[string]any{
			"type": "message_start",
			"message": map[string]any{
				"id":            state.MessageID,
				"type":          "message",
				"role":          "assistant",
				"model":         state.Model,
				"content":       []any{},
				"stop_reason":   nil,
				"stop_sequence": nil,
				"usage": map[string]any{
					"input_tokens":  0,
					"output_tokens": 0,
				},
			},
		})
	}

	if choices, ok := chunk["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if delta, ok := choice["delta"].(map[string]any); ok {
				events = append(events, convertClaudeDelta(delta, state)...)
			}
			if fr, ok := choice["finish_reason"].(string); ok && fr != "" && state.finishReason == "" {
				state.finishReason = fr
			}
		}
	}

	if u, ok := chunk["usage"].(map[string]any); ok && len(u) > 0 {
		state.usage = u
	}

	// Terminalize only after the finish reason AND usage are both known; a
	// finish chunk without usage is deferred until the usage-only tail chunk.
	if state.finishReason != "" && len(state.usage) > 0 {
		events = append(events, state.terminalEvents()...)
		state.Done = true
	}
	return events
}

// convertClaudeDelta maps one OpenAI stream delta (text / reasoning / tool
// calls) to Anthropic content block events, opening a block on first content.
func convertClaudeDelta(delta map[string]any, state *ClaudeStreamState) []map[string]any {
	var events []map[string]any

	// reasoning_content → thinking block (DeepSeek / reasoning providers).
	if rc, _ := delta["reasoning_content"].(string); rc != "" {
		if !state.thinkingOpen {
			events = append(events, state.closeOpenBlocks()...)
			idx := state.nextIndex
			state.nextIndex++
			events = append(events, map[string]any{
				"type":          "content_block_start",
				"index":         idx,
				"content_block": map[string]any{"type": "thinking", "thinking": ""},
			})
			state.openBlocks = append(state.openBlocks, claudeOpenBlock{index: idx, kind: "thinking"})
			state.thinkingOpen = true
		}
		events = append(events, map[string]any{
			"type":  "content_block_delta",
			"index": state.openBlocks[len(state.openBlocks)-1].index,
			"delta": map[string]any{"type": "thinking_delta", "thinking": rc},
		})
	}

	// content → text block.
	if content, _ := delta["content"].(string); content != "" {
		if !state.textOpen {
			events = append(events, state.closeOpenBlocks()...)
			idx := state.nextIndex
			state.nextIndex++
			events = append(events, map[string]any{
				"type":          "content_block_start",
				"index":         idx,
				"content_block": map[string]any{"type": "text", "text": ""},
			})
			state.openBlocks = append(state.openBlocks, claudeOpenBlock{index: idx, kind: "text"})
			state.textOpen = true
		}
		events = append(events, map[string]any{
			"type":  "content_block_delta",
			"index": state.openBlocks[len(state.openBlocks)-1].index,
			"delta": map[string]any{"type": "text_delta", "text": content},
		})
	}

	// tool_calls → tool_use block(s) with input_json_delta fragments. OpenAI
	// streams parallel tool calls by index; the first sight of an index opens
	// the block, later fragments append partial JSON.
	if tcs, ok := delta["tool_calls"].([]any); ok {
		if state.toolByIndex == nil {
			state.toolByIndex = map[int]*claudeToolState{}
		}
		for _, tcAny := range tcs {
			tc, _ := tcAny.(map[string]any)
			oi, _ := tc["index"].(float64)
			oi64 := int(oi)
			st := state.toolByIndex[oi64]
			if st == nil {
				st = &claudeToolState{}
				st.id, _ = tc["id"].(string)
				if fn, ok := tc["function"].(map[string]any); ok {
					st.name, _ = fn["name"].(string)
				}
				state.toolByIndex[oi64] = st
				idx := state.nextIndex
				state.nextIndex++
				st.index = idx
				events = append(events, map[string]any{
					"type":          "content_block_start",
					"index":         idx,
					"content_block": map[string]any{"type": "tool_use", "id": st.id, "name": st.name, "input": map[string]any{}},
				})
				state.openBlocks = append(state.openBlocks, claudeOpenBlock{index: idx, kind: "tool"})
			}
			if fn, ok := tc["function"].(map[string]any); ok {
				if args, ok := fn["arguments"].(string); ok && args != "" {
					events = append(events, map[string]any{
						"type":  "content_block_delta",
						"index": st.index,
						"delta": map[string]any{"type": "input_json_delta", "partial_json": args},
					})
				}
			}
		}
	}

	return events
}

// closeOpenBlocks emits content_block_stop for every open block in order and
// resets the open-block state (called when switching block kinds or on finish).
func (state *ClaudeStreamState) closeOpenBlocks() []map[string]any {
	var events []map[string]any
	for _, b := range state.openBlocks {
		events = append(events, map[string]any{"type": "content_block_stop", "index": b.index})
	}
	state.openBlocks = nil
	state.textOpen = false
	state.thinkingOpen = false
	state.toolByIndex = nil
	return events
}

// terminalEvents emits the Anthropic end-of-stream sequence:
// content_block_stop(s) → message_delta (stop_reason + usage) → message_stop.
func (state *ClaudeStreamState) terminalEvents() []map[string]any {
	events := state.closeOpenBlocks()
	output := streamInt(state.usage["completion_tokens"])
	events = append(events, map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   claudeStopReason(state.finishReason),
			"stop_sequence": nil,
		},
		"usage": map[string]any{"output_tokens": output},
	})
	events = append(events, map[string]any{"type": "message_stop"})
	return events
}

// ForceFinishEvents terminalizes an unterminated stream (clean upstream EOF
// without a finish/usage chunk) so an Anthropic client never hangs waiting
// for message_stop. No-op once the stream already reached message_stop.
func (state *ClaudeStreamState) ForceFinishEvents() []map[string]any {
	if state.Done {
		return nil
	}
	if state.finishReason == "" {
		state.finishReason = "stop"
	}
	events := state.terminalEvents()
	state.Done = true
	return events
}

// streamInt tolerates JSON float64 and native Go integer types.
func streamInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	}
	return 0
}

// claudeStopReason maps OpenAI finish_reason to Anthropic stop_reason.
func claudeStopReason(finish string) string {
	switch finish {
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "refusal"
	default:
		return "end_turn"
	}
}
