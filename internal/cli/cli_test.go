package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const cfgWithSecret = `version: 1
servers:
  atlassian:
    enabled: true
    transport: http
    url: https://mcp.example.com/v1/mcp
    auth:
      type: env
      header: Authorization
      value: "Bearer ${OZY_TEST_TOKEN}"
search:
  lexical:
    enabled: true
`

func run(args ...string) (stdout, stderr string, code int) {
	var out, errBuf bytes.Buffer
	code = Execute(args, &out, &errBuf)
	return out.String(), errBuf.String(), code
}

func writeCfg(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
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

func TestUnimplementedReturnsNotImplemented(t *testing.T) {
	out, _, code := run("--format", "json", "index")
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

func TestInitWritesLoadableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	_, _, code := run("--config", path, "init")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("init did not write config: %v", err)
	}
}

func TestDoctorDoesNotLeakSecret(t *testing.T) {
	path := writeCfg(t, cfgWithSecret)
	t.Setenv("OZY_TEST_TOKEN", "supersecretvalue")
	out, _, code := run("--config", path, "--format", "json", "doctor")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Errorf("doctor output leaked the secret:\n%s", out)
	}
}

func TestDoctorReportsMissingEnv(t *testing.T) {
	path := writeCfg(t, cfgWithSecret)
	os.Unsetenv("OZY_TEST_TOKEN")
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
