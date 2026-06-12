package mcp

import (
	"context"
	"encoding/json"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokask/ozy/internal/broker"
	"github.com/rokask/ozy/internal/catalog"
)

func connect(t *testing.T) *mcpsdk.ClientSession {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	adapter := New(broker.NewSkeleton(catalog.NewMemory()), "test")

	go func() { _ = adapter.Server().Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func textPayload(t *testing.T, res *mcpsdk.CallToolResult) map[string]any {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result had no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *TextContent", res.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &payload); err != nil {
		t.Fatalf("content is not valid JSON: %v", err)
	}
	return payload
}

func TestAdapter_AdvertisesExactlyThreeTools(t *testing.T) {
	cs := connect(t)
	list, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	got := make(map[string]bool)
	for _, tool := range list.Tools {
		got[tool.Name] = true
	}
	want := []string{"findTool", "describeTool", "callTool"}
	if len(list.Tools) != len(want) {
		t.Errorf("advertised %d tools, want %d: %v", len(list.Tools), len(want), got)
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}
}

func TestAdapter_FindToolReturnsCatalogEmptyShape(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "findTool",
		Arguments: map[string]any{"query": "search confluence"},
	})
	if err != nil {
		t.Fatalf("CallTool(findTool): %v", err)
	}
	payload := textPayload(t, res)
	if payload["decision"] != "catalog_empty" {
		t.Errorf("decision = %v, want catalog_empty", payload["decision"])
	}
	if payload["agentInstruction"] == "" || payload["agentInstruction"] == nil {
		t.Error("findTool response must include an agentInstruction")
	}
}

func TestAdapter_CallToolReturnsStructuredFailure(t *testing.T) {
	cs := connect(t)
	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name:      "callTool",
		Arguments: map[string]any{"toolRef": "atlassian.confluence_search", "arguments": map[string]any{}},
	})
	if err != nil {
		t.Fatalf("CallTool(callTool): %v", err)
	}
	if !res.IsError {
		t.Error("callTool failure should set IsError")
	}
	payload := textPayload(t, res)
	if payload["ok"] != false {
		t.Errorf("ok = %v, want false", payload["ok"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want object", payload["error"])
	}
	if errObj["type"] == "" || errObj["type"] == nil {
		t.Error("failure must carry an error.type")
	}
	if errObj["agentInstruction"] == "" || errObj["agentInstruction"] == nil {
		t.Error("failure must carry an agentInstruction")
	}
}
