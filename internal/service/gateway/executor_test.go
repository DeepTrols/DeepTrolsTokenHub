package gateway

import "testing"

func TestCustomHeadersFromConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]any
		want map[string]string
	}{
		{
			name: "nil config yields nil headers",
			cfg:  nil,
			want: nil,
		},
		{
			name: "missing custom_headers yields nil",
			cfg:  map[string]any{"api_key": "sk-1"},
			want: nil,
		},
		{
			name: "empty custom_headers yields nil",
			cfg:  map[string]any{"custom_headers": map[string]any{}},
			want: nil,
		},
		{
			name: "string values are extracted and whitespace keys trimmed",
			cfg: map[string]any{
				"custom_headers": map[string]any{
					" X-Custom-Header ": "v1",
					"X-Provider-Id":     "gw-1",
					"X-Ignored":         42,
				},
			},
			want: map[string]string{"X-Custom-Header": "v1", "X-Provider-Id": "gw-1"},
		},
		{
			name: "non-object custom_headers yields nil",
			cfg:  map[string]any{"custom_headers": "not-an-object"},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CustomHeadersFromConfig(tt.cfg)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("headers[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}
