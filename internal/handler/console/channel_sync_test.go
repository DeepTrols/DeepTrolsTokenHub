package console

import (
	"testing"

	"github.com/google/uuid"
)

func TestClassifyPreview(t *testing.T) {
	bindings := map[string]bindingView{
		"gpt-4o":   {ModelID: uuid.New(), Code: "gpt-4o", Enabled: true, Upstream: "gpt-4o"},
		"claude-x": {ModelID: uuid.New(), Code: "claude-x", Enabled: false, Upstream: "claude-x"},
	}
	items := classifyPreview([]string{"gpt-4o", "claude-x", "new-model", "gpt-4o"}, bindings)
	if len(items) != 3 {
		t.Fatalf("expected 3 deduped items, got %d", len(items))
	}
	status := map[string]string{}
	for _, it := range items {
		status[it.Upstream] = it.Status
	}
	if status["gpt-4o"] != "bound" {
		t.Fatalf("gpt-4o should be bound, got %q", status["gpt-4o"])
	}
	if status["claude-x"] != "disabled" {
		t.Fatalf("claude-x should be disabled, got %q", status["claude-x"])
	}
	if status["new-model"] != "new" {
		t.Fatalf("new-model should be new, got %q", status["new-model"])
	}
	for _, it := range items {
		if it.Upstream != "new-model" && it.ModelID == "" {
			t.Fatalf("bound/disabled item %q should carry a model_id", it.Upstream)
		}
	}
}
