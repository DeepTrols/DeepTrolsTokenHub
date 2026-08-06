// Package jsonb provides shared helpers for marshaling/unmarshaling JSONB columns.
package jsonb

import (
	"encoding/json"
	"strings"
)

// Marshal converts a map to JSON bytes for JSONB columns.
// Returns {"}" for nil or marshal errors.
func Marshal(v map[string]any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// Unmarshal converts a JSON string back to a map.
// Returns an empty map for empty/null/invalid input.
func Unmarshal(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" || raw == "null" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return map[string]any{}
	}
	return m
}
