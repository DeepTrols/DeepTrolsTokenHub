package gateway

import (
	"testing"

	"github.com/deeptrols/api/internal/domain"
)

func TestFilterByGroup(t *testing.T) {
	mk := func(group string) RouteResult {
		return RouteResult{Channel: &domain.Channel{GroupName: group}}
	}
	candidates := []RouteResult{mk(""), mk("vip"), mk("free")}

	if got := FilterByGroup(candidates, ""); len(got) != 3 {
		t.Fatalf("empty group should keep all, got %d", len(got))
	}
	got := FilterByGroup(candidates, "vip")
	if len(got) != 2 {
		t.Fatalf("vip group should keep 2 (empty+vip), got %d", len(got))
	}
	for _, c := range got {
		if c.Channel.GroupName == "free" {
			t.Fatal("free-group channel should be excluded for vip caller")
		}
	}
	got2 := FilterByGroup(candidates, "nope")
	if len(got2) != 1 || got2[0].Channel.GroupName != "" {
		t.Fatalf("unknown group should keep only ungrouped channel, got %+v", got2)
	}
}
