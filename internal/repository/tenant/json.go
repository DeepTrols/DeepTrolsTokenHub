package tenant

import (
	"encoding/json"
	"strings"
)

// marshalJSONB marshals a map to JSON bytes for JSONB columns.
func marshalJSONB(v map[string]any) []byte {
	if v == nil {
		return []byte("{}")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return []byte("{}")
	}
	return b
}

// unmarshalJSONB unmarshals a JSON string into a map.
func unmarshalJSONB(raw string) map[string]any {
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
