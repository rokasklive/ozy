// Package config loads, validates, and redacts Ozy's single configuration file
// (SPEC.md §11). Configuration is explicit and inspectable: downstream servers
// are declared under the opencode-shaped `mcp` key, secrets are supplied through
// {env:NAME} references rather than literals, unresolved references are reported
// as diagnostics, and a redacted view is available for `ozy doctor`.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"github.com/tailscale/hujson"

	"github.com/rokasklive/ozy/internal/contract"
)

// DefaultDiscoveryTimeoutMillis matches opencode's default MCP tool discovery
// timeout when a server entry omits `timeout`.
const DefaultDiscoveryTimeoutMillis = 5000

// Config is the typed JSONC configuration model. Downstream MCP servers live in
// MCP, while Ozy-owned sections remain top-level siblings.
type Config struct {
	Schema    string                  `json:"$schema,omitempty"`
	Version   int                     `json:"version,omitempty"`
	MCP       map[string]ServerConfig `json:"mcp,omitempty"`
	Embedding EmbeddingConfig         `json:"embedding,omitempty"`
	Search    SearchConfig            `json:"search,omitempty"`
	Budgets   BudgetsConfig           `json:"budgets,omitempty"`
}

// ServerConfig describes one downstream MCP server using the opencode shape.
type ServerConfig struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	CWD         string            `json:"cwd,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	OAuth       json.RawMessage   `json:"oauth,omitempty"`
	Enabled     bool              `json:"enabled"`
	Timeout     int               `json:"timeout,omitempty"`
}

type serverConfigJSON struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command"`
	CWD         string            `json:"cwd"`
	Environment map[string]string `json:"environment"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers"`
	OAuth       json.RawMessage   `json:"oauth"`
	Enabled     *bool             `json:"enabled"`
	Timeout     *int              `json:"timeout"`
}

// UnmarshalJSON applies opencode MCP defaults: omitted `enabled` means enabled,
// and omitted `timeout` means the documented 5000ms discovery timeout.
func (s *ServerConfig) UnmarshalJSON(data []byte) error {
	var raw serverConfigJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	enabled := true
	if raw.Enabled != nil {
		enabled = *raw.Enabled
	}
	timeout := DefaultDiscoveryTimeoutMillis
	if raw.Timeout != nil {
		timeout = *raw.Timeout
	}
	*s = ServerConfig{
		Type:        raw.Type,
		Command:     append([]string(nil), raw.Command...),
		CWD:         raw.CWD,
		Environment: cloneStringMap(raw.Environment),
		URL:         raw.URL,
		Headers:     cloneStringMap(raw.Headers),
		OAuth:       append(json.RawMessage(nil), raw.OAuth...),
		Enabled:     enabled,
		Timeout:     timeout,
	}
	return nil
}

// IsEnabled reports whether Ozy should connect to this server.
func (s ServerConfig) IsEnabled() bool {
	return s.Enabled
}

// DiscoveryTimeout returns the per-server total discovery budget.
func (s ServerConfig) DiscoveryTimeout() time.Duration {
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = DefaultDiscoveryTimeoutMillis
	}
	return time.Duration(timeout) * time.Millisecond
}

// EmbeddingConfig configures the optional embedding worker.
type EmbeddingConfig struct {
	Provider string `json:"provider,omitempty"`
	Required bool   `json:"required"`
}

// SearchConfig configures the lexical baseline and optional semantic search.
type SearchConfig struct {
	Lexical  LexicalSearch  `json:"lexical,omitempty"`
	Semantic SemanticSearch `json:"semantic,omitempty"`
}

// LexicalSearch toggles the mandatory lexical baseline.
type LexicalSearch struct {
	Enabled bool `json:"enabled"`
}

// SemanticSearch toggles optional semantic search and whether it is required.
type SemanticSearch struct {
	Enabled  bool `json:"enabled"`
	Required bool `json:"required"`
}

// BudgetsConfig holds per-tool response budgets (SPEC.md §13).
type BudgetsConfig struct {
	FindTool     FindToolBudget     `json:"findTool,omitempty"`
	DescribeTool DescribeToolBudget `json:"describeTool,omitempty"`
	CallTool     CallToolBudget     `json:"callTool,omitempty"`
}

// FindToolBudget bounds findTool responses.
type FindToolBudget struct {
	MaxResults         int  `json:"maxResults"`
	IncludeFullSchemas bool `json:"includeFullSchemas"`
}

// DescribeToolBudget bounds describeTool responses.
type DescribeToolBudget struct {
	IncludeExamples bool `json:"includeExamples"`
}

// CallToolBudget bounds callTool result payloads.
type CallToolBudget struct {
	MaxResultBytes int `json:"maxResultBytes"`
}

// MissingRef records an unresolved {env:NAME} reference found during loading.
type MissingRef struct {
	Var    string `json:"var"`
	Server string `json:"server"`
	Field  string `json:"field"`
}

// Loaded is the outcome of loading configuration: the raw (pre-substitution)
// view safe to display, the resolved view for runtime use, the source path, and
// any unresolved environment references.
type Loaded struct {
	Path     string
	Raw      *Config
	Resolved *Config
	Missing  []MissingRef
}

