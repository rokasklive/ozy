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

	"github.com/tailscale/hujson"

	"github.com/rokask/ozy/internal/contract"
)

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
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
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

// DefaultPath returns the default configuration location, honoring OZY_CONFIG
// before project-local ozy.jsonc/ozy.json and then the user config dir.
func DefaultPath() string {
	if p := os.Getenv("OZY_CONFIG"); p != "" {
		return p
	}
	for _, p := range []string{"ozy.jsonc", "ozy.json"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "ozy.jsonc"
	}
	return filepath.Join(dir, "ozy", "ozy.jsonc")
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
	if in.Environment != nil {
		out.Environment = make(map[string]string, len(in.Environment))
		for k, v := range in.Environment {
			out.Environment[k] = v
		}
	}
	if in.Headers != nil {
		out.Headers = make(map[string]string, len(in.Headers))
		for k, v := range in.Headers {
			out.Headers[k] = v
		}
	}
	return out
}
