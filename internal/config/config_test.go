package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/contract"
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
	if remote.Type != "remote" || remote.URL != "https://mcp.example.com/v1/mcp" || !remote.IsEnabled() {
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
	cwd := t.TempDir()
	t.Chdir(cwd)
	xdg := filepath.Join(t.TempDir(), "xdg")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	wantDefault := filepath.Join(xdg, "ozy", "ozy.jsonc")
	if got := Home(); got != filepath.Join(xdg, "ozy") {
		t.Fatalf("Home() = %q, want %q", got, filepath.Join(xdg, "ozy"))
	}
	if got := DefaultPath(); got != wantDefault {
		t.Fatalf("DefaultPath() = %q, want user config path %q", got, wantDefault)
	}

	if err := os.WriteFile("ozy.json", []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write ozy.json: %v", err)
	}
	if got := DefaultPath(); got != wantDefault {
		t.Fatalf("DefaultPath() = %q, want project-local files ignored in favor of %q", got, wantDefault)
	}

	if err := os.WriteFile("ozy.jsonc", []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write ozy.jsonc: %v", err)
	}
	if got := DefaultPath(); got != wantDefault {
		t.Fatalf("DefaultPath() = %q, want project-local files ignored in favor of %q", got, wantDefault)
	}

	t.Setenv("OZY_CONFIG", "/tmp/custom-ozy.jsonc")
	if got := DefaultPath(); got != "/tmp/custom-ozy.jsonc" {
		t.Fatalf("DefaultPath() = %q, want env override", got)
	}
}

func TestConfigHomeFallbacks(t *testing.T) {
	t.Setenv("OZY_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := Home(), filepath.Join(home, ".config", "ozy"); got != want {
		t.Fatalf("Home() = %q, want %q", got, want)
	}
	if got := configHomeFor("windows", "", "C:\\Users\\Ada\\AppData\\Roaming"); got != filepath.Join("C:\\Users\\Ada\\AppData\\Roaming", "ozy") {
		t.Fatalf("windows configHomeFor() = %q, want roaming config dir plus ozy", got)
	}
}

