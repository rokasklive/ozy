package index

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

type fakeConnector struct {
	results []downstream.Result
}

func (f fakeConnector) ConnectAll(context.Context, *config.Config) []downstream.Result {
	return f.results
}

type fakeSession struct {
	tools []*mcpsdk.Tool
	err   error
}

func (f fakeSession) ListTools(context.Context, *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	return &mcpsdk.ListToolsResult{Tools: f.tools}, f.err
}

func (fakeSession) CallTool(context.Context, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	return &mcpsdk.CallToolResult{}, nil
}

func (fakeSession) Close() error { return nil }

type blockingSession struct{}

func (blockingSession) ListTools(ctx context.Context, _ *mcpsdk.ListToolsParams) (*mcpsdk.ListToolsResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingSession) CallTool(ctx context.Context, _ *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (blockingSession) Close() error { return nil }

func TestIndexer_NormalizesDiscoveredTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	indexer := New(store, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session: fakeSession{tools: []*mcpsdk.Tool{{
				Name:        "confluence_search",
				Title:       "Confluence Search",
				Description: "Search Confluence",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
				},
			}}},
		},
	}}, WithClock(func() time.Time { return now }))

	summary := indexer.Run(ctx, &config.Config{})
	if summary.ServersReached != 1 || summary.ToolsIndexed != 1 || !summary.OK {
		t.Fatalf("summary = %+v, want one reached server and one indexed tool", summary)
	}
	tool, ok, err := store.GetTool(ctx, "atlassian.confluence_search")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if !ok {
		t.Fatal("indexed tool not found")
	}
	if tool.ToolRef != "atlassian.confluence_search" ||
		tool.ServerID != "atlassian" ||
		tool.DownstreamToolName != "confluence_search" ||
		tool.Title != "Confluence Search" ||
		tool.Freshness != catalog.FreshnessFresh ||
		tool.ServerStatus != catalog.ServerOnline ||
		!tool.CallableNow ||
		!tool.LastIndexedAt.Equal(now) ||
		tool.SchemaHash == "" {
		t.Fatalf("indexed tool = %+v, want normalized metadata", tool)
	}
	if got := tool.InputSchema["type"]; got != "object" {
		t.Fatalf("InputSchema[type] = %v, want object", got)
	}
}

func TestIndexer_PartialFailureDoesNotAbortReachableServers(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	indexer := New(store, fakeConnector{results: []downstream.Result{
		{
			ServerID: "bad",
			Error: &contract.Error{
				Type:     contract.ErrTypeDownstreamServerOffline,
				ServerID: "bad",
				Message:  "offline",
			},
		},
		{
			ServerID: "good",
			Session:  fakeSession{tools: []*mcpsdk.Tool{{Name: "search", InputSchema: map[string]any{"type": "object"}}}},
		},
	}})

	summary := indexer.Run(ctx, &config.Config{})
	if !summary.OK {
		t.Fatalf("summary.OK = false, want true when at least one server is reachable")
	}
	if summary.ServersReached != 1 || summary.ServersFailed != 1 || summary.ToolsIndexed != 1 {
		t.Fatalf("summary = %+v, want one reached, one failed, one tool", summary)
	}
	if len(summary.Errors) != 1 || summary.Errors[0].ServerID != "bad" {
		t.Fatalf("summary errors = %+v, want bad server error", summary.Errors)
	}
}

func TestIndexer_NoReachableServersIsInstructional(t *testing.T) {
	t.Parallel()
	indexer := New(catalog.NewMemory(), fakeConnector{results: []downstream.Result{
		{
			ServerID: "bad",
			Error: &contract.Error{
				Type:     contract.ErrTypeDownstreamServerOffline,
				ServerID: "bad",
				Message:  "offline",
			},
		},
	}})

	summary := indexer.Run(context.Background(), &config.Config{})
	if summary.OK {
		t.Fatal("summary.OK = true, want false when no server is reachable")
	}
	if summary.AgentInstruction == "" {
		t.Fatal("no-reachable-server summary must be instructional")
	}
}

