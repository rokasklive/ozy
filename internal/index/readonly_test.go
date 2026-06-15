package index

import (
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNormalizeToolReadOnly(t *testing.T) {
	cases := []struct {
		name string
		ann  *mcpsdk.ToolAnnotations
		want bool
	}{
		{"readOnlyHint true is read-only", &mcpsdk.ToolAnnotations{ReadOnlyHint: true}, true},
		{"readOnlyHint false is not read-only", &mcpsdk.ToolAnnotations{ReadOnlyHint: false}, false},
		{"absent annotations default to not read-only", nil, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTool("srv", &mcpsdk.Tool{Name: "t", Annotations: tt.ann}, time.Now())
			if err != nil {
				t.Fatalf("normalizeTool: %v", err)
			}
			if got.ReadOnly != tt.want {
				t.Errorf("ReadOnly = %v, want %v", got.ReadOnly, tt.want)
			}
		})
	}
}
