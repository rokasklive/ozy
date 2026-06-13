package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/downstream"
	ozymcp "github.com/rokasklive/ozy/internal/mcp"
)

const cfgWithSecret = `{
  "mcp": {
    "atlassian": {
      "type": "remote",
      "url": "https://mcp.example.com/v1/mcp",
      "headers": {
        "Authorization": "Bearer {env:OZY_TEST_TOKEN}"
      },
      "enabled": true
    }
  },
  "search": {
    "lexical": {
      "enabled": true
    }
  }
}`

func TestMain(m *testing.M) {
	if os.Getenv("OZY_TEST_MCP_SERVER") == "1" {
		os.Exit(runTestMCPServer())
	}
	os.Exit(m.Run())
}

func runTestMCPServer() int {
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "ozy-test-mcp", Version: "0"}, nil)
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "fixture_search",
		Title:       "Fixture Search",
		Description: "Search fixture data",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}, func(context.Context, *mcpsdk.CallToolRequest, map[string]any) (*mcpsdk.CallToolResult, any, error) {
		return &mcpsdk.CallToolResult{}, nil, nil
	})
	mcpsdk.AddTool(server, &mcpsdk.Tool{
		Name:        "fixture_echo",
		Title:       "Fixture Echo",
		Description: "Echo the `query` argument back to the caller",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args map[string]any) (*mcpsdk.CallToolResult, any, error) {
		query, _ := args["query"].(string)
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "echo: " + query}},
		}, nil, nil
	})
	if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		return 1
	}
	return 0
}

