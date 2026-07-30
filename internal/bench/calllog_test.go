package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// callToolViaServer connects an in-memory client to a fixture toolset and calls
// one tool — exercising the real receiving path (including the invocation-log
// middleware).
func callToolViaServer(t *testing.T, toolset, tool string, args map[string]any) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv, err := newMCPServer(toolset, weatherFixtureDir)
	if err != nil {
		t.Fatalf("newMCPServer: %v", err)
	}
	st, ct := mcpsdk.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "t", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = cs.Close() }()
	if _, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: args}); err != nil {
		t.Fatalf("call %s/%s: %v", toolset, tool, err)
	}
}

// TestCallLogMiddlewareWrites is the regression guard for the invocation log: a
// tool call through a fixture server must append a record naming the downstream
// server and tool to OZY_BENCH_CALL_LOG. (Regression: the middleware receives
// *CallToolParamsRaw, not *CallToolParams — asserting the wrong type silently
// dropped every record and broke required_tools grading.)
func TestCallLogMiddlewareWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls.jsonl")
	t.Setenv(callLogEnv, logPath)

	callToolViaServer(t, "weather", "get_historical_weather", map[string]any{
		"latitude": 54.7, "longitude": 25.3, "start_date": "2024-07-15", "end_date": "2024-07-15",
	})

	calls := LoadCallLog(logPath)
	if len(calls) != 1 {
		t.Fatalf("expected 1 logged call, got %d", len(calls))
	}
	if calls[0].Server != "weather" || calls[0].Tool != "get_historical_weather" {
		t.Errorf("logged call = %+v, want weather/get_historical_weather", calls[0])
	}
}

// TestCallLogDisabledWhenUnset confirms logging is a no-op without the env var.
func TestCallLogDisabledWhenUnset(t *testing.T) {
	t.Setenv(callLogEnv, "")
	dir := t.TempDir()
	// A tool call must still succeed and write no log file.
	callToolViaServer(t, "duckduckgo", "search", map[string]any{"query": "vilnius"})
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("expected no files written, got %d", len(entries))
	}
}
