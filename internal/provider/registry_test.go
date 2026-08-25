package provider

import (
	"net/url"
	"testing"
)

func TestTemplates_DomesticOnlyAndValid(t *testing.T) {
	seen := map[string]bool{}
	for _, tmpl := range Templates {
		if seen[tmpl.Type] {
			t.Errorf("duplicate provider type %q", tmpl.Type)
		}
		seen[tmpl.Type] = true
		if tmpl.Name == "" || tmpl.BaseURL == "" {
			t.Errorf("provider %q missing name/base_url", tmpl.Type)
		}
		u, err := url.Parse(tmpl.BaseURL)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			t.Errorf("provider %q invalid base_url %q", tmpl.Type, tmpl.BaseURL)
		}
		if len(tmpl.Capabilities) == 0 {
			t.Errorf("provider %q declares no capabilities", tmpl.Type)
		}
		if !tmpl.Supports(CapChat) {
			t.Errorf("provider %q must support chat", tmpl.Type)
		}
	}
	for _, foreign := range []string{"openai", "anthropic", "google", "gemini", "codex", "azure"} {
		if ValidType(foreign) {
			t.Errorf("foreign provider %q must not be in the domestic catalog", foreign)
		}
	}
	if len(Templates) != len(TemplateBaseURLs()) {
		t.Error("TemplateBaseURLs length mismatch")
	}
}

func TestLookupAndCapabilities(t *testing.T) {
	tmpl, ok := Lookup("deepseek")
	if !ok {
		t.Fatal("deepseek template missing")
	}
	if !tmpl.Supports(CapChatStream) || tmpl.Supports(CapVideo) {
		t.Error("deepseek capability set wrong")
	}
	if _, ok := Lookup("openai"); ok {
		t.Error("openai should not resolve")
	}
}
