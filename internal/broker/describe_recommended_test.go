package broker

import (
	"context"
	"testing"

	"github.com/rokasklive/ozy/internal/catalog"
)

func TestDescribeTool_PopulatesRecommendedCall(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	err := store.PutTool(context.Background(), catalog.Tool{
		ToolRef:            "web.search",
		ServerID:           "web",
		DownstreamToolName: "search",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []any{"query", "limit"},
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
				"limit": map[string]any{"type": "integer"},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	res, derr := NewSkeleton(store).DescribeTool(context.Background(), "web.search")
	if derr != nil {
		t.Fatalf("DescribeTool: %v", derr)
	}
	rc := res.RecommendedCall
	if rc == nil || rc.Tool != "callTool" {
		t.Fatalf("recommendedCall missing or wrong tool: %+v", rc)
	}
	if rc.Arguments["toolRef"] != "web.search" {
		t.Fatalf("recommendedCall.toolRef = %v", rc.Arguments["toolRef"])
	}
	skeleton, _ := rc.Arguments["arguments"].(map[string]any)
	if skeleton["query"] != "<string>" || skeleton["limit"] != "<integer>" {
		t.Fatalf("argument skeleton = %v, want typed placeholders for required fields", skeleton)
	}
}
