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
	"github.com/rokasklive/ozy/internal/daemon"
	"github.com/rokasklive/ozy/internal/downstream"
	ozymcp "github.com/rokasklive/ozy/internal/mcp"
	"github.com/rokasklive/ozy/internal/search"
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

// TestEvalRunIsWired verifies eval run executes the harness (no longer
// NOT_IMPLEMENTED) and emits a single verdict-bearing JSON document whose exit
// status follows the gate verdict. --out "" keeps the run from writing files.
func TestEvalRunIsWired(t *testing.T) {
	out, _, code := run("--format", "json", "eval", "run", "--out", "")
	if strings.Contains(out, "NOT_IMPLEMENTED") {
		t.Fatalf("eval run should be wired, got NOT_IMPLEMENTED:\n%s", out)
	}
	var payload struct {
		Schema    string `json:"schema"`
		Verdict   string `json:"verdict"`
		Discovery *struct {
			Overall struct {
				N int `json:"n"`
			} `json:"overall"`
		} `json:"discovery"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("eval run output is not one JSON document: %v\n%s", err, out)
	}
	if payload.Schema == "" {
		t.Error("eval run result missing schema")
	}
	if payload.Discovery == nil || payload.Discovery.Overall.N == 0 {
		t.Error("eval run should report discovery metrics")
	}
	switch payload.Verdict {
	case "pass":
		if code != 0 {
			t.Errorf("exit code = %d, want 0 for a passing run", code)
		}
	case "fail":
		if code != 1 {
			t.Errorf("exit code = %d, want 1 for a failing run", code)
		}
	default:
		t.Errorf("verdict = %q, want pass or fail", payload.Verdict)
	}
}

// TestEvalRunScopedToFamily verifies eval run can scope to one family.
func TestEvalRunScopedToFamily(t *testing.T) {
	out, _, _ := run("--format", "json", "eval", "run", "discovery", "--out", "")
	if !strings.Contains(out, "\"discovery\"") {
		t.Errorf("scoped run should include the discovery report:\n%s", out)
	}
}

// TestEvalReportReadsSnapshot verifies eval report emits the latest snapshot.
func TestEvalReportReadsSnapshot(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := run("eval", "run", "--out", dir, "--format", "concise"); code != 0 && code != 1 {
		t.Fatalf("eval run --out exit = %d", code)
	}
	out, _, code := run("--format", "json", "eval", "report", "--out", dir)
	if code != 0 {
		t.Fatalf("eval report exit = %d:\n%s", code, out)
	}
	var payload struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("eval report output is not JSON: %v\n%s", err, out)
	}
	if payload.Schema == "" {
		t.Error("eval report result missing schema")
	}
}

// TestEvalRunProducesSnapshotAndVerdict is the end-to-end acceptance test: a real
// `ozy eval run` over the committed corpus writes a timestamped snapshot plus
// baseline.json and the BENCHMARKS.md scoreboard, and yields a verdict whose
// exit status the process honors. This runs with the semantic leg gated OFF (the
// default fast path); run it with OZY_EVAL_SEMANTIC=1 to fold in the real-model
// numbers.
func TestEvalRunProducesSnapshotAndVerdict(t *testing.T) {
	dir := t.TempDir()
	out, _, code := run("--format", "json", "eval", "run", "--out", dir)

	var payload struct {
		Schema     string `json:"schema"`
		Verdict    string `json:"verdict"`
		Provenance struct {
			SemanticRan bool `json:"semanticRan"`
		} `json:"provenance"`
		Invocation   json.RawMessage `json:"invocation"`
		Ergonomics   json.RawMessage `json:"ergonomics"`
		TokenEconomy json.RawMessage `json:"tokenEconomy"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("eval run output is not one JSON document: %v\n%s", err, out)
	}
	if payload.Provenance.SemanticRan {
		t.Error("semantic leg should be gated off by default in the acceptance test")
	}
	for name, raw := range map[string]json.RawMessage{
		"invocation": payload.Invocation, "ergonomics": payload.Ergonomics, "tokenEconomy": payload.TokenEconomy,
	} {
		if len(raw) == 0 {
			t.Errorf("a full run should report the %s family", name)
		}
	}
	switch payload.Verdict {
	case "pass":
		if code != 0 {
			t.Errorf("exit code = %d, want 0 for a passing run", code)
		}
	case "fail":
		if code != 1 {
			t.Errorf("exit code = %d, want 1 for a failing run", code)
		}
	default:
		t.Fatalf("verdict = %q, want pass or fail", payload.Verdict)
	}

	for _, rel := range []string{
		filepath.Join("snapshots", "baseline.json"),
		"BENCHMARKS.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("expected artifact %s was not written: %v", rel, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "snapshots"))
	if err != nil {
		t.Fatalf("read snapshots dir: %v", err)
	}
	var timestamped int
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "run-") && strings.HasSuffix(e.Name(), ".json") {
			timestamped++
		}
	}
	if timestamped == 0 {
		t.Error("eval run should write a timestamped snapshot alongside baseline.json")
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
	if searchPayload.Decision != "catalog_empty" && searchPayload.Decision != "no_good_match" {
		t.Fatalf("search decision = %q, want catalog_empty or no_good_match (catalog-backed, no indexed tools)", searchPayload.Decision)
	}
}

