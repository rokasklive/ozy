package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rokasklive/ozy/internal/eval"
)

func TestSurfaceMetrics(t *testing.T) {
	t.Parallel()

	estimator := eval.DefaultEstimator

	directTools := []ToolSurface{
		{Name: "search_text", Server: "code-search", Description: "search code", SchemaBytes: 200, SchemaTokens: 50},
		{Name: "search_symbol", Server: "code-search", Description: "search symbols", SchemaBytes: 200, SchemaTokens: 50},
		{Name: "read_file", Server: "code-search", Description: "read file", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "find_references", Server: "code-search", Description: "find refs", SchemaBytes: 200, SchemaTokens: 50},
		{Name: "git_log", Server: "git", Description: "git log", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "git_show", Server: "git", Description: "git show", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "git_blame", Server: "git", Description: "git blame", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "git_diff", Server: "git", Description: "git diff", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "list_tables", Server: "incident-db", Description: "list tables", SchemaBytes: 120, SchemaTokens: 30},
		{Name: "describe_table", Server: "incident-db", Description: "describe table", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "query_readonly", Server: "incident-db", Description: "query readonly", SchemaBytes: 180, SchemaTokens: 45},
		{Name: "current_time", Server: "time", Description: "current time", SchemaBytes: 100, SchemaTokens: 25},
		{Name: "convert_timezone", Server: "time", Description: "convert timezone", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "search_memory", Server: "memory", Description: "search memory", SchemaBytes: 150, SchemaTokens: 38},
		{Name: "store_memory", Server: "memory", Description: "store memory", SchemaBytes: 120, SchemaTokens: 30},
		{Name: "create_plan", Server: "notes", Description: "create plan", SchemaBytes: 120, SchemaTokens: 30},
		{Name: "append_note", Server: "notes", Description: "append note", SchemaBytes: 100, SchemaTokens: 25},
	}

	ozyTools := []ToolSurface{
		{Name: "findTool", Server: "ozy", Description: "find a tool", SchemaBytes: 250, SchemaTokens: 63},
		{Name: "describeTool", Server: "ozy", Description: "describe a tool", SchemaBytes: 200, SchemaTokens: 50},
		{Name: "callTool", Server: "ozy", Description: "call a tool", SchemaBytes: 200, SchemaTokens: 50},
	}

	// Canonical set: only these two tools are "relevant"; everything else counts
	// as irrelevant schema tokens (corpus/distractor bloat at scale).
	canonical := []RequiredTool{
		{Server: "code-search", Tool: "search_text"},
		{Server: "git", Tool: "git_show"},
	}
	direct := MeasureSurface("direct", directTools, canonical, estimator)
	ozy := MeasureSurface("ozy", ozyTools, canonical, estimator)

	if direct.ToolsVisible != 17 {
		t.Errorf("direct tools = %d, want 17", direct.ToolsVisible)
	}
	if ozy.ToolsVisible != 3 {
		t.Errorf("ozy tools = %d, want 3", ozy.ToolsVisible)
	}
	// Irrelevant = total (639) minus the two canonical tools: search_text(50) +
	// git_show(38) = 88 → 551.
	if direct.IrrelevantSchemaTokens != 551 {
		t.Errorf("direct irrelevant tokens = %d, want 551", direct.IrrelevantSchemaTokens)
	}
	// None of ozy's broker tools are canonical, so all of its surface is irrelevant.
	if ozy.IrrelevantSchemaTokens != ozy.SchemaTokens {
		t.Errorf("ozy irrelevant tokens = %d, want %d (all)", ozy.IrrelevantSchemaTokens, ozy.SchemaTokens)
	}

	comparison := CompareSurfaces(direct, ozy)
	if comparison.Reduction.ToolCount != 14 {
		t.Errorf("reduction tool count = %d, want 14", comparison.Reduction.ToolCount)
	}
	if comparison.Reduction.Ratio <= 0 || comparison.Reduction.Ratio >= 1 {
		t.Errorf("reduction ratio = %f, expected between 0 and 1", comparison.Reduction.Ratio)
	}

	dir := t.TempDir()
	metricsDir := filepath.Join(dir, "direct")
	metricsPath := filepath.Join(metricsDir, "metrics.json")
	if err := WriteSurfaceMetrics(metricsPath, direct); err != nil {
		t.Fatalf("WriteSurfaceMetrics: %v", err)
	}

	surfacePath := filepath.Join(dir, "surface.json")
	if err := WriteSurface(surfacePath, comparison); err != nil {
		t.Fatalf("WriteSurface: %v", err)
	}

	// Verify files were written.
	if _, err := os.Stat(metricsPath); err != nil {
		t.Errorf("metrics.json not found: %v", err)
	}
	if _, err := os.Stat(surfacePath); err != nil {
		t.Errorf("surface.json not found: %v", err)
	}
}

