package relayconvert

import "testing"

func TestClaudeStreamToOpenAIEvent(t *testing.T) {
	start := map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id": "msg_1", "model": "claude-sonnet-4-5", "role": "assistant",
			"usage": map[string]any{"input_tokens": 12},
		},
	}
	chunk, ok, done := ClaudeStreamToOpenAIEvent(start)
	if !ok || done {
		t.Fatalf("start: ok=%v done=%v", ok, done)
	}
	choices := chunk["choices"].([]map[string]any)
	if choices[0]["delta"].(openAIStreamDelta).Role != "assistant" {
		t.Fatalf("expected assistant role")
	}
	if numOrZero(chunk["usage"].(map[string]any)["prompt_tokens"]) != 12 {
		t.Fatalf("usage not mapped: %+v", chunk["usage"])
	}

	delta := map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": "Hi"}}
	chunk, ok, _ = ClaudeStreamToOpenAIEvent(delta)
	if !ok || chunk["choices"].([]map[string]any)[0]["delta"].(openAIStreamDelta).Content != "Hi" {
		t.Fatalf("text delta not mapped")
	}

	md := map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn"}, "usage": map[string]any{"output_tokens": 3}}
	chunk, ok, _ = ClaudeStreamToOpenAIEvent(md)
	if !ok {
		t.Fatal("message_delta should produce a chunk")
	}
	fr := chunk["choices"].([]map[string]any)[0]["finish_reason"].(*string)
	if *fr != "stop" {
		t.Fatalf("finish_reason: %s", *fr)
	}

	_, ok, done = ClaudeStreamToOpenAIEvent(map[string]any{"type": "message_stop"})
	if ok || !done {
		t.Fatalf("message_stop should be done")
	}
}
