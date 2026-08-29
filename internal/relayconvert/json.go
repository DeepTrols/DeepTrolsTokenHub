package relayconvert

import (
	"encoding/json"
)

func parseJSON(s string) any {
	if s == "" {
		return nil
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil
	}
	return v
}

func stringifyJSON(v any) string {
	if v == nil {
		return ""
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func openAIItemToClaudeBlock(item any) ClaudeContentBlock {
	if m, ok := item.(map[string]any); ok {
		switch typ, _ := m["type"].(string); typ {
		case "text":
			txt, _ := m["text"].(string)
			return ClaudeContentBlock{Type: "text", Text: txt}
		case "image_url":
			if img, ok := m["image_url"].(map[string]any); ok {
				if url, _ := img["url"].(string); url != "" {
					return ClaudeContentBlock{Type: "image", Source: &ClaudeImageSource{Type: "base64", MediaType: "image/png", Data: url}}
				}
			}
		}
	}
	return ClaudeContentBlock{Type: "text"}
}
