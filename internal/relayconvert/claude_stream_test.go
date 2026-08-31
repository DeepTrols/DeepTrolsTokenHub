package relayconvert

import (
	"encoding/json"
	"reflect"
	"testing"
)

func oaiChunk(m map[string]any) map[string]any {
	return m
}

func evtType(evt map[string]any) string {
	s, _ := evt["type"].(string)
	return s
}

// startClaudeStream feeds the role-only first chunk (message_start) so the
// following assertions exercise content-block conversion only.
func startClaudeStream(state *ClaudeStreamState) {
	OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"id":      "chatcmpl-1",
		"model":   "deepseek-chat",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil}},
	}), state)
}

// TestOpenAIStreamChunkToClaudeEvents_MessageStart covers the first chunk of
// an OpenAI chat.completion.chunk stream: only message_start is emitted.
func TestOpenAIStreamChunkToClaudeEvents_MessageStart(t *testing.T) {
	state := &ClaudeStreamState{}
	events := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"id":    "chatcmpl-1",
		"model": "deepseek-chat",
		"choices": []any{
			map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil},
		},
	}), state)

	if len(events) != 1 || evtType(events[0]) != "message_start" {
		t.Fatalf("expected single message_start, got %v", events)
	}
	msg, _ := events[0]["message"].(map[string]any)
	if msg["id"] != "chatcmpl-1" || msg["model"] != "deepseek-chat" || msg["role"] != "assistant" {
		t.Fatalf("unexpected message envelope: %v", msg)
	}
	if state.Done {
		t.Fatal("state should not be done after message_start")
	}
}

// TestOpenAIStreamChunkToClaudeEvents_TextBlocks verifies the text block
// lifecycle: start + first delta, then subsequent deltas only.
func TestOpenAIStreamChunkToClaudeEvents_TextBlocks(t *testing.T) {
	state := &ClaudeStreamState{}
	startClaudeStream(state)

	first := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "Hello"}, "finish_reason": nil}},
	}), state)
	if len(first) != 2 {
		t.Fatalf("expected content_block_start + delta, got %v", first)
	}
	if evtType(first[0]) != "content_block_start" {
		t.Fatalf("expected content_block_start first, got %s", evtType(first[0]))
	}
	block, _ := first[0]["content_block"].(map[string]any)
	if block["type"] != "text" {
		t.Fatalf("expected text block, got %v", block)
	}
	if streamInt(first[0]["index"]) != 0 {
		t.Fatalf("expected index 0, got %v", first[0]["index"])
	}
	if evtType(first[1]) != "content_block_delta" {
		t.Fatalf("expected content_block_delta second, got %s", evtType(first[1]))
	}
	delta, _ := first[1]["delta"].(map[string]any)
	if delta["type"] != "text_delta" || delta["text"] != "Hello" {
		t.Fatalf("unexpected text delta: %v", delta)
	}

	second := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": " world"}, "finish_reason": nil}},
	}), state)
	if len(second) != 1 || evtType(second[0]) != "content_block_delta" {
		t.Fatalf("expected single delta for continuation, got %v", second)
	}
}

// TestOpenAIStreamChunkToClaudeEvents_FinishWithUsage verifies the terminal
// sequence: content_block_stop, message_delta (stop_reason + usage), message_stop.
func TestOpenAIStreamChunkToClaudeEvents_FinishWithUsage(t *testing.T) {
	state := &ClaudeStreamState{}
	startClaudeStream(state)
	OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "hi"}, "finish_reason": nil}},
	}), state)

	events := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	}), state)

	if len(events) != 3 {
		t.Fatalf("expected 3 terminal events, got %v", events)
	}
	if evtType(events[0]) != "content_block_stop" {
		t.Fatalf("expected content_block_stop first, got %s", evtType(events[0]))
	}
	if evtType(events[1]) != "message_delta" {
		t.Fatalf("expected message_delta second, got %s", evtType(events[1]))
	}
	mdDelta, _ := events[1]["delta"].(map[string]any)
	if mdDelta["stop_reason"] != "end_turn" {
		t.Fatalf("expected stop_reason end_turn, got %v", mdDelta)
	}
	usage, _ := events[1]["usage"].(map[string]any)
	if streamInt(usage["output_tokens"]) != 5 {
		t.Fatalf("expected output_tokens 5, got %v", usage)
	}
	if evtType(events[2]) != "message_stop" {
		t.Fatalf("expected message_stop third, got %s", evtType(events[2]))
	}
	if !state.Done {
		t.Fatal("state should be done after message_stop")
	}
}

// TestOpenAIStreamChunkToClaudeEvents_DeferredFinish verifies finish_reason
// without usage defers terminal events until the usage-only chunk arrives.
func TestOpenAIStreamChunkToClaudeEvents_DeferredFinish(t *testing.T) {
	state := &ClaudeStreamState{}
	startClaudeStream(state)
	OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "hi"}, "finish_reason": nil}},
	}), state)

	finish := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "length"}},
	}), state)
	if len(finish) != 0 {
		t.Fatalf("terminal events must be deferred without usage, got %v", finish)
	}

	usageOnly := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
	}), state)
	if len(usageOnly) != 3 || evtType(usageOnly[2]) != "message_stop" {
		t.Fatalf("expected terminal events on usage chunk, got %v", usageOnly)
	}
	mdDelta, _ := usageOnly[1]["delta"].(map[string]any)
	if mdDelta["stop_reason"] != "max_tokens" {
		t.Fatalf("expected stop_reason max_tokens for length, got %v", mdDelta)
	}
	if !state.Done {
		t.Fatal("state should be done")
	}
}