func TestIndexer_PerServerTimeoutCancelsListTools(t *testing.T) {
	t.Parallel()
	indexer := New(catalog.NewMemory(), fakeConnector{results: []downstream.Result{
		{
			ServerID: "slow",
			Session:  blockingSession{},
		},
	}})

	summary := indexer.Run(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"slow": {Type: "remote", URL: "memory", Enabled: true, Timeout: 1},
		},
	})

	if summary.OK {
		t.Fatal("summary.OK = true, want false when only server times out")
	}
	if len(summary.Errors) != 1 || summary.Errors[0].Type != contract.ErrTypeDownstreamCallFailed {
		t.Fatalf("summary errors = %+v, want one downstream call failure", summary.Errors)
	}
	if !strings.Contains(summary.Errors[0].Message, context.DeadlineExceeded.Error()) {
		t.Fatalf("timeout error = %q, want deadline exceeded", summary.Errors[0].Message)
	}
}

func TestIndexer_ListToolsErrorRedactsConfiguredSecrets(t *testing.T) {
	t.Parallel()
	const secret = "supersecretvalue"
	indexer := New(catalog.NewMemory(), fakeConnector{results: []downstream.Result{
		{
			ServerID: "remote",
			Session:  fakeSession{err: errors.New("tools/list failed with " + secret)},
		},
	}})

	summary := indexer.Run(context.Background(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"remote": {
				Type:        "remote",
				URL:         "memory",
				Enabled:     true,
				Headers:     map[string]string{"Authorization": "Bearer " + secret},
				Environment: map[string]string{"TOKEN": secret},
			},
		},
	})

	if len(summary.Errors) != 1 {
		t.Fatalf("summary errors = %+v, want one error", summary.Errors)
	}
	if strings.Contains(summary.Errors[0].Message, secret) {
		t.Fatalf("ListTools error leaked secret: %+v", summary.Errors[0])
	}
}

func TestIndexer_SetsLastIndexedAtOnSuccess(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	indexTime := time.Date(2026, 6, 14, 14, 30, 0, 0, time.UTC)
	indexer := New(store, fakeConnector{results: []downstream.Result{
		{
			ServerID: "atlassian",
			Session: fakeSession{tools: []*mcpsdk.Tool{{
				Name: "confluence_search", InputSchema: map[string]any{"type": "object"},
			}}},
		},
	}}, WithClock(func() time.Time { return indexTime }))

	summary := indexer.Run(ctx, &config.Config{})
	if !summary.OK || summary.ServersReached != 1 {
		t.Fatalf("summary = %+v, want success", summary)
	}

	ts, ok, err := store.LastIndexedAt(ctx)
	if err != nil {
		t.Fatalf("LastIndexedAt() error = %v", err)
	}
	if !ok {
		t.Error("LastIndexedAt() ok = false after successful index, want true")
	}
	if !ts.Equal(indexTime) {
		t.Errorf("LastIndexedAt() = %v, want %v", ts, indexTime)
	}
}

func TestIndexer_DoesNotAdvanceLastIndexedAtOnTotalFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	indexer := New(store, fakeConnector{results: []downstream.Result{
		{
			ServerID: "bad",
			Error: &contract.Error{
				Type: contract.ErrTypeDownstreamServerOffline, ServerID: "bad", Message: "offline",
			},
		},
	}})

	summary := indexer.Run(ctx, &config.Config{})
	if summary.OK || summary.ServersReached != 0 {
		t.Fatalf("summary = %+v, want total failure", summary)
	}

	ts, ok, err := store.LastIndexedAt(ctx)
	if err != nil {
		t.Fatalf("LastIndexedAt() error = %v", err)
	}
	if ok {
		t.Error("LastIndexedAt() ok = true after failed index, want false")
	}
	if !ts.IsZero() {
		t.Errorf("LastIndexedAt() = %v, want zero time", ts)
	}
}

func TestIndexer_SetsLastIndexedAtWhenReachableServerHasZeroTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	indexTime := time.Date(2026, 6, 14, 15, 0, 0, 0, time.UTC)
	indexer := New(store, fakeConnector{results: []downstream.Result{
		{
			ServerID: "empty",
			Session:  fakeSession{tools: nil},
		},
	}}, WithClock(func() time.Time { return indexTime }))

	summary := indexer.Run(ctx, &config.Config{})
	if summary.ServersReached != 1 || summary.ToolsIndexed != 0 {
		t.Fatalf("summary = %+v, want one reachable server with zero tools", summary)
	}

	ts, ok, err := store.LastIndexedAt(ctx)
	if err != nil {
		t.Fatalf("LastIndexedAt() error = %v", err)
	}
	if !ok {
		t.Error("LastIndexedAt() ok = false for reachable server with zero tools, want true")
	}
	if !ts.Equal(indexTime) {
		t.Errorf("LastIndexedAt() = %v, want %v", ts, indexTime)
	}
}
