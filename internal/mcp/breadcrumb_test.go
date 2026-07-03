package mcp

import (
	"context"
	"fmt"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
)

func TestBreadcrumb(t *testing.T) {
	if got := Breadcrumb(nil); got != "" {
		t.Errorf("Breadcrumb(nil) = %q, want empty", got)
	}
	if got := Breadcrumb([]string{"github", "atlassian"}); !strings.Contains(got, "atlassian, github") {
		t.Errorf("Breadcrumb should list servers sorted, got %q", got)
	}
	many := make([]string, MaxBreadcrumbServers+3)
	for i := range many {
		many[i] = fmt.Sprintf("srv%02d", i)
	}
	got := Breadcrumb(many)
	if !strings.Contains(got, "+3 more") {
		t.Errorf("Breadcrumb should indicate overflow beyond the cap, got %q", got)
	}
}

func TestAdapter_FindDescriptionIncludesBreadcrumb(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	adapter := New(StaticProvider(broker.NewSkeleton(catalog.NewMemory())), "test", Breadcrumb([]string{"github", "atlassian"}))
	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	var findDesc string
	for _, tool := range list.Tools {
		if tool.Name == "findTool" {
			findDesc = tool.Description
		}
	}
	if !strings.Contains(findDesc, "atlassian") || !strings.Contains(findDesc, "github") {
		t.Errorf("findTool description should include the breadcrumb servers, got %q", findDesc)
	}
	if !strings.Contains(findDesc, "semantic") {
		t.Error("findTool description should retain the base description text")
	}
}

func TestAdapter_FindToolResponseCarriedOnceInContent(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "anything"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	if res.StructuredContent != nil {
		t.Errorf("findTool StructuredContent = %v, want nil (single representation in content)", res.StructuredContent)
	}
	_ = textPayload(t, res) // content must still be present and valid JSON
}
