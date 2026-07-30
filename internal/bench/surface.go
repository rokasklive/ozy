package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/eval"
	ozymcp "github.com/rokasklive/ozy/internal/mcp"
)

// ToolSurface describes a single advertised tool at startup.
type ToolSurface struct {
	Name         string `json:"name"`
	Server       string `json:"server"`
	Description  string `json:"description"`
	SchemaBytes  int    `json:"schemaBytes"`
	SchemaTokens int    `json:"schemaTokens"`
}

// SurfaceMetrics captures the startup tool surface for one mode.
type SurfaceMetrics struct {
	Mode                   string        `json:"mode"`
	ToolsVisible           int           `json:"toolsVisible"`
	SchemaBytes            int           `json:"schemaBytes"`
	SchemaTokens           int           `json:"schemaTokens"`
	IrrelevantSchemaTokens int           `json:"irrelevantSchemaTokens"`
	Tools                  []ToolSurface `json:"tools"`
}

// SurfaceReduction is the tool-count / schema-token delta of a reduced surface
// (ozy) against a fuller one (direct, or direct-lean). Ratio is reduced/base.
type SurfaceReduction struct {
	ToolCount    int     `json:"toolCount"`
	SchemaTokens int     `json:"schemaTokens"`
	Ratio        float64 `json:"ratio"`
}

// SurfaceComparison compares the startup surfaces of the direct, direct-lean, and
// ozy modes. DirectLean/LeanReduction are omitted when the lean surface was not
// computed (kept optional so every existing reader stays valid).
type SurfaceComparison struct {
	Estimator     string            `json:"estimator"`
	Direct        SurfaceMetrics    `json:"direct"`
	DirectLean    *SurfaceMetrics   `json:"directLean,omitempty"`
	Ozy           SurfaceMetrics    `json:"ozy"`
	Reduction     SurfaceReduction  `json:"reduction"`               // ozy vs direct
	LeanReduction *SurfaceReduction `json:"leanReduction,omitempty"` // ozy vs direct-lean
}

// ComputeSurfaceComparison enumerates the startup tool surface of both modes
// in-process — no model, no subprocesses. Direct mode connects an in-memory
// client to each scenario server (functional toolset or corpus stub); ozy mode
// connects to the real MCP adapter's three-tool surface. Schema tokens are
// estimated with est. The corpus is counted identically for both modes: it is
// part of direct's advertised surface and of what ozy indexes.
func ComputeSurfaceComparison(servers []benchServer, fixtureDir, corpusDir string, canonical []RequiredTool, est eval.TokenEstimator) (*SurfaceComparison, error) {
	if est == nil {
		est = eval.DefaultEstimator
	}
	ctx := context.Background()

	// directTools is the full estate; leanTools is the functional-only subset
	// (corpus servers dropped) — the direct-lean surface.
	var directTools, leanTools []ToolSurface
	for _, bs := range servers {
		srv, err := newBenchServer(bs, fixtureDir, corpusDir)
		if err != nil {
			return nil, fmt.Errorf("build %s server: %w", bs.Name, err)
		}
		tools, err := enumerateServerTools(ctx, srv, bs.Name, est)
		if err != nil {
			return nil, fmt.Errorf("enumerate %s tools: %w", bs.Name, err)
		}
		directTools = append(directTools, tools...)
		if !bs.Corpus {
			leanTools = append(leanTools, tools...)
		}
	}

	// ozy mode: the real adapter surface. A nil broker is safe — ListTools does
	// not invoke the handlers, and the three tool schemas are static.
	adapter := ozymcp.New(ozymcp.StaticProvider(nil), Version, "")
	ozyTools, err := enumerateServerTools(ctx, adapter.Server(), "ozy", est)
	if err != nil {
		return nil, fmt.Errorf("enumerate ozy tools: %w", err)
	}

	direct := MeasureSurface("direct", directTools, canonical, est)
	lean := MeasureSurface("direct-lean", leanTools, canonical, est)
	ozy := MeasureSurface("ozy", ozyTools, canonical, est)
	c := CompareSurfaces(direct, ozy)
	c.DirectLean = lean
	leanRed := surfaceReduction(lean, ozy)
	c.LeanReduction = &leanRed
	c.Estimator = est.Name()
	return c, nil
}

