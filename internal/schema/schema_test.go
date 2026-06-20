package schema

import "testing"

func TestValidate(t *testing.T) {
	schema := map[string]any{
		"type":     "object",
		"required": []any{"channel", "text"},
		"properties": map[string]any{
			"channel": map[string]any{"type": "string"},
			"text":    map[string]any{"type": "string"},
			"count":   map[string]any{"type": "integer"},
			"to":      map[string]any{"type": "array"},
		},
	}
	tests := []struct {
		name        string
		args        map[string]any
		wantProblem bool
	}{
		{"valid", map[string]any{"channel": "#ops", "text": "hi"}, false},
		{"missing required", map[string]any{"channel": "#ops"}, true},
		{"wrong scalar type", map[string]any{"channel": "#ops", "text": 7}, true},
		{"wrong array type", map[string]any{"channel": "#ops", "text": "hi", "to": "x"}, true},
		{"integer ok as float64", map[string]any{"channel": "#ops", "text": "hi", "count": float64(3)}, false},
		{"unknown field allowed", map[string]any{"channel": "#ops", "text": "hi", "extra": true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Validate(schema, tt.args)
			if (len(got) > 0) != tt.wantProblem {
				t.Errorf("Validate(%v) problems = %v, wantProblem = %v", tt.args, got, tt.wantProblem)
			}
		})
	}
}