func TestLoad_OpencodeMCPSectionCompatibility(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{
  "mcp": {
    "local": {
      "type": "local",
      "command": ["npx", "-y", "my-mcp"],
      "cwd": "/tmp/workspace",
      "environment": {"TOKEN": "{env:LOCAL_TOKEN}"}
    },
    "remote": {
      "type": "remote",
      "url": "https://mcp.example.com",
      "headers": {"Authorization": "Bearer {env:REMOTE_TOKEN}"},
      "oauth": {
        "clientId": "{env:CLIENT_ID}",
        "clientSecret": "{env:CLIENT_SECRET}",
        "scope": "tools:read"
      },
      "enabled": false,
      "timeout": 10000
    },
    "api-key": {
      "type": "remote",
      "url": "https://api-key.example.com/mcp",
      "oauth": false
    },
    "old-server": {
      "enabled": false
    }
  },
  "agent": {"ignored": true},
  "tools": {"ignored": false}
}`)
	t.Setenv("LOCAL_TOKEN", "local-secret")
	t.Setenv("REMOTE_TOKEN", "remote-secret")

	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}

	local := loaded.Resolved.MCP["local"]
	if !local.IsEnabled() {
		t.Fatal("local server omitted enabled, want enabled by default")
	}
	if local.CWD != "/tmp/workspace" {
		t.Fatalf("local CWD = %q, want /tmp/workspace", local.CWD)
	}
	if local.Timeout != DefaultDiscoveryTimeoutMillis {
		t.Fatalf("local Timeout = %d, want default %d", local.Timeout, DefaultDiscoveryTimeoutMillis)
	}

	remote := loaded.Resolved.MCP["remote"]
	if remote.IsEnabled() {
		t.Fatal("remote enabled=false, want disabled")
	}
	if remote.Headers["Authorization"] != "Bearer remote-secret" {
		t.Fatalf("remote Authorization = %q, want resolved header", remote.Headers["Authorization"])
	}
	if remote.Timeout != 10000 {
		t.Fatalf("remote Timeout = %d, want 10000", remote.Timeout)
	}
	if !strings.Contains(string(remote.OAuth), `"clientId"`) {
		t.Fatalf("remote OAuth = %s, want preserved object", remote.OAuth)
	}

	apiKey := loaded.Resolved.MCP["api-key"]
	if !apiKey.IsEnabled() {
		t.Fatal("api-key server omitted enabled, want enabled by default")
	}
	if string(apiKey.OAuth) != "false" {
		t.Fatalf("api-key OAuth = %s, want false", apiKey.OAuth)
	}
	if loaded.Resolved.MCP["old-server"].IsEnabled() {
		t.Fatal("old-server enabled=false without type, want disabled compatibility entry")
	}
}

func TestLoad_ExampleMCPFixture(t *testing.T) {
	loaded, cerr := Load(filepath.Join("..", "..", "examples", "test_mcp_examples.jsonc"))
	if cerr != nil {
		t.Fatalf("Load(example fixture) error = %v", cerr)
	}
	for _, id := range []string{"searxng", "javadoc", "opengrok"} {
		server := loaded.Resolved.MCP[id]
		if server.Type != "local" || !server.IsEnabled() || len(server.Command) == 0 {
			t.Fatalf("server %s = %+v, want enabled local server with command", id, server)
		}
		if len(server.Environment) == 0 {
			t.Fatalf("server %s environment = %+v, want preserved environment map", id, server.Environment)
		}
	}
	if got := loaded.Resolved.MCP["javadoc"].Timeout; got != 180000 {
		t.Fatalf("javadoc timeout = %d, want 180000", got)
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

func TestEmbeddingConfig_DefaultsWhenEmbeddingOmitted(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{}}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	emb := loaded.Resolved.Embedding
	if emb.VectorBackend != DefaultVectorBackend {
		t.Errorf("VectorBackend = %q, want %q", emb.VectorBackend, DefaultVectorBackend)
	}
	if emb.Model != DefaultEmbeddingModel {
		t.Errorf("Model = %q, want %q", emb.Model, DefaultEmbeddingModel)
	}
}

func TestEmbeddingConfig_FAISSOptIn(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{},"embedding":{"vectorBackend":"faiss"}}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if got := loaded.Resolved.Embedding.VectorBackend; got != VectorBackendFAISS {
		t.Errorf("VectorBackend = %q, want %q", got, VectorBackendFAISS)
	}
}

func TestEmbeddingConfig_UnknownBackendRejected(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{},"embedding":{"vectorBackend":"qdrant"}}`)
	_, cerr := Load(path)
	if cerr == nil || cerr.Type != contract.ErrTypeConfigError {
		t.Fatalf("Load() error = %v, want CONFIG_ERROR", cerr)
	}
	if !strings.Contains(cerr.Message, "qdrant") || !strings.Contains(cerr.Message, "embedding.vectorBackend") {
		t.Errorf("error = %+v, want to name qdrant and the field", cerr)
	}
}

func TestSemanticSearch_DefaultsToEnabledWhenOmitted(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{}}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if !loaded.Resolved.Search.Semantic.Enabled {
		t.Errorf("Semantic.Enabled = false, want true when search.semantic is omitted")
	}
}

func TestSemanticSearch_DefaultsToEnabledWhenSearchSectionOmitted(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{},"search":{"lexical":{"enabled":true}}}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if !loaded.Resolved.Search.Semantic.Enabled {
		t.Errorf("Semantic.Enabled = false, want true when search.semantic is omitted but search.lexical is set")
	}
}

func TestSemanticSearch_ExplicitFalseDisables(t *testing.T) {
	path := writeTemp(t, "ozy.jsonc", `{"mcp":{},"search":{"semantic":{"enabled":false}}}`)
	loaded, cerr := Load(path)
	if cerr != nil {
		t.Fatalf("Load() error = %v", cerr)
	}
	if loaded.Resolved.Search.Semantic.Enabled {
		t.Errorf("Semantic.Enabled = true, want false when explicitly disabled")
	}
}
