// Package config loads, validates, and redacts Ozy's single configuration file
// (SPEC.md §11). Configuration is explicit and inspectable: secrets are supplied
// through ${ENV} references rather than literals, unresolved references are
// reported as diagnostics, and a redacted view is available for `ozy doctor`.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/rokask/ozy/internal/contract"
)

// Config is the typed configuration model mirroring SPEC.md §11.
type Config struct {
	Version   int                     `yaml:"version" json:"version"`
	Servers   map[string]ServerConfig `yaml:"servers" json:"servers"`
	Embedding EmbeddingConfig         `yaml:"embedding" json:"embedding"`
	Search    SearchConfig            `yaml:"search" json:"search"`
	Budgets   BudgetsConfig           `yaml:"budgets" json:"budgets"`
}

// ServerConfig describes one downstream MCP server.
type ServerConfig struct {
	Enabled   bool              `yaml:"enabled" json:"enabled"`
	Transport string            `yaml:"transport" json:"transport"`
	URL       string            `yaml:"url,omitempty" json:"url,omitempty"`
	Command   string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	Auth      *AuthConfig       `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// AuthConfig describes how to authenticate to a downstream server.
type AuthConfig struct {
	Type   string `yaml:"type" json:"type"`
	Header string `yaml:"header,omitempty" json:"header,omitempty"`
	Value  string `yaml:"value,omitempty" json:"value,omitempty"`
}

// EmbeddingConfig configures the optional embedding worker.
type EmbeddingConfig struct {
	Provider string `yaml:"provider,omitempty" json:"provider,omitempty"`
	Required bool   `yaml:"required" json:"required"`
}

// SearchConfig configures the lexical baseline and optional semantic search.
type SearchConfig struct {
	Lexical  LexicalSearch  `yaml:"lexical" json:"lexical"`
	Semantic SemanticSearch `yaml:"semantic" json:"semantic"`
}

// LexicalSearch toggles the mandatory lexical baseline.
type LexicalSearch struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// SemanticSearch toggles optional semantic search and whether it is required.
type SemanticSearch struct {
	Enabled  bool `yaml:"enabled" json:"enabled"`
	Required bool `yaml:"required" json:"required"`
}

// BudgetsConfig holds per-tool response budgets (SPEC.md §13).
type BudgetsConfig struct {
	FindTool     FindToolBudget     `yaml:"findTool" json:"findTool"`
	DescribeTool DescribeToolBudget `yaml:"describeTool" json:"describeTool"`
	CallTool     CallToolBudget     `yaml:"callTool" json:"callTool"`
}

// FindToolBudget bounds findTool responses.
type FindToolBudget struct {
	MaxResults         int  `yaml:"maxResults" json:"maxResults"`
	IncludeFullSchemas bool `yaml:"includeFullSchemas" json:"includeFullSchemas"`
}

// DescribeToolBudget bounds describeTool responses.
type DescribeToolBudget struct {
	IncludeExamples bool `yaml:"includeExamples" json:"includeExamples"`
}

// CallToolBudget bounds callTool result payloads.
type CallToolBudget struct {
	MaxResultBytes int `yaml:"maxResultBytes" json:"maxResultBytes"`
}

// MissingRef records an unresolved ${ENV} reference found during loading.
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

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// DefaultPath returns the default configuration location, honoring an OZY_CONFIG
// override before falling back to the user config dir.
func DefaultPath() string {
	if p := os.Getenv("OZY_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "ozy.yaml"
	}
	return filepath.Join(dir, "ozy", "config.yaml")
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
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, &contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        false,
			Message:          fmt.Sprintf("Invalid YAML in %s: %v", path, err),
			AgentInstruction: "Fix the YAML syntax reported above and retry.",
		}
	}

	if cerr := validate(&raw); cerr != nil {
		return nil, cerr
	}

	var resolved Config
	_ = yaml.Unmarshal(data, &resolved)
	missing := resolveEnv(&resolved)

	return &Loaded{Path: path, Raw: &raw, Resolved: &resolved, Missing: missing}, nil
}

// validate checks structural correctness and returns a CONFIG_ERROR naming the
// first offending field.
func validate(c *Config) *contract.Error {
	if c.Version != 1 {
		return configError(fmt.Sprintf("version must be 1, got %d", c.Version),
			"Set `version: 1` at the top of the configuration file.")
	}
	for id, s := range c.Servers {
		switch s.Transport {
		case "http":
			if s.URL == "" {
				return configError(fmt.Sprintf("server %q uses transport http but has no url", id),
					fmt.Sprintf("Add a `url` to server %q or change its transport.", id))
			}
		case "stdio":
			if s.Command == "" {
				return configError(fmt.Sprintf("server %q uses transport stdio but has no command", id),
					fmt.Sprintf("Add a `command` to server %q or change its transport.", id))
			}
		default:
			return configError(fmt.Sprintf("server %q has unknown transport %q", id, s.Transport),
				fmt.Sprintf("Set server %q transport to `http` or `stdio`.", id))
		}
	}
	return nil
}

func configError(msg, instruction string) *contract.Error {
	return &contract.Error{
		Type:             contract.ErrTypeConfigError,
		Retryable:        false,
		Message:          msg,
		AgentInstruction: instruction,
	}
}

// resolveEnv substitutes ${ENV} references in secret-bearing fields of c in place
// and returns any references that could not be resolved.
func resolveEnv(c *Config) []MissingRef {
	var missing []MissingRef
	for id, s := range c.Servers {
		if s.Auth != nil {
			s.Auth.Value, missing = substitute(s.Auth.Value, id, "auth.value", missing)
		}
		s.URL, missing = substitute(s.URL, id, "url", missing)
		for k, v := range s.Env {
			var sub string
			sub, missing = substitute(v, id, "env."+k, missing)
			s.Env[k] = sub
		}
		c.Servers[id] = s
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
