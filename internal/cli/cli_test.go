package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rokask/ozy/internal/catalog"
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
		t.Fatalf("exit code = %d, want 0 (catalog_empty is a valid decision)", code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("output is not one JSON document: %v\n%s", err, out)
	}
	if payload["decision"] != "catalog_empty" {
		t.Errorf("decision = %v, want catalog_empty", payload["decision"])
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
	if searchPayload.Decision != "no_good_match" {
		t.Fatalf("search decision = %q, want no_good_match", searchPayload.Decision)
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