var envRefPattern = regexp.MustCompile(`\{env:([A-Za-z_][A-Za-z0-9_]*)\}`)

// Home returns Ozy's user config directory.
func Home() string {
	if runtime.GOOS != "windows" {
		if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
			return configHomeFor(runtime.GOOS, xdg, "")
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, ".config", "ozy")
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil || dir == "" {
		return "ozy"
	}
	return configHomeFor(runtime.GOOS, "", dir)
}

func configHomeFor(goos, xdgConfigHome, userConfigDir string) string {
	if goos != "windows" && xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "ozy")
	}
	if userConfigDir == "" {
		return "ozy"
	}
	return filepath.Join(userConfigDir, "ozy")
}

// DefaultPath returns the default configuration location, honoring OZY_CONFIG
// before the OS user config directory.
func DefaultPath() string {
	if p := os.Getenv("OZY_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(Home(), "ozy.jsonc")
}

// Load reads, parses, validates, and resolves configuration at path. A missing
// file or any structural problem is returned as a CONFIG_ERROR with repair
// guidance (SPEC.md §9.3, §11).
func Load(path string) (*Loaded, *contract.Error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, &contract.Error{
				Type:             contract.ErrTypeConfigError,
				Retryable:        false,
				Message:          fmt.Sprintf("No configuration file at %s.", path),
				AgentInstruction: "Run `ozy init` to scaffold a configuration file, then edit it to add downstream servers.",
			}
		}
		return nil, &contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        true,
			Message:          fmt.Sprintf("Could not read %s: %v", path, err),
			AgentInstruction: "Check the path and file permissions, then retry.",
		}
	}

	var raw Config
	if err := unmarshalJSONC(data, &raw); err != nil {
		return nil, &contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        false,
			Message:          fmt.Sprintf("Invalid JSONC in %s: %v", path, err),
			AgentInstruction: "Fix the JSONC syntax reported above and retry.",
		}
	}

	if cerr := validate(&raw); cerr != nil {
		return nil, cerr
	}

	resolved := cloneConfig(raw)
	missing := resolveEnv(&resolved)

	return &Loaded{Path: path, Raw: &raw, Resolved: &resolved, Missing: missing}, nil
}

func unmarshalJSONC(data []byte, dst any) error {
	standard, err := hujson.Standardize(data)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(standard, dst); err != nil {
		return err
	}
	return nil
}

// validate checks structural correctness and returns a CONFIG_ERROR naming the
// first offending server and field.
func validate(c *Config) *contract.Error {
	for id, s := range c.MCP {
		if !s.IsEnabled() && s.Type == "" {
			continue
		}
		switch s.Type {
		case "local":
			if len(s.Command) == 0 || s.Command[0] == "" {
				return configError(id, "command",
					fmt.Sprintf("server %q has type local but has no command", id),
					fmt.Sprintf("Add a non-empty `command` array to server %q.", id))
			}
		case "remote":
			if s.URL == "" {
				return configError(id, "url",
					fmt.Sprintf("server %q has type remote but has no url", id),
					fmt.Sprintf("Add a `url` to server %q.", id))
			}
		default:
			return configError(id, "type",
				fmt.Sprintf("server %q has invalid type %q", id, s.Type),
				fmt.Sprintf("Set server %q type to `local` or `remote`.", id))
		}
	}
	return nil
}

func configError(server, field, msg, instruction string) *contract.Error {
	return &contract.Error{
		Type:             contract.ErrTypeConfigError,
		ServerID:         server,
		Retryable:        false,
		Message:          fmt.Sprintf("%s (field %s)", msg, field),
		AgentInstruction: instruction,
	}
}

// resolveEnv substitutes {env:NAME} references in secret-bearing MCP fields of
// c in place and returns any references that could not be resolved.
func resolveEnv(c *Config) []MissingRef {
	var missing []MissingRef
	for id, s := range c.MCP {
		for k, v := range s.Headers {
			var sub string
			sub, missing = substitute(v, id, "headers."+k, missing)
			s.Headers[k] = sub
		}
		for k, v := range s.Environment {
			var sub string
			sub, missing = substitute(v, id, "environment."+k, missing)
			s.Environment[k] = sub
		}
		c.MCP[id] = s
	}
	return missing
}

func substitute(value, server, field string, missing []MissingRef) (string, []MissingRef) {
	if value == "" {
		return value, missing
	}
	result := envRefPattern.ReplaceAllStringFunc(value, func(match string) string {
		name := envRefPattern.FindStringSubmatch(match)[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing = append(missing, MissingRef{Var: name, Server: server, Field: field})
		return match
	})
	return result, missing
}

func cloneConfig(in Config) Config {
	out := in
	out.MCP = make(map[string]ServerConfig, len(in.MCP))
	for id, s := range in.MCP {
		out.MCP[id] = cloneServerConfig(s)
	}
	return out
}

func cloneServerConfig(in ServerConfig) ServerConfig {
	out := in
	out.Command = append([]string(nil), in.Command...)
	out.Environment = cloneStringMap(in.Environment)
	out.Headers = cloneStringMap(in.Headers)
	out.OAuth = append(json.RawMessage(nil), in.OAuth...)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