func run(args ...string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func writeCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestHelpListsAllCommands(t *testing.T) {
	out, _, code := run("--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	for _, name := range []string{"init", "daemon", "mcp", "index", "doctor", "list", "search", "describe", "call", "eval"} {
		if !strings.Contains(out, name) {
			t.Errorf("--help output missing command %q", name)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	out, _, code := run("--version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out, Version) {
		t.Errorf("--version output %q missing version %q", out, Version)
	}
}

func TestSearchJSONIsSingleDocument(t *testing.T) {
	path := writeCfg(t, cfgWithSecret)
	t.Setenv("OZY_TEST_TOKEN", "x")
	out, _, code := run("--config", path, "--format", "json", "search", "find internal docs")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (findTool decisions are not errors)", code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not one JSON document: %v\n%s", err, out)
	}
	decision := payload["decision"]
	// Live discovery: unreachable servers produce known_but_unavailable, not catalog_empty.
	if decision != "known_but_unavailable" && decision != "catalog_empty" {
		t.Errorf("decision = %v, want known_but_unavailable or catalog_empty", decision)
	}
}

func TestEvalReturnsNotImplemented(t *testing.T) {
	out, _, code := run("--format", "json", "eval", "run")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 for an unimplemented operation", code)
	}
	var payload struct {
		OK    bool `json:"ok"`
		Error struct {
			Type             string `json:"type"`
			AgentInstruction string `json:"agentInstruction"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if payload.OK {
		t.Error("ok = true, want false")
	}
	if payload.Error.Type != "NOT_IMPLEMENTED" {
		t.Errorf("error.type = %q, want NOT_IMPLEMENTED", payload.Error.Type)
	}
	if payload.Error.AgentInstruction == "" {
		t.Error("NOT_IMPLEMENTED must carry an agentInstruction")
	}
}

func TestIndexNoReachableServerReturnsInstructionalSummary(t *testing.T) {
	path := writeCfg(t, cfgWithSecret)
	t.Setenv("OZY_TEST_TOKEN", "x")
	t.Setenv("OZY_CATALOG", filepath.Join(t.TempDir(), "catalog.json"))

	out, _, code := run("--config", path, "--format", "json", "index")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 when no server is reachable", code)
	}
	var payload struct {
		OK               bool   `json:"ok"`
		ServersReached   int    `json:"serversReached"`
		ServersFailed    int    `json:"serversFailed"`
		AgentInstruction string `json:"agentInstruction"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if payload.OK || payload.ServersReached != 0 || payload.ServersFailed != 1 {
		t.Fatalf("index summary = %+v, want total failure summary", payload)
	}
	if payload.AgentInstruction == "" {
		t.Fatal("index total failure must be instructional")
	}
}

func TestCallStructuredFailureExitsNonZero(t *testing.T) {
	path := writeCfg(t, cfgWithSecret)
	t.Setenv("OZY_TEST_TOKEN", "x")
	out, _, code := run("--config", path, "--format", "json", "call", "atlassian.confluence_search", "--json", "{}")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if payload["ok"] != false {
		t.Errorf("ok = %v, want false", payload["ok"])
	}
}

func TestListDescribeAndSearchUsePersistedCatalog(t *testing.T) {
	cfgPath := writeCfg(t, cfgWithSecret)
	t.Setenv("OZY_TEST_TOKEN", "x")
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("OZY_CATALOG", catalogPath)

	store, err := catalog.NewFile(catalogPath)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	if err := store.PutServer(context.Background(), catalog.Server{ID: "atlassian", Status: catalog.ServerOnline}); err != nil {
		t.Fatalf("PutServer() error = %v", err)
	}
	if err := store.PutTool(context.Background(), catalog.Tool{
		ToolRef:            "atlassian.confluence_search",
		ServerID:           "atlassian",
		DownstreamToolName: "confluence_search",
		Title:              "Confluence Search",
		Description:        "Search Confluence",
		InputSchema:        map[string]any{"type": "object"},
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
		LastIndexedAt:      time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC),
		SchemaHash:         "abc123",
		Freshness:          catalog.FreshnessFresh,
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	out, _, code := run("--config", cfgPath, "--format", "json", "list")
	if code != 0 {
		t.Fatalf("list exit code = %d, want 0", code)
	}
	var listPayload struct {
		Tools []struct {
			ToolRef   string `json:"toolRef"`
			ServerID  string `json:"serverId"`
			Freshness string `json:"freshness"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &listPayload); err != nil {
		t.Fatalf("list output is not valid JSON: %v\n%s", err, out)
	}
	if len(listPayload.Tools) != 1 || listPayload.Tools[0].ToolRef != "atlassian.confluence_search" ||
		listPayload.Tools[0].ServerID != "atlassian" || listPayload.Tools[0].Freshness != "fresh" {
		t.Fatalf("list payload = %+v, want persisted indexed tool", listPayload)
	}

	out, _, code = run("--config", cfgPath, "--format", "json", "describe", "atlassian.confluence_search")
	if code != 0 {
		t.Fatalf("describe exit code = %d, want 0", code)
	}
	var describePayload struct {
		ToolRef     string         `json:"toolRef"`
		InputSchema map[string]any `json:"inputSchema"`
		Status      struct {
			CatalogFreshness string `json:"catalogFreshness"`
			ServerStatus     string `json:"serverStatus"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &describePayload); err != nil {
		t.Fatalf("describe output is not valid JSON: %v\n%s", err, out)
	}
	if describePayload.ToolRef != "atlassian.confluence_search" ||
		describePayload.InputSchema["type"] != "object" ||
		describePayload.Status.CatalogFreshness != "fresh" ||
		describePayload.Status.ServerStatus != "online" {
		t.Fatalf("describe payload = %+v, want persisted schema/status", describePayload)
	}

	out, _, code = run("--config", cfgPath, "--format", "json", "search", "anything")
	if code != 0 {
		t.Fatalf("search exit code = %d, want 0", code)
	}
	var searchPayload struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal([]byte(out), &searchPayload); err != nil {
		t.Fatalf("search output is not valid JSON: %v\n%s", err, out)
	}
	if searchPayload.Decision != "known_but_unavailable" {
		t.Fatalf("search decision = %q, want known_but_unavailable (live discovery without reachable servers)", searchPayload.Decision)
	}
}

func TestInitWritesLoadableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ozy.jsonc")
	_, _, code := run("--config", path, "init")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("init did not write config: %v", err)
	}
}

func TestInitWritesDefaultUserConfigFile(t *testing.T) {
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("OZY_CONFIG", "")
	t.Chdir(t.TempDir())
	if err := os.WriteFile("ozy.jsonc", []byte(`{"mcp":{}}`), 0o644); err != nil {
		t.Fatalf("write project-local config: %v", err)
	}

	out, _, code := run("init")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\n%s", code, out)
	}
	path := filepath.Join(xdg, "ozy", "ozy.jsonc")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("init did not write default user config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := os.Stat("ozy.jsonc"); err != nil {
		t.Fatalf("project-local config should remain untouched: %v", err)
	}
}

func TestCLIIndexesAndExposesToolsFromExplicitMCPConfig(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("OZY_CATALOG", catalogPath)
	cfgPath := writeCfg(t, fmt.Sprintf(`{
  "mcp": {
    "fixture": {
      "type": "local",
      "command": [%q],
      "environment": {"OZY_TEST_MCP_SERVER": "1"},
      "timeout": 5000
    }
  }
}`, os.Args[0]))

	out, _, code := run("--config", cfgPath, "--format", "json", "index")
	if code != 0 {
		t.Fatalf("index exit code = %d, want 0\n%s", code, out)
	}
	var indexPayload struct {
		OK             bool `json:"ok"`
		ServersReached int  `json:"serversReached"`
		ToolsIndexed   int  `json:"toolsIndexed"`
	}
	if err := json.Unmarshal([]byte(out), &indexPayload); err != nil {
		t.Fatalf("index output is not valid JSON: %v\n%s", err, out)
	}
	if !indexPayload.OK || indexPayload.ServersReached != 1 || indexPayload.ToolsIndexed != 2 {
		t.Fatalf("index payload = %+v, want one reached server and two tools", indexPayload)
	}

	out, _, code = run("--config", cfgPath, "--format", "json", "list")
	if code != 0 {
		t.Fatalf("list exit code = %d, want 0\n%s", code, out)
	}
	var listPayload struct {
		Tools []struct {
			ToolRef   string `json:"toolRef"`
			ServerID  string `json:"serverId"`
			Freshness string `json:"freshness"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(out), &listPayload); err != nil {
		t.Fatalf("list output is not valid JSON: %v\n%s", err, out)
	}
	if len(listPayload.Tools) != 2 {
		t.Fatalf("list payload = %+v, want two indexed fixture tools", listPayload)
	}
	seenTools := make(map[string]struct {
		serverID  string
		freshness string
	})
	for _, tool := range listPayload.Tools {
		if tool.ServerID != "fixture" || tool.Freshness != "fresh" {
			t.Fatalf("list payload = %+v, want every tool to be on server fixture with fresh freshness", listPayload)
		}
		seenTools[tool.ToolRef] = struct {
			serverID  string
			freshness string
		}{tool.ServerID, tool.Freshness}
	}
	if _, ok := seenTools["fixture.fixture_search"]; !ok {
		t.Errorf("list missing fixture.fixture_search: %+v", listPayload)
	}
	if _, ok := seenTools["fixture.fixture_echo"]; !ok {
		t.Errorf("list missing fixture.fixture_echo: %+v", listPayload)
	}

	out, _, code = run("--config", cfgPath, "--format", "json", "describe", "fixture.fixture_search")
	if code != 0 {
		t.Fatalf("describe exit code = %d, want 0\n%s", code, out)
	}
	var describePayload struct {
		ToolRef     string         `json:"toolRef"`
		InputSchema map[string]any `json:"inputSchema"`
		Status      struct {
			CatalogFreshness string `json:"catalogFreshness"`
			ServerStatus     string `json:"serverStatus"`
		} `json:"status"`
	}
	if err := json.Unmarshal([]byte(out), &describePayload); err != nil {
		t.Fatalf("describe output is not valid JSON: %v\n%s", err, out)
	}
	if describePayload.ToolRef != "fixture.fixture_search" ||
		describePayload.InputSchema["type"] != "object" ||
		describePayload.Status.CatalogFreshness != "fresh" ||
		describePayload.Status.ServerStatus != "online" {
		t.Fatalf("describe payload = %+v, want fixture schema/status", describePayload)
	}

	out, _, code = run("--config", cfgPath, "--format", "json", "search", "fixture search")
	if code != 0 {
		t.Fatalf("search exit code = %d, want 0\n%s", code, out)
	}
	var searchPayload struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal([]byte(out), &searchPayload); err != nil {
		t.Fatalf("search output is not valid JSON: %v\n%s", err, out)
	}
	if searchPayload.Decision == "catalog_empty" {
		t.Fatalf("search decision = catalog_empty, want populated catalog decision")
	}
}

func TestDoctorReportsServerHealthAndRedactsSecrets(t *testing.T) {
	cfg := strings.ReplaceAll(cfgWithSecret, "https://mcp.example.com/v1/mcp", "http://127.0.0.1:1/mcp")
	path := writeCfg(t, cfg)
	t.Setenv("OZY_TEST_TOKEN", "supersecretvalue")
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("OZY_CATALOG", catalogPath)

	store, err := catalog.NewFile(catalogPath)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	if err := store.PutTool(context.Background(), catalog.Tool{
		ToolRef:      "atlassian.confluence_search",
		ServerID:     "atlassian",
		Freshness:    catalog.FreshnessFresh,
		ServerStatus: catalog.ServerOnline,
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Fatalf("doctor output leaked secret:\n%s", out)
	}
	if !strings.Contains(out, "server:atlassian") || !strings.Contains(out, "indexed tools: 1") {
		t.Fatalf("doctor output =\n%s\nwant per-server health and indexed-tool count", out)
	}
}

func TestDoctorDoesNotLeakSecret(t *testing.T) {
	path := writeCfg(t, strings.ReplaceAll(cfgWithSecret, "https://mcp.example.com/v1/mcp", "http://127.0.0.1:1/mcp"))
	t.Setenv("OZY_TEST_TOKEN", "supersecretvalue")
	t.Setenv("OZY_CATALOG", filepath.Join(t.TempDir(), "catalog.json"))
	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Errorf("doctor output leaked the secret:\n%s", out)
	}
}

func TestDoctorReportsMissingEnv(t *testing.T) {
	path := writeCfg(t, strings.ReplaceAll(cfgWithSecret, "https://mcp.example.com/v1/mcp", "http://127.0.0.1:1/mcp"))
	os.Unsetenv("OZY_TEST_TOKEN")
	t.Setenv("OZY_CATALOG", filepath.Join(t.TempDir(), "catalog.json"))
	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var payload struct {
		OK     bool `json:"ok"`
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if payload.OK {
		t.Error("doctor ok = true, want false when an env var is missing")
	}
	found := false
	for _, c := range payload.Checks {
		if strings.Contains(c.Detail, "OZY_TEST_TOKEN") {
			found = true
		}
	}
	if !found {
		t.Errorf("doctor did not name the missing env var:\n%s", out)
	}
}

func TestCall_InvokesFixtureDownstreamViaCLIAndParityMatchesMCPPath(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("OZY_CATALOG", catalogPath)
	cfgPath := writeCfg(t, fmt.Sprintf(`{
  "mcp": {
    "fixture": {
      "type": "local",
      "command": [%q],
      "environment": {"OZY_TEST_MCP_SERVER": "1"},
      "timeout": 5000
    }
  }
}`, os.Args[0]))

	const (
		toolRef = "fixture.fixture_echo"
		query   = "hello"
	)

	out, _, code := run("--config", cfgPath, "--format", "json", "call", toolRef, "--json", fmt.Sprintf(`{"query":%q}`, query))
	if code != 0 {
		t.Fatalf("CLI call exit code = %d, want 0\n%s", code, out)
	}
	var cliPayload map[string]any
	if err := json.Unmarshal([]byte(out), &cliPayload); err != nil {
		t.Fatalf("CLI output is not valid JSON: %v\n%s", err, out)
	}
	if cliPayload["ok"] != true {
		t.Errorf("CLI ok = %v, want true", cliPayload["ok"])
	}
	if cliPayload["toolRef"] != toolRef {
		t.Errorf("CLI toolRef = %v, want %q", cliPayload["toolRef"], toolRef)
	}
	if cliPayload["result"] != fmt.Sprintf("echo: %s", query) {
		t.Errorf("CLI result = %v, want %q", cliPayload["result"], fmt.Sprintf("echo: %s", query))
	}
	if summary, _ := cliPayload["resultSummary"].(string); summary == "" {
		t.Errorf("CLI resultSummary must be non-empty, got %v", cliPayload["resultSummary"])
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	dsServer := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fixture-downstream", Version: "0"}, nil)
	mcpsdk.AddTool(dsServer, &mcpsdk.Tool{
		Name:        "fixture_echo",
		Title:       "Fixture Echo",
		Description: "Echo the `query` argument back to the caller",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"query": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, _ *mcpsdk.CallToolRequest, args map[string]any) (*mcpsdk.CallToolResult, any, error) {
		q, _ := args["query"].(string)
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "echo: " + q}},
		}, nil, nil
	})
	dsServerT, dsClientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = dsServer.Run(ctx, dsServerT) }()

	connector := downstream.New(downstream.WithTransportFactory(
		func(_ string, _ config.ServerConfig) (mcpsdk.Transport, error) {
			return dsClientT, nil
		},
	))
	liveBroker := broker.NewLive(catalog.NewMemory(), &config.Config{
		MCP: map[string]config.ServerConfig{
			"fixture": {Type: "local", Enabled: true},
		},
	}, connector)

	ozyServerT, ozyClientT := mcpsdk.NewInMemoryTransports()
	adapter := ozymcp.New(liveBroker, "test")
	go func() { _ = adapter.Server().Run(ctx, ozyServerT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, ozyClientT, nil)
	if err != nil {
		t.Fatalf("MCP client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	res, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{
		Name: "callTool",
		Arguments: map[string]any{
			"toolRef":   toolRef,
			"arguments": map[string]any{"query": query},
		},
	})
	if err != nil {
		t.Fatalf("MCP CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("MCP callTool returned IsError; content=%v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatal("MCP callTool returned no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("MCP content[0] is %T, want *TextContent", res.Content[0])
	}
	var mcpPayload map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &mcpPayload); err != nil {
		t.Fatalf("MCP content is not valid JSON: %v\n%s", err, tc.Text)
	}

	if mcpPayload["ok"] != cliPayload["ok"] {
		t.Errorf("parity: ok = %v (MCP) vs %v (CLI)", mcpPayload["ok"], cliPayload["ok"])
	}
	if mcpPayload["toolRef"] != cliPayload["toolRef"] {
		t.Errorf("parity: toolRef = %v (MCP) vs %v (CLI)", mcpPayload["toolRef"], cliPayload["toolRef"])
	}
	if mcpPayload["result"] != cliPayload["result"] {
		t.Errorf("parity: result = %v (MCP) vs %v (CLI)", mcpPayload["result"], cliPayload["result"])
	}
	if mcpPayload["resultSummary"] != cliPayload["resultSummary"] {
		t.Errorf("parity: resultSummary = %v (MCP) vs %v (CLI)", mcpPayload["resultSummary"], cliPayload["resultSummary"])
	}
}
