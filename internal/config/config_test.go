package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rokask/ozy/internal/contract"
)

const validYAML = `version: 1
servers:
  atlassian:
    enabled: true
    transport: http
    url: https://mcp.example.com/v1/mcp
    auth:
      type: env
      header: Authorization
      value: "Bearer ${ATLASSIAN_MCP_TOKEN}"
search:
  lexical:
    enabled: true
  semantic:
    enabled: false
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_ValidWithEnvResolution(t *testing.T) {
	path := writeTemp(t, validYAML)
	t.Setenv("ATLASSIAN_MCP_TOKEN", "s3cret")

	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if loaded.Resolved.Servers["atlassian"].Auth.Value != "Bearer s3cret" {
		t.Errorf("resolved auth value = %q, want %q",
			loaded.Resolved.Servers["atlassian"].Auth.Value, "Bearer s3cret")
	}
	if len(loaded.Missing) != 0 {
		t.Errorf("Missing = %+v, want none", loaded.Missing)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, cerr := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
	if cerr.AgentInstruction == "" {
		t.Error("missing-file error must carry repair guidance")
	}
}

func TestLoad_MissingEnvVarIsDiagnostic(t *testing.T) {
	path := writeTemp(t, validYAML)
	os.Unsetenv("ATLASSIAN_MCP_TOKEN")

	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if len(loaded.Missing) != 1 {
		t.Fatalf("Missing = %+v, want exactly one", loaded.Missing)
	}
	got := loaded.Missing[0]
	if got.Var != "ATLASSIAN_MCP_TOKEN" || got.Server != "atlassian" || got.Field != "auth.value" {
		t.Errorf("Missing[0] = %+v, want var/server/field for the token", got)
	}
}

func TestLoad_InvalidTransport(t *testing.T) {
	const bad = `version: 1
servers:
  x:
    enabled: true
    transport: carrier-pigeon
`
	_, cerr := Load(writeTemp(t, bad))
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
}

func TestLoad_HTTPRequiresURL(t *testing.T) {
	const bad = `version: 1
servers:
  x:
    enabled: true
    transport: http
`
	_, cerr := Load(writeTemp(t, bad))
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
}

func TestLoad_WrongVersion(t *testing.T) {
	_, cerr := Load(writeTemp(t, "version: 2\n"))
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
}

func TestRedacted_HidesSecrets(t *testing.T) {
	path := writeTemp(t, validYAML)
	t.Setenv("ATLASSIAN_MCP_TOKEN", "s3cret")
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}

	red := loaded.Redacted()
	got := red.Servers["atlassian"].Auth.Value
	if got != "Bearer ${ATLASSIAN_MCP_TOKEN}" {
		t.Errorf("redacted auth value = %q, want the unresolved ${ref} form", got)
	}
	if got == "Bearer s3cret" {
		t.Error("redacted config leaked the resolved secret")
	}
}

func TestWriteStarter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.yaml")
	if err := WriteStarter(path); err != nil {
		t.Fatalf("WriteStarter() error = %v", err)
	}
	if _, cerr := Load(path); cerr != nil {
		t.Fatalf("starter config does not load cleanly: %v", cerr)
	}
	if err := WriteStarter(path); err == nil {
		t.Error("WriteStarter() should refuse to overwrite an existing file")
	}
}