// computeTestSurface builds a small direct-vs-ozy surface (functional toolsets,
// no corpus) for reporting tests that just need a populated surface.
func computeTestSurface(t *testing.T) (*SurfaceComparison, error) {
	t.Helper()
	servers, err := scenarioServers(
		[]string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
		false, t.TempDir(), "")
	if err != nil {
		return nil, err
	}
	return ComputeSurfaceComparison(servers, t.TempDir(), "", nil, eval.DefaultEstimator)
}

// TestComputeSurfaceComparison exercises the in-process enumeration of both
// modes' startup surfaces — no model, no subprocesses.
func TestComputeSurfaceComparison(t *testing.T) {
	t.Parallel()

	servers, err := scenarioServers(
		[]string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
		false, t.TempDir(), "")
	if err != nil {
		t.Fatalf("scenarioServers: %v", err)
	}
	c, err := ComputeSurfaceComparison(servers, t.TempDir(), "", nil, eval.DefaultEstimator)
	if err != nil {
		t.Fatalf("ComputeSurfaceComparison: %v", err)
	}
	// Ozy advertises exactly its three broker tools.
	if c.Ozy.ToolsVisible != 3 {
		t.Errorf("ozy tools = %d, want 3", c.Ozy.ToolsVisible)
	}
	// Direct advertises all seven fixture toolsets' tools — many more than ozy.
	if c.Direct.ToolsVisible <= c.Ozy.ToolsVisible {
		t.Errorf("direct tools = %d, want > ozy (%d)", c.Direct.ToolsVisible, c.Ozy.ToolsVisible)
	}
	if c.Direct.SchemaTokens <= c.Ozy.SchemaTokens {
		t.Errorf("direct schema tokens = %d, want > ozy (%d)", c.Direct.SchemaTokens, c.Ozy.SchemaTokens)
	}
	if c.Estimator == "" {
		t.Error("estimator not recorded")
	}
	// With corpus disabled the lean surface equals the full functional surface.
	if c.DirectLean == nil || c.DirectLean.ToolsVisible != c.Direct.ToolsVisible {
		t.Errorf("no-corpus lean surface = %v, want equal to direct (%d tools)", c.DirectLean, c.Direct.ToolsVisible)
	}
}

// writeTestCorpusServer writes a minimal valid corpus server file with nTools
// stub tools, so a scenario can attach a synthetic corpus in tests.
func writeTestCorpusServer(t *testing.T, dir, name string, nTools int) {
	t.Helper()
	tools := make([]map[string]any, nTools)
	for i := range tools {
		tools[i] = map[string]any{
			"name":        fmt.Sprintf("%s_tool_%d", name, i),
			"description": "corpus stub tool",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		}
	}
	doc := map[string]any{"server": name, "mirrorSource": "test/fixture", "tools": tools}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal corpus server: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644); err != nil {
		t.Fatalf("write corpus server: %v", err)
	}
}

// TestDirectLeanSurfaceBetweenDirectAndOzy asserts the lean surface (functional
// toolsets, corpus dropped) sits strictly between the full direct surface and the
// ozy broker surface when a corpus attaches.
func TestDirectLeanSurfaceBetweenDirectAndOzy(t *testing.T) {
	t.Parallel()

	corpusDir := t.TempDir()
	writeTestCorpusServer(t, corpusDir, "distractor-a", 3)
	writeTestCorpusServer(t, corpusDir, "distractor-b", 4)

	servers, err := scenarioServers(
		[]string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
		true, t.TempDir(), corpusDir)
	if err != nil {
		t.Fatalf("scenarioServers: %v", err)
	}
	c, err := ComputeSurfaceComparison(servers, t.TempDir(), corpusDir, nil, eval.DefaultEstimator)
	if err != nil {
		t.Fatalf("ComputeSurfaceComparison: %v", err)
	}
	if c.DirectLean == nil || c.LeanReduction == nil {
		t.Fatal("lean surface / reduction not computed")
	}
	// ozy < direct-lean < direct in both tool count and schema tokens.
	if c.Ozy.ToolsVisible >= c.DirectLean.ToolsVisible || c.DirectLean.ToolsVisible >= c.Direct.ToolsVisible {
		t.Errorf("lean tools %d not between ozy %d and direct %d", c.DirectLean.ToolsVisible, c.Ozy.ToolsVisible, c.Direct.ToolsVisible)
	}
	if c.Ozy.SchemaTokens >= c.DirectLean.SchemaTokens || c.DirectLean.SchemaTokens >= c.Direct.SchemaTokens {
		t.Errorf("lean tokens %d not between ozy %d and direct %d", c.DirectLean.SchemaTokens, c.Ozy.SchemaTokens, c.Direct.SchemaTokens)
	}
	// The tool delta between direct and lean is exactly the 7 corpus tools.
	if delta := c.Direct.ToolsVisible - c.DirectLean.ToolsVisible; delta != 7 {
		t.Errorf("direct−lean tool delta = %d, want 7 corpus tools", delta)
	}
	// LeanReduction is ozy relative to the lean surface (a proper reduction).
	if c.LeanReduction.Ratio <= 0 || c.LeanReduction.Ratio >= 1 {
		t.Errorf("lean reduction ratio = %f, want between 0 and 1", c.LeanReduction.Ratio)
	}
}
