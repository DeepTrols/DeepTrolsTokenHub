package relayconvert

import (
	"encoding/json"
	"reflect"
	"testing"
)

func respEvtType(evt map[string]any) string {
	s, _ := evt["type"].(string)
	return s
}

// startResponsesStream feeds the role-only first chunk (response.created) so
// the following assertions exercise output-item conversion only.
func startResponsesStream(state *ResponsesStreamState) {
	OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"id":      "chatcmpl-r1",
		"model":   "deepseek-chat",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil}},
	}), state)
}

func TestOpenAIStreamChunkToResponsesEvents_Created(t *testing.T) {
	state := &ResponsesStreamState{}
	events := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"id":      "chatcmpl-r1",
		"model":   "deepseek-chat",
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": ""}, "finish_reason": nil}},
	}), state)
	if len(events) != 1 || respEvtType(events[0]) != "response.created" {
		t.Fatalf("expected single response.created, got %v", events)
	}
	env, _ := events[0]["response"].(map[string]any)
	if env["id"] != "chatcmpl-r1" || env["model"] != "deepseek-chat" || env["status"] != "in_progress" {
		t.Fatalf("unexpected response envelope: %v", env)
	}
	if state.Done {
		t.Fatal("state should not be done after created")
	}
}

func TestOpenAIStreamChunkToResponsesEvents_TextLifecycle(t *testing.T) {
	state := &ResponsesStreamState{}
	startResponsesStream(state)

	first := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "Hello"}, "finish_reason": nil}},
	}), state)
	types := []string{}
	for _, e := range first {
		types = append(types, respEvtType(e))
	}
	if !reflect.DeepEqual(types, []string{"response.output_item.added", "response.content_part.added", "response.output_text.delta"}) {
		t.Fatalf("unexpected first-text events: %v", types)
	}
	if streamInt(first[2]["delta"]) == 0 && first[2]["delta"] != "Hello" {
		t.Fatalf("expected text delta Hello, got %v", first[2]["delta"])
	}

	next := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": " world"}, "finish_reason": nil}},
	}), state)
	if len(next) != 1 || respEvtType(next[0]) != "response.output_text.delta" || next[0]["delta"] != " world" {
		t.Fatalf("expected single text delta, got %v", next)
	}
}

func TestOpenAIStreamChunkToResponsesEvents_FinishWithUsage(t *testing.T) {
	state := &ResponsesStreamState{}
	startResponsesStream(state)
	OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "hi"}, "finish_reason": nil}},
	}), state)

	events := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	}), state)
	types := []string{}
	for _, e := range events {
		types = append(types, respEvtType(e))
	}
	want := []string{"response.output_text.done", "response.content_part.done", "response.output_item.done", "response.completed"}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("unexpected terminal events: %v", types)
	}
	completed := events[len(events)-1]["response"].(map[string]any)
	if completed["status"] != "completed" {
		t.Fatalf("status: %v", completed["status"])
	}
	usage, _ := completed["usage"].(map[string]any)
	if streamInt(usage["input_tokens"]) != 10 || streamInt(usage["output_tokens"]) != 5 {
		t.Fatalf("usage: %v", usage)
	}
	if !state.Done {
		t.Fatal("state should be done")
	}
}

func TestOpenAIStreamChunkToResponsesEvents_DeferredFinish(t *testing.T) {
	state := &ResponsesStreamState{}
	startResponsesStream(state)
	OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": "hi"}, "finish_reason": nil}},
	}), state)

	finish := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "length"}},
	}), state)
	if len(finish) != 0 {
		t.Fatalf("terminal events must be deferred without usage, got %v", finish)
	}

	usage := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{},
		"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 4, "total_tokens": 7},
	}), state)
	completed := usage[len(usage)-1]["response"].(map[string]any)
	if completed["status"] != "incomplete" {
		t.Fatalf("length finish should produce incomplete status, got %v", completed["status"])
	}
	if !state.Done {
		t.Fatal("state should be done")
	}
}

func TestOpenAIStreamChunkToResponsesEvents_Reasoning(t *testing.T) {
	state := &ResponsesStreamState{}
	startResponsesStream(state)

	events := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"reasoning_content": "let me think"}, "finish_reason": nil}},
	}), state)
	if len(events) != 2 || respEvtType(events[0]) != "response.output_item.added" || respEvtType(events[1]) != "response.reasoning_summary.delta" {
		t.Fatalf("expected reasoning item + summary delta, got %v", events)
	}
	item, _ := events[0]["item"].(map[string]any)
	if item["type"] != "reasoning" {
		t.Fatalf("expected reasoning item, got %v", item)
	}
}

func TestOpenAIStreamChunkToResponsesEvents_ToolCalls(t *testing.T) {
	state := &ResponsesStreamState{}
	startResponsesStream(state)

	start := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
			"tool_calls": []any{map[string]any{
				"index": 0, "id": "call_1", "type": "function",
				"function": map[string]any{"name": "get_weather", "arguments": ""},
			}},
		}, "finish_reason": nil}},
	}), state)
	if len(start) != 1 || respEvtType(start[0]) != "response.output_item.added" {
		t.Fatalf("expected function_call item, got %v", start)
	}
	item, _ := start[0]["item"].(map[string]any)
	if item["type"] != "function_call" || item["name"] != "get_weather" || item["call_id"] != "call_1" {
		t.Fatalf("unexpected function_call item: %v", item)
	}

	args := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
			"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": `{"city":"Shanghai"}`}}},
		}, "finish_reason": nil}},
	}), state)
	if len(args) != 1 || respEvtType(args[0]) != "response.function_call_arguments.delta" {
		t.Fatalf("expected function_call_arguments.delta, got %v", args)
	}

	finish := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{
		"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "tool_calls"}},
		"usage":   map[string]any{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
	}), state)
	types := []string{}
	for _, e := range finish {
		types = append(types, respEvtType(e))
	}
	want := []string{"response.function_call_arguments.done", "response.output_item.done", "response.completed"}
	if !reflect.DeepEqual(types, want) {
		t.Fatalf("unexpected tool terminal events: %v", types)
	}
}

func TestOpenAIStreamChunkToResponsesEvents_AfterDone(t *testing.T) {
	state := &ResponsesStreamState{Done: true}
	if events := OpenAIStreamChunkToResponsesEvents(oaiChunk(map[string]any{"choices": []any{}}), state); len(events) != 0 {
		t.Fatalf("expected no events after done, got %v", events)
	}
}

func TestOpenAIStreamChunkToResponsesEvents_JSONSerializable(t *testing.T) {
	state := &ResponsesStreamState{}
	chunks := []map[string]any{
		{"id": "chatcmpl-r1", "model": "deepseek-chat", "choices": []any{map[string]any{"index": 0, "delta": map[string]any{"role": "assistant", "content": "Hi"}, "finish_reason": nil}}},
		{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 2, "completion_tokens": 1, "total_tokens": 3}},
	}
	for _, c := range chunks {
		for _, evt := range OpenAIStreamChunkToResponsesEvents(c, state) {
			if _, err := json.Marshal(evt); err != nil {
				t.Fatalf("event not JSON serializable: %v (%v)", evt, err)
			}
		}
	}
}
