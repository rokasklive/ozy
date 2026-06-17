package broker

import (
	"context"
	"errors"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/search"
)

// seededBroker builds a live broker over a memory catalog pre-populated with the
// given tools, so callTool argument validation has a cataloged schema to check.
func seededBroker(t *testing.T, cfg *config.Config, conn Connector, tools ...catalog.Tool) Broker {
	t.Helper()
	store := catalog.NewMemory()
	for _, tl := range tools {
		if err := store.PutTool(context.Background(), tl); err != nil {
			t.Fatalf("PutTool: %v", err)
		}
	}
	return NewLive(store, cfg, conn, search.New(store, nil))
}

var searchToolSchema = map[string]any{
	"type":       "object",
	"required":   []any{"q"},
	"properties": map[string]any{"q": map[string]any{"type": "string"}},
}

func searchTool() catalog.Tool {
	return catalog.Tool{ToolRef: "atlassian.confluence_search", ServerID: "atlassian", InputSchema: searchToolSchema}
}

func enabledAtlassian() *config.Config {
	return &config.Config{MCP: map[string]config.ServerConfig{
		"atlassian": {Type: "remote", URL: "memory", Enabled: true},
	}}
}

func TestCallTool_MissingRequiredArgFailsValidationBeforeDownstream(t *testing.T) {
	t.Parallel()
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: &callSession{}}}
	b := seededBroker(t, enabledAtlassian(), connector, searchTool())

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeArgumentValidationFailed {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeArgumentValidationFailed)
	}
	if ce.Retryable {
		t.Error("validation failure must not be retryable for the same arguments")
	}
	if !strings.Contains(strings.ToLower(ce.AgentInstruction), "describetool") {
		t.Errorf("agentInstruction should point at describeTool, got %q", ce.AgentInstruction)
	}
	if ids, _ := connector.connectedIDs.Load().([]string); len(ids) != 0 {
		t.Errorf("validation should reject before any downstream connect, but connected to %v", ids)
	}
}

func TestCallTool_WrongTypeArgFailsValidationBeforeDownstream(t *testing.T) {
	t.Parallel()
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: &callSession{}}}
	b := seededBroker(t, enabledAtlassian(), connector, searchTool())

	_, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{"q": 7})
	var ce *contract.Error
	if !errors.As(err, &ce) || ce.Type != contract.ErrTypeArgumentValidationFailed {
		t.Fatalf("want ARGUMENT_VALIDATION_FAILED, got %v", err)
	}
	if ids, _ := connector.connectedIDs.Load().([]string); len(ids) != 0 {
		t.Errorf("wrong-type arg should reject before downstream connect, connected to %v", ids)
	}
}

func TestCallTool_ValidArgsPassValidationAndReachDownstream(t *testing.T) {
	t.Parallel()
	session := &callSession{res: &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	b := seededBroker(t, enabledAtlassian(), connector, searchTool())

	res, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{"q": "billing"})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v", err)
	}
	if !res.OK {
		t.Error("valid args should pass validation and invoke downstream")
	}
	if ids, _ := connector.connectedIDs.Load().([]string); len(ids) != 1 {
		t.Errorf("valid call should reach downstream once, connected to %v", ids)
	}
}

func TestCallTool_NoCatalogedSchemaSkipsValidation(t *testing.T) {
	t.Parallel()
	session := &callSession{res: &mcpsdk.CallToolResult{Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}}}}
	connector := &callConnector{connectResult: downstream.Result{ServerID: "atlassian", Session: session}}
	// No tool seeded → no cataloged schema → validation skipped (index-free invocation).
	b := seededBroker(t, enabledAtlassian(), connector)

	res, err := b.CallTool(context.Background(), "atlassian.confluence_search", map[string]any{})
	if err != nil {
		t.Fatalf("CallTool() unexpected error = %v (validation must be skipped without a cataloged schema)", err)
	}
	if !res.OK {
		t.Error("missing cataloged schema should skip validation and invoke downstream")
	}
}
