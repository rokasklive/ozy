package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokask/ozy/internal/contract"
)

const validJSONC = `{
  // Downstream servers use the opencode mcp shape.
  "mcp": {
    "atlassian": {
      "type": "remote",
      "url": "https://mcp.example.com/v1/mcp",
      "headers": {
        "Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}"
      },
      "enabled": true,
    },
    "filesystem": {
      "type": "local",
      "command": ["filesystem-mcp", "--root", "."],
      "environment": {
        "OZY_ROOT": "{env:OZY_ROOT}"
      },
      "enabled": false
    },
  },
  "search": {
    "lexical": {"enabled": true},
    "semantic": {"enabled": false, "required": false},
  },
  "embedding": {"provider": "python-local", "required": false},
  "budgets": {
    "findTool": {"maxResults": 5, "includeFullSchemas": false},
    "describeTool": {"includeExamples": true},
    "callTool": {"maxResultBytes": 65536},
  },
}`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

func TestLoad_ValidJSONCWithOpencodeMCPShape(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", validJSONC)
	t.Setenv("ATLASSIAN_MCP_TOKEN", "s3cret")
	t.Setenv("OZY_ROOT", "/tmp/ozy")

	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	remote := loaded.Resolved.MCP["atlassian"]
	if remote.Type != "remote" || remote.URL != "https://mcp.example.com/v1/mcp" || !remote.Enabled {
		t.Errorf("remote server = %+v, want resolved remote server", remote)
	}
	if remote.Headers["Authorization"] != "Bearer s3cret" {
		t.Errorf("resolved Authorization = %q, want %q", remote.Headers["Authorization"], "Bearer s3cret")
	}
	local := loaded.Resolved.MCP["filesystem"]
	if local.Type != "local" || len(local.Command) != 3 || local.Command[0] != "filesystem-mcp" {
		t.Errorf("local server command = %+v, want command array", local.Command)
	}
	if local.Environment["OZY_ROOT"] != "/tmp/ozy" {
		t.Errorf("resolved environment = %q, want /tmp/ozy", local.Environment["OZY_ROOT"])
	}
	if len(loaded.Missing) != 0 {
		t.Errorf("Missing = %+v, want none", loaded.Missing)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, cerr := Load(filepath.Join(t.TempDir(), "ozy.jsonc"))
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
	if cerr.AgentInstruction == "" || !strings.Contains(cerr.Message, "ozy.jsonc") {
		t.Errorf("missing-file error = %+v, want path and repair guidance", cerr)
	}
}

func TestLoad_MissingEnvVarIsDiagnostic(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", validJSONC)
	os.Unsetenv("ATLASSIAN_MCP_TOKEN")
	t.Setenv("OZY_ROOT", "/tmp/ozy")

	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if len(loaded.Missing) != 1 {
		t.Fatalf("Missing = %+v, want exactly one", loaded.Missing)
	}
	got := loaded.Missing[0]
	if got.Var != "ATLASSIAN_MCP_TOKEN" || got.Server != "atlassian" || got.Field != "headers.Authorization" {
		t.Errorf("Missing[0] = %+v, want var/server/header field", got)
	}
}

func TestLoad_ValidationErrorsNameServerAndField(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "local without command",
			content: `{"mcp":{"x":{"type":"local","enabled":true}}}`,
			want:    "command",
		},
		{
			name:    "remote without url",
			content: `{"mcp":{"x":{"type":"remote","enabled":true}}}`,
			want:    "url",
		},
		{
			name:    "unknown type",
			content: `{"mcp":{"x":{"type":"smtp","enabled":true}}}`,
			want:    "type",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, cerr := Load(writeTemp(t, "ozy.jsonc", tt.content))
			if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
				t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
			}
			if cerr.ServerID != "x" || !strings.Contains(cerr.Message, tt.want) {
				t.Errorf("Load() error = %+v, want server x and field %q", cerr, tt.want)
			}
		})
	}
}

func TestRedacted_HidesResolvedSecretsAndShowsEnvRefs(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", validJSONC)
	t.Setenv("ATLASSIAN_MCP_TOKEN", "supersecretvalue")
	t.Setenv("OZY_ROOT", "/tmp/ozy")
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}

	red := loaded.Redacted()
	auth := red.MCP["atlassian"].Headers["Authorization"]
	if auth != "Bearer {env:ATLASSIAN_MCP_TOKEN}" {
		t.Errorf("redacted Authorization = %q, want unresolved {env:} form", auth)
	}
	if strings.Contains(auth, "supersecretvalue") {
		t.Error("redacted config leaked the resolved secret")
	}
}

func TestDefaultPathPrecedence(t *testing.T) {
	t.Setenv("OZY_CONFIG", "")
	t.Chdir(t.TempDir())

	if got := DefaultPath(); filepath.Base(got) != "ozy.jsonc" || strings.Contains(got, ".yaml") {
		t.Fatalf("DefaultPath() = %q, want user config ozy.jsonc fallback", got)
	}

	if err := os.WriteFile("ozy.json", []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write ozy.json: %v", err)
	}
	if got := DefaultPath(); got != "ozy.json" {
		t.Fatalf("DefaultPath() = %q, want ./ozy.json", got)
	}

	if err := os.WriteFile("ozy.jsonc", []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write ozy.jsonc: %v", err)
	}
	if got := DefaultPath(); got != "ozy.jsonc" {
		t.Fatalf("DefaultPath() = %q, want ./ozy.jsonc", got)
	}

	t.Setenv("OZY_CONFIG", "/tmp/custom-ozy.jsonc")
	if got := DefaultPath(); got != "/tmp/custom-ozy.jsonc" {
		t.Fatalf("DefaultPath() = %q, want env override", got)
	}
}

func TestWriteStarter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "ozy.jsonc")
	if err := WriteStarter(path); err != nil {
		t.Fatalf("WriteStarter() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read starter config: %v", err)
	}
	if !strings.Contains(string(data), `"mcp"`) || !strings.Contains(string(data), `{env:ATLASSIAN_MCP_TOKEN}`) {
		t.Fatalf("starter config =\n%s\nwant opencode mcp shape with env refs", data)
	}
	if _, cerr := Load(path); cerr != nil {
		t.Fatalf("starter config does not load cleanly: %v", cerr)
	}
	if err := WriteStarter(path); err == nil {
		t.Error("WriteStarter() should refuse to overwrite an existing file")
	}
}
