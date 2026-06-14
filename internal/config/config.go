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

// VectorBackend names the on-disk vector index implementation used by the
// embedding sidecar. turbovec is the zero-config default; faiss is an opt-in
// alternative chosen before the first index is built.
const (
	VectorBackendTurbovec = "turbovec"
	VectorBackendFAISS    = "faiss"
)

// DefaultVectorBackend is the resolved vector backend when configuration omits
// the field. It must match the default the proposal promises so a user who
// enables semantic search without picking a backend gets turbovec.
const DefaultVectorBackend = VectorBackendTurbovec

// DefaultEmbeddingModel is the FastEmbed model id used when configuration does
// not name one. Documented in SPEC.md §10.4 and pinned in the sidecar.
const DefaultEmbeddingModel = "BAAI/bge-small-en-v1.5"

// embeddingConfigJSON is the raw JSON form of EmbeddingConfig, used so that
// omitted fields can be distinguished from explicit falsy values. VectorBackend
// and Model default at the type level (the zero value is empty), so we apply
// the documented defaults in applyDefaults below.
type embeddingConfigJSON struct {
	Provider      string `json:"provider"`
	Required      bool   `json:"required"`
	VectorBackend string `json:"vectorBackend"`
	Model         string `json:"model"`
}

// EmbeddingConfig configures the optional embedding worker and its vector
// index. VectorBackend defaults to "turbovec"; Model defaults to the
// FastEmbed CPU-friendly default. The vector dimension is derived from the
// selected model at runtime by the sidecar — it is not configured.
type EmbeddingConfig struct {
	Provider      string `json:"provider,omitempty"`
	Required      bool   `json:"required"`
	VectorBackend string `json:"vectorBackend,omitempty"`
	Model         string `json:"model,omitempty"`
}

// UnmarshalJSON applies the documented defaults for omitted fields.
func (e *EmbeddingConfig) UnmarshalJSON(data []byte) error {
	var raw embeddingConfigJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Provider = raw.Provider
	e.Required = raw.Required
	e.VectorBackend = raw.VectorBackend
	if e.VectorBackend == "" {
		e.VectorBackend = DefaultVectorBackend
	}
	e.Model = raw.Model
	if e.Model == "" {
		e.Model = DefaultEmbeddingModel
	}
	return nil
}

// SearchConfig configures the lexical baseline and optional semantic search.
// When the `semantic` section is entirely omitted, semantic search is treated
// as enabled (the default-on behavior) — see UnmarshalJSON.
type SearchConfig struct {
	Lexical  LexicalSearch  `json:"lexical,omitempty"`
	Semantic SemanticSearch `json:"semantic,omitempty"`
}

// searchConfigJSON lets us distinguish "search.semantic omitted" from
// "search.semantic present with all defaults".
type searchConfigJSON struct {
	Lexical  *LexicalSearch  `json:"lexical"`
	Semantic *SemanticSearch `json:"semantic"`
}

// UnmarshalJSON applies the default-on semantic search when the semantic
// sub-section is omitted. A pointer distinguishes omitted from explicit zero.
func (s *SearchConfig) UnmarshalJSON(data []byte) error {
	var raw searchConfigJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Lexical != nil {
		s.Lexical = *raw.Lexical
	} else {
		s.Lexical = LexicalSearch{Enabled: true}
	}
	if raw.Semantic != nil {
		s.Semantic = *raw.Semantic
	} else {
		s.Semantic = SemanticSearch{Enabled: true}
	}
	return nil
}

// LexicalSearch toggles the mandatory lexical baseline.
type LexicalSearch struct {
	Enabled bool `json:"enabled"`
}

// semanticSearchJSON is the raw JSON form of SemanticSearch so that an omitted
// `enabled` field can default to true (semantic on by default) while an
// explicit `false` disables it.
type semanticSearchJSON struct {
	Enabled  *bool `json:"enabled"`
	Required bool  `json:"required"`
}

// SemanticSearch toggles optional semantic search and whether it is required.
// When `enabled` is omitted, semantic search is treated as ON (default-on for
// the out-of-the-box hybrid experience). Set `enabled: false` explicitly to
// opt back into lexical-only.
type SemanticSearch struct {
	Enabled  bool `json:"enabled"`
	Required bool `json:"required"`
}

// UnmarshalJSON applies the default-on semantic semantics.
func (s *SemanticSearch) UnmarshalJSON(data []byte) error {
	var raw semanticSearchJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw.Enabled == nil {
		s.Enabled = true
	} else {
		s.Enabled = *raw.Enabled
	}
	s.Required = raw.Required
	return nil
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

	applyDefaults(&raw, sectionPresent(data, "search"))

	resolved := cloneConfig(raw)
	missing := resolveEnv(&resolved)

	return &Loaded{Path: path, Raw: &raw, Resolved: &resolved, Missing: missing}, nil
}

// sectionPresent reports whether a top-level key exists in the JSONC document.
// Used to distinguish "omitted" from "present with zero value" for sections
// that drive default-on semantics (e.g. `search`).
func sectionPresent(data []byte, key string) bool {
	standard, err := hujson.Standardize(data)
	if err != nil {
		return false
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(standard, &probe); err != nil {
		return false
	}
	_, ok := probe[key]
	return ok
}

// applyDefaults resolves the documented defaults for fields the user omitted.
// UnmarshalJSON handles per-section omission of optional fields; this catches
// the case where the entire section (e.g. `embedding` or `search.semantic`) is
// missing from the JSON document.
func applyDefaults(c *Config, searchPresent bool) {
	if c.Embedding.VectorBackend == "" {
		c.Embedding.VectorBackend = DefaultVectorBackend
	}
	if c.Embedding.Model == "" {
		c.Embedding.Model = DefaultEmbeddingModel
	}
	// When search is entirely omitted we have no signal that the user wanted
	// the lexical-only escape hatch; treat the default as default-on semantic.
	if !searchPresent {
		c.Search.Semantic.Enabled = true
	}
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
	switch c.Embedding.VectorBackend {
	case "", VectorBackendTurbovec, VectorBackendFAISS:
		// ok — empty resolves to the documented default at the type level
	default:
		return configError("", "embedding.vectorBackend",
			fmt.Sprintf("embedding.vectorBackend %q is not a known backend", c.Embedding.VectorBackend),
			fmt.Sprintf("Set embedding.vectorBackend to %q or %q.", VectorBackendTurbovec, VectorBackendFAISS))
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
