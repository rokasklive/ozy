package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CorpusTool is one tool definition in a corpus server file. InputSchema is a
// raw JSON Schema object (served verbatim); CannedResponse, when present,
// overrides the generated deterministic stub response.
//
// Behavior selects how the corpus server answers a call, so a same-capability
// rival behaves like the real service it mirrors (D3):
//   - "" / "stub"        — deterministic generic stub carrying no scenario fact.
//   - "auth_error"       — a realistic authentication-required error (auth-gated
//     services a keyless agent cannot use), so the pick fails legibly.
//   - "functional:<b>"   — real data via the shared backend <b> ("search" ranks
//     the fixture corpus; "pdf" writes a PDF to the agent workspace), for no-auth
//     same-capability rivals that would really complete the task.
type CorpusTool struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	InputSchema    json.RawMessage `json:"inputSchema"`
	Behavior       string          `json:"behavior,omitempty"`
	CannedResponse json.RawMessage `json:"cannedResponse,omitempty"`
}

// corpusBackends are the functional backends the corpus server can route to.
var corpusBackends = map[string]bool{"search": true, "pdf": true}

// validCorpusBehavior reports whether a CorpusTool.Behavior value is one the
// corpus server knows how to route.
func validCorpusBehavior(b string) bool {
	switch {
	case b == "" || b == "stub" || b == "auth_error":
		return true
	case strings.HasPrefix(b, "functional:"):
		return corpusBackends[strings.TrimPrefix(b, "functional:")]
	default:
		return false
	}
}

// CorpusServer is one data-defined fixture MCP server: a name, its mirror
// provenance, and its tools. It is served by the generic corpus server mode
// (`ozy-bench mcp --server <name> --corpus-dir <dir>`) — adding a server is a
// data change, no Go edit.
type CorpusServer struct {
	Server       string       `json:"server"`
	MirrorSource string       `json:"mirrorSource"`
	Tools        []CorpusTool `json:"tools"`
}

// corpusSchema is the minimal JSON Schema shape the corpus validator inspects:
// an object with typed, described properties.
type corpusSchema struct {
	Type       string `json:"type"`
	Properties map[string]struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	} `json:"properties"`
}

// Validate checks structural realism invariants shared by the loader and the
// corpus floor test: required fields present, unique tool names, and every
// schema an object whose properties are each typed and described.
func (s *CorpusServer) Validate() error {
	if s.Server == "" {
		return fmt.Errorf("corpus server: missing server name")
	}
	if s.MirrorSource == "" {
		return fmt.Errorf("corpus server %q: missing mirrorSource", s.Server)
	}
	if len(s.Tools) == 0 {
		return fmt.Errorf("corpus server %q: no tools", s.Server)
	}
	seen := map[string]bool{}
	for _, t := range s.Tools {
		if t.Name == "" {
			return fmt.Errorf("corpus server %q: tool with empty name", s.Server)
		}
		if seen[t.Name] {
			return fmt.Errorf("corpus server %q: duplicate tool name %q", s.Server, t.Name)
		}
		seen[t.Name] = true
		if strings.TrimSpace(t.Description) == "" {
			return fmt.Errorf("corpus server %q tool %q: missing description", s.Server, t.Name)
		}
		if !validCorpusBehavior(t.Behavior) {
			return fmt.Errorf("corpus server %q tool %q: invalid behavior %q", s.Server, t.Name, t.Behavior)
		}
		var sch corpusSchema
		if err := json.Unmarshal(t.InputSchema, &sch); err != nil {
			return fmt.Errorf("corpus server %q tool %q: invalid inputSchema: %w", s.Server, t.Name, err)
		}
		if sch.Type != "object" {
			return fmt.Errorf("corpus server %q tool %q: inputSchema type must be \"object\", got %q", s.Server, t.Name, sch.Type)
		}
		for prop, spec := range sch.Properties {
			if spec.Type == "" {
				return fmt.Errorf("corpus server %q tool %q: property %q lacks a type", s.Server, t.Name, prop)
			}
			if strings.TrimSpace(spec.Description) == "" {
				return fmt.Errorf("corpus server %q tool %q: property %q lacks a description", s.Server, t.Name, prop)
			}
		}
	}
	return nil
}

// LoadCorpusServer reads and validates one corpus server file. The file's base
// name (minus .json) must equal its declared server name so the wiring can map
// `--server <name>` to `<name>.json` without an index.
//
//nolint:gosec // G304: path comes from the trusted corpus dir, not user input.
func LoadCorpusServer(path string) (*CorpusServer, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read corpus server: %w", err)
	}
	var s CorpusServer
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", path, err)
	}
	base := strings.TrimSuffix(filepath.Base(path), ".json")
	if base != s.Server {
		return nil, fmt.Errorf("corpus file %s declares server %q — filename must match", path, s.Server)
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	return &s, nil
}

// LoadCorpus loads every *.json corpus server under dir, sorted by name for
// determinism. A missing dir yields an empty corpus (a scenario may run with
// corpus disabled), never an error.
func LoadCorpus(dir string) ([]CorpusServer, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob corpus: %w", err)
	}
	sort.Strings(paths)
	var out []CorpusServer
	for _, p := range paths {
		s, err := LoadCorpusServer(p)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

// CorpusServerNeedsFixture reports whether the named corpus server hosts a tool
// whose functional backend reads the baked fixture dir (currently only
// `functional:search`), so the runner passes `--fixture-dir` to exactly those
// corpus servers. `functional:pdf` writes to the agent workspace and needs none.
func CorpusServerNeedsFixture(corpusDir, name string) bool {
	cs, err := LoadCorpusServer(filepath.Join(corpusDir, name+".json"))
	if err != nil {
		return false
	}
	for _, t := range cs.Tools {
		if t.Behavior == "functional:search" {
			return true
		}
	}
	return false
}

// CorpusServerNames lists the corpus server names under dir (sorted). It is the
// cheap enumeration the runner and surface use to wire the corpus into both
// modes without loading every tool.
func CorpusServerNames(dir string) ([]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("glob corpus: %w", err)
	}
	sort.Strings(paths)
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	return names, nil
}