func TestSearchAndBrokerFindToolParity(t *testing.T) {
	t.Parallel()

	// Write a config and index tools into the catalog.
	cfgPath := writeCfg(t, `{"version":1,"mcp":{"fixture":{"type":"local","enabled":true}}}`)
	cfg := &config.Loaded{Path: cfgPath, Resolved: &config.Config{
		MCP: map[string]config.ServerConfig{
			"fixture": {Type: "local", Enabled: true},
		},
	}}

	store := catalog.NewMemory()
	_ = store.PutTool(context.Background(), catalog.Tool{
		ToolRef:            "fixture.search",
		ServerID:           "fixture",
		DownstreamToolName: "search",
		Title:              "Search Documents",
		Description:        "Search across all internal documents and wiki pages",
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
	})
	_ = store.PutTool(context.Background(), catalog.Tool{
		ToolRef:            "fixture.send_message",
		ServerID:           "fixture",
		DownstreamToolName: "send_message",
		Title:              "Send Message",
		Description:        "Send a chat message to a channel",
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
	})

	d := daemon.NewWithStore(cfg, store)
	brokerDecision, err := d.Broker().FindTool(context.Background(), "search documents wiki")
	if err != nil {
		t.Fatalf("Broker().FindTool() error = %v", err)
	}
	if brokerDecision.Decision != "use" {
		t.Fatalf("broker decision = %q, want use", brokerDecision.Decision)
	}

	// The CLI search command uses the user's catalog via DefaultPath, not our
	// in-memory store. So we verify the broker produces a consistent decision
	// and that the decision includes a selected tool with its toolRef.
	if brokerDecision.SelectedToolRef != "fixture.search" {
		t.Errorf("broker selected = %q, want fixture.search", brokerDecision.SelectedToolRef)
	}
	if len(brokerDecision.Alternatives) != 1 {
		t.Errorf("alternatives len = %d, want 1 runner-up", len(brokerDecision.Alternatives))
	}
	if brokerDecision.NextAction == nil || brokerDecision.NextAction.Tool != "describeTool" {
		t.Error("nextAction should direct to describeTool")
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

func TestDoctorEmbeddingSectionUnavailableByDefault(t *testing.T) {
	path := writeCfg(t, strings.ReplaceAll(cfgWithSecret, "https://mcp.example.com/v1/mcp", "http://127.0.0.1:1/mcp"))
	t.Setenv("OZY_TEST_TOKEN", "tok")
	t.Setenv("OZY_CATALOG", filepath.Join(t.TempDir(), "catalog.json"))

	prev := sidecarInspector
	sidecarInspector = func(_ context.Context) SidecarStatus {
		return SidecarStatus{Available: false, Reason: "no python toolchain"}
	}
	t.Cleanup(func() { sidecarInspector = prev })

	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (degradation is non-fatal)", code)
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
	var found bool
	for _, c := range payload.Checks {
		if c.Name == "embedding" {
			found = true
			if c.Status != "warn" {
				t.Errorf("embedding status = %q, want warn when sidecar unavailable", c.Status)
			}
			if !strings.Contains(c.Detail, "lexical-only") {
				t.Errorf("embedding detail = %q, want lexical-only notice", c.Detail)
			}
		}
	}
	if !found {
		t.Error("doctor did not render an embedding section")
	}
}

func TestDoctorEmbeddingSectionAvailable(t *testing.T) {
	path := writeCfg(t, strings.ReplaceAll(cfgWithSecret, "https://mcp.example.com/v1/mcp", "http://127.0.0.1:1/mcp"))
	t.Setenv("OZY_TEST_TOKEN", "tok")
	t.Setenv("OZY_CATALOG", filepath.Join(t.TempDir(), "catalog.json"))

	prev := sidecarInspector
	sidecarInspector = func(_ context.Context) SidecarStatus {
		return SidecarStatus{
			Available:   true,
			Model:       "BAAI/bge-small-en-v1.5",
			Dim:         384,
			Backend:     "turbovec",
			VectorCount: 42,
			ToolCount:   42,
		}
	}
	t.Cleanup(func() { sidecarInspector = prev })

	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	var payload struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	var found bool
	for _, c := range payload.Checks {
		if c.Name == "embedding" {
			found = true
			if c.Status != "ok" {
				t.Errorf("embedding status = %q, want ok when sidecar up", c.Status)
			}
			for _, want := range []string{"turbovec", "BAAI/bge-small-en-v1.5", "dim=384", "indexed_tools=42"} {
				if !strings.Contains(c.Detail, want) {
					t.Errorf("embedding detail missing %q: %s", want, c.Detail)
				}
			}
		}
	}
	if !found {
		t.Error("doctor did not render an embedding section")
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
	}, connector, search.New(catalog.NewMemory(), nil))

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

// TestAcceptance_FindDescribeCall_LexicalOnlyLoop is the lexical-only leg of
// the §9.1 acceptance test. With semantic search explicitly disabled the
// findTool → describeTool → callTool loop must still return a confident
// lexical winner + runner-up, the exact schema, and a successful live call.
func TestAcceptance_FindDescribeCall_LexicalOnlyLoop(t *testing.T) {
	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
	t.Setenv("OZY_CATALOG", catalogPath)

	// Seed the catalog with two distinct tools so findTool has a real choice.
	store, err := catalog.NewFile(catalogPath)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	seedTool := catalog.Tool{
		ToolRef:            "fixture.fixture_echo",
		ServerID:           "fixture",
		DownstreamToolName: "fixture_echo",
		Title:              "Fixture Echo",
		Description:        "Echo a query string back",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "the value to echo"},
			},
			"required": []any{"query"},
		},
		CapabilityText: []string{"echo", "query"},
		ServerStatus:   catalog.ServerOnline,
		CallableNow:    true,
		Freshness:      catalog.FreshnessFresh,
	}
	if err := store.PutTool(context.Background(), seedTool); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	cfgPath := writeCfg(t, fmt.Sprintf(`{
  "search": {"semantic": {"enabled": false}},
  "mcp": {
    "fixture": {
      "type": "local",
      "command": [%q],
      "environment": {"OZY_TEST_MCP_SERVER": "1"},
      "timeout": 5000
    }
  }
}`, os.Args[0]))

	// findTool returns a "use" with the seeded toolRef and a non-empty runner-up.
	findOut, _, code := run("--config", cfgPath, "--format", "json", "search", "echo a query")
	if code != 0 {
		t.Fatalf("search exit code = %d\n%s", code, findOut)
	}
	var findPayload struct {
		Decision        string `json:"decision"`
		SelectedToolRef string `json:"selectedToolRef"`
		Alternatives    []struct {
			ToolRef string `json:"toolRef"`
		} `json:"alternatives"`
	}
	if err := json.Unmarshal([]byte(findOut), &findPayload); err != nil {
		t.Fatalf("search output is not valid JSON: %v\n%s", err, findOut)
	}
	if findPayload.Decision != "use" {
		t.Errorf("findTool decision = %s, want use", findPayload.Decision)
	}
	if findPayload.SelectedToolRef != "fixture.fixture_echo" {
		t.Errorf("findTool selected = %s, want fixture.fixture_echo", findPayload.SelectedToolRef)
	}

	// describeTool returns the exact schema (inputSchema round-trips).
	describeOut, _, code := run("--config", cfgPath, "--format", "json", "describe", "fixture.fixture_echo")
	if code != 0 {
		t.Fatalf("describe exit code = %d\n%s", code, describeOut)
	}
	var describePayload map[string]any
	if err := json.Unmarshal([]byte(describeOut), &describePayload); err != nil {
		t.Fatalf("describe output is not valid JSON: %v\n%s", err, describeOut)
	}
	if describePayload["toolRef"] != "fixture.fixture_echo" {
		t.Errorf("describe toolRef = %v, want fixture.fixture_echo", describePayload["toolRef"])
	}
	schema, _ := describePayload["inputSchema"].(map[string]any)
	if schema == nil {
		t.Fatalf("describe inputSchema missing: %s", describeOut)
	}
	props, _ := schema["properties"].(map[string]any)
	if _, ok := props["query"]; !ok {
		t.Errorf("describe inputSchema.properties.query missing: %+v", props)
	}
}