// TestOpenAIStreamChunkToClaudeEvents_ToolCalls verifies tool_use block start
// plus input_json_delta fragments and a tool_use stop reason.
func TestOpenAIStreamChunkToClaudeEvents_ToolCalls(t *testing.T) {
	state := &ClaudeStreamState{}
	startClaudeStream(state)

	start := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
			"tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": "get_weather", "arguments": ""},
			}},
		}, "finish_reason": nil}},
	}), state)
	if len(start) != 1 || evtType(start[0]) != "content_block_start" {
		t.Fatalf("expected tool content_block_start, got %v", start)
	}
	block, _ := start[0]["content_block"].(map[string]any)
	if block["type"] != "tool_use" || block["id"] != "call_1" || block["name"] != "get_weather" {
		t.Fatalf("unexpected tool_use block: %v", block)
	}

	args := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
			"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": `{"city":"Shanghai"}`}}},
		}, "finish_reason": nil}},
	}), state)
	if len(args) != 1 || evtType(args[0]) != "content_block_delta" {
		t.Fatalf("expected input_json_delta, got %v", args)
	}
	delta, _ := args[0]["delta"].(map[string]any)
	if delta["type"] != "input_json_delta" {
		t.Fatalf("expected input_json_delta, got %v", delta)
	}

	finish := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
		"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
	}), state)
	if len(finish) != 3 || evtType(finish[0]) != "content_block_stop" {
		t.Fatalf("expected tool block stop + message_delta + message_stop, got %v", finish)
	}
	mdDelta, _ := finish[1]["delta"].(map[string]any)
	if mdDelta["stop_reason"] != "tool_use" {
		t.Fatalf("expected stop_reason tool_use, got %v", mdDelta)
	}
}

// TestOpenAIStreamChunkToClaudeEvents_Thinking verifies reasoning_content maps
// to a thinking block before the text block.
func TestOpenAIStreamChunkToClaudeEvents_Thinking(t *testing.T) {
	state := &ClaudeStreamState{}
	startClaudeStream(state)

	events := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"reasoning_content": "let me think"}, "finish_reason": nil}},
	}), state)
	if len(events) != 2 || evtType(events[0]) != "content_block_start" {
		t.Fatalf("expected thinking block start, got %v", events)
	}
	block, _ := events[0]["content_block"].(map[string]any)
	if block["type"] != "thinking" {
		t.Fatalf("expected thinking block, got %v", block)
	}

	// A subsequent text delta must stop the thinking block and start text.
	text := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "answer"}, "finish_reason": nil}},
	}), state)
	types := []string{}
	for _, e := range text {
		types = append(types, evtType(e))
	}
	if !reflect.DeepEqual(types, []string{"content_block_stop", "content_block_start", "content_block_delta"}) {
		t.Fatalf("unexpected transition after thinking: %v", types)
	}
}

// TestOpenAIStreamChunkToClaudeEvents_AfterDone verifies no events are emitted
// once the stream is terminal.
func TestOpenAIStreamChunkToClaudeEvents_AfterDone(t *testing.T) {
	state := &ClaudeStreamState{Done: true}
	if events := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{"choices": []any{}}), state); len(events) != 0 {
		t.Fatalf("expected no events after done, got %v", events)
	}
}

// TestOpenAIStreamChunkToClaudeEvents_FirstChunkWithContent covers the case
// where a first chunk already carries text: message_start plus the content
// block start and first delta are emitted in the same call.
func TestOpenAIStreamChunkToClaudeEvents_FirstChunkWithContent(t *testing.T) {
	state := &ClaudeStreamState{}
	events := OpenAIStreamChunkToClaudeEvents(oaiChunk(map[string]any{
		"id":      "chatcmpl-1",
		"model":   "deepseek-chat",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "Hello"}, "finish_reason": nil}},
	}), state)
	if len(events) != 3 {
		t.Fatalf("expected message_start + start + delta, got %v", events)
	}
	if evtType(events[0]) != "message_start" || evtType(events[1]) != "content_block_start" || evtType(events[2]) != "content_block_delta" {
		t.Fatalf("unexpected event order: %v", events)
	}
}

// TestOpenAIStreamChunkToClaudeEvents_JSONSerializable ensures every event
// survives a JSON round-trip (gateway writes them as SSE data).
func TestOpenAIStreamChunkToClaudeEvents_JSONSerializable(t *testing.T) {
	state := &ClaudeStreamState{}
	chunks := []map[string]any{
		{"id": "chatcmpl-1", "model": "deepseek-chat", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "Hi"}, "finish_reason": nil}}},
		{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3}},
	}
	for _, c := range chunks {
		for _, evt := range OpenAIStreamChunkToClaudeEvents(c, state) {
			if _, err := json.Marshal(evt); err != nil {
				t.Fatalf("event not JSON serializable: %v (%v)", evt, err)
			}
		}
	}
}