// enumerateServerTools connects an in-memory client to srv, lists its tools, and
// returns a ToolSurface per tool with schema bytes/tokens from the advertised
// {name, description, inputSchema} document.
func enumerateServerTools(ctx context.Context, srv *mcpsdk.Server, server string, est eval.TokenEstimator) ([]ToolSurface, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverT, clientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "ozy-bench-surface", Version: Version}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.ListTools(ctx, &mcpsdk.ListToolsParams{})
	if err != nil {
		return nil, fmt.Errorf("list tools: %w", err)
	}

	var out []ToolSurface
	for _, t := range res.Tools {
		doc := map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema}
		schema, _ := json.Marshal(doc)
		out = append(out, ToolSurface{
			Name:         t.Name,
			Server:       server,
			Description:  t.Description,
			SchemaBytes:  len(schema),
			SchemaTokens: est.Estimate(string(schema)),
		})
	}
	return out, nil
}

// WriteSurface writes the surface comparison to surface.json.
func WriteSurface(path string, c *SurfaceComparison) error {
	return writeJSON(path, c)
}

// MeasureSurface enumerates the tools advertised at startup for the given mode.
// Irrelevant schema tokens are those spent on tools outside the scenario's
// canonical set — with a ≥500-tool estate this is corpus/distractor bloat, not a
// hardcoded distractor name list.
func MeasureSurface(mode string, tools []ToolSurface, canonical []RequiredTool, estimator eval.TokenEstimator) *SurfaceMetrics {
	m := &SurfaceMetrics{
		Mode:  mode,
		Tools: tools,
	}

	for _, t := range tools {
		m.SchemaBytes += t.SchemaBytes
		m.SchemaTokens += t.SchemaTokens
		if !isCanonicalTool(t, canonical) {
			m.IrrelevantSchemaTokens += t.SchemaTokens
		}
	}

	m.ToolsVisible = len(tools)
	return m
}

// isCanonicalTool reports whether a surface tool is one of the scenario's
// canonical (required) tools.
func isCanonicalTool(t ToolSurface, canonical []RequiredTool) bool {
	for _, rt := range canonical {
		if rt.Server != "" && rt.Server != t.Server {
			continue
		}
		if t.Name == rt.Tool || strings.HasSuffix(t.Name, "_"+rt.Tool) {
			return true
		}
	}
	return false
}

// CompareSurfaces computes the reduction ratio between direct and ozy surfaces.
func CompareSurfaces(direct, ozy *SurfaceMetrics) *SurfaceComparison {
	return &SurfaceComparison{
		Direct:    *direct,
		Ozy:       *ozy,
		Reduction: surfaceReduction(direct, ozy),
	}
}

// surfaceReduction computes how much smaller the reduced surface is than base:
// the tool-count and schema-token deltas, and the reduced/base token ratio.
func surfaceReduction(base, reduced *SurfaceMetrics) SurfaceReduction {
	var r SurfaceReduction
	if base.SchemaTokens > 0 {
		r.ToolCount = base.ToolsVisible - reduced.ToolsVisible
		r.SchemaTokens = base.SchemaTokens - reduced.SchemaTokens
		r.Ratio = float64(reduced.SchemaTokens) / float64(base.SchemaTokens)
	}
	return r
}

// WriteSurfaceMetrics writes surface metrics to path.
//
//nolint:gosec // G301: 0755 permissions are intentional for bench output directories.
func WriteSurfaceMetrics(path string, m *SurfaceMetrics) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metrics dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create metrics file: %w", err)
	}
	defer func() { _ = f.Close() }()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encode metrics: %w", err)
	}
	return nil
}
