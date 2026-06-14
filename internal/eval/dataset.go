// Package eval is Ozy's evaluation harness. It loads the committed corpus
// (a synthetic downstream MCP catalog plus labeled gold/scenario sets), drives
// the real broker, search engine, CLI, and MCP seams over it, computes the
// SPEC.md §14.2 metrics deterministically, and emits a machine-readable run
// result plus a human-readable benchmark scoreboard gated against thresholds.
//
// Every scenario is data, not Go code: adding a labeled intent means editing a
// file under evals/data/, never this package. The dataset schema lives in
// evals/README.md and the metric mathematics in evals/METHODOLOGY.md.
package eval

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
)

// Corpus is the fully loaded, validated eval corpus.
type Corpus struct {
	Catalog    Catalog
	Discovery  []DiscoveryCase
	Invocation []InvocationScenario
	Ergonomics []ErgonomicsCase
}

// Catalog is the synthetic downstream MCP catalog ("the world") the search legs
// rank against.
type Catalog struct {
	Version     int             `json:"version"`
	Description string          `json:"description"`
	Servers     []CatalogServer `json:"servers"`
	Tools       []CatalogTool   `json:"tools"`
}

// CatalogServer is one configured downstream server in the corpus.
type CatalogServer struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

// CatalogTool is one downstream tool entry in the corpus, mirroring the minimum
// metadata Ozy catalogs (SPEC.md §8).
type CatalogTool struct {
	ToolRef     string         `json:"toolRef"`
	ServerID    string         `json:"serverId"`
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Discovery categories. Each labeled intent declares one so metrics can be
// reported per category and category-specific gates applied.
const (
	CategoryLexical     = "lexical"
	CategorySemantic    = "semantic"
	CategoryNoMatch     = "no_match"
	CategoryAmbiguous   = "ambiguous"
	CategoryWrongServer = "wrong_server"
)

// DiscoveryCase is one labeled discovery intent: a user phrasing mapped to the
// acceptable target toolRef(s). An empty Acceptable means the capability is
// absent and the broker is expected to refuse (no-match correctness).
type DiscoveryCase struct {
	Intent     string   `json:"intent"`
	Category   string   `json:"category"`
	Acceptable []string `json:"acceptable"`
	Rationale  string   `json:"rationale"`
}

// Invocation outcomes. Each scenario declares exactly one.
const (
	// OutcomeSuccess: the first call with the given arguments should succeed.
	OutcomeSuccess = "success"
	// OutcomeRepair: the first call fails validation with ExpectedError, and the
	// Corrected arguments then succeed (the repair loop, SPEC.md §14.1).
	OutcomeRepair = "repair"
	// OutcomeError: the call terminates in the structured ExpectedError (e.g. an
	// offline server or schema drift) with no successful repair expected.
	OutcomeError = "error"
)

// InvocationScenario is one labeled callTool scenario (SPEC.md §14.1). Argument
// validity is judged against the cataloged tool's inputSchema; LiveSchema, when
// set, is the drifted downstream schema a TOOL_SCHEMA_CHANGED case detects.
type InvocationScenario struct {
	Name            string         `json:"name"`
	ToolRef         string         `json:"toolRef"`
	Arguments       map[string]any `json:"arguments"`
	ExpectedOutcome string         `json:"expectedOutcome"`
	ExpectedError   string         `json:"expectedError,omitempty"`
	Corrected       map[string]any `json:"corrected,omitempty"`
	LiveSchema      map[string]any `json:"liveSchema,omitempty"`
	Rationale       string         `json:"rationale"`
}

// Ergonomics case kinds — which broker operation the case exercises.
const (
	KindFind     = "find"
	KindDescribe = "describe"
	KindCall     = "call"
)

// ErgonomicsCase is one agent-facing input exercised on both the CLI and MCP
// surfaces for the structural §4.5/§9/§13 conformance checks and CLI↔MCP parity.
// ExpectDecision / ExpectErrorType are optional intent anchors; the conformance
// checks run regardless.
type ErgonomicsCase struct {
	Name            string         `json:"name"`
	Kind            string         `json:"kind"`
	Query           string         `json:"query,omitempty"`
	ToolRef         string         `json:"toolRef,omitempty"`
	Arguments       map[string]any `json:"arguments,omitempty"`
	ExpectDecision  string         `json:"expectDecision,omitempty"`
	ExpectErrorType string         `json:"expectErrorType,omitempty"`
	Rationale       string         `json:"rationale"`
}

// IndexedText is the natural-language text embedded for the semantic leg. It
// mirrors the SPEC.md §10.2 indexed fields a real index run would push to the
// sidecar, so the eval embeds tools the same way production does.
func (t CatalogTool) IndexedText() string {
	var b strings.Builder
	b.WriteString(t.Title)
	if t.Description != "" {
		b.WriteString(". ")
		b.WriteString(t.Description)
	}
	b.WriteString(" [")
	b.WriteString(t.ServerID)
	b.WriteString(".")
	b.WriteString(t.Name)
	b.WriteString("]")
	if props := schemaPropertyNames(t.InputSchema); len(props) > 0 {
		b.WriteString(" Fields: ")
		b.WriteString(strings.Join(props, ", "))
	}
	return b.String()
}

// schemaPropertyNames returns the sorted property names of a JSON-schema object.
func schemaPropertyNames(schema map[string]any) []string {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// validCategory reports whether c is a known discovery category.
func validCategory(c string) bool {
	switch c {
	case CategoryLexical, CategorySemantic, CategoryNoMatch, CategoryAmbiguous, CategoryWrongServer:
		return true
	default:
		return false
	}
}

// toolRefs returns the set of toolRefs present in the corpus catalog.
func (c *Corpus) toolRefs() map[string]struct{} {
	refs := make(map[string]struct{}, len(c.Catalog.Tools))
	for _, t := range c.Catalog.Tools {
		refs[t.ToolRef] = struct{}{}
	}
	return refs
}

// toolByRef returns the cataloged tool for a toolRef, if present.
func (c *Corpus) toolByRef(ref string) (CatalogTool, bool) {
	for _, t := range c.Catalog.Tools {
		if t.ToolRef == ref {
			return t, true
		}
	}
	return CatalogTool{}, false
}

// Store builds an in-memory catalog store populated with the corpus tools, ready
// for the search engine and broker to rank. Tools are marked online, callable,
// and fresh so the broker treats the corpus as a healthy live catalog.
func (c *Corpus) Store() (catalog.Store, error) {
	store := catalog.NewMemory()
	ctx := context.Background()
	status := func(s string) catalog.ServerStatus {
		switch s {
		case string(catalog.ServerOffline):
			return catalog.ServerOffline
		case string(catalog.ServerUnknown):
			return catalog.ServerUnknown
		default:
			return catalog.ServerOnline
		}
	}
	serverStatus := make(map[string]catalog.ServerStatus, len(c.Catalog.Servers))
	for _, s := range c.Catalog.Servers {
		serverStatus[s.ID] = status(s.Status)
		if err := store.PutServer(ctx, catalog.Server{ID: s.ID, Status: status(s.Status)}); err != nil {
			return nil, err
		}
	}
	for _, t := range c.Catalog.Tools {
		st, ok := serverStatus[t.ServerID]
		if !ok {
			st = catalog.ServerOnline
		}
		if err := store.PutTool(ctx, catalog.Tool{
			ToolRef:            t.ToolRef,
			ServerID:           t.ServerID,
			DownstreamToolName: t.Name,
			Title:              t.Title,
			Description:        t.Description,
			InputSchema:        t.InputSchema,
			ServerStatus:       st,
			CallableNow:        st == catalog.ServerOnline,
			Freshness:          catalog.FreshnessFresh,
			LastIndexedAt:      time.Now(),
		}); err != nil {
			return nil, err
		}
	}
	return store, nil
}
