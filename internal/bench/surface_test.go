package bench

import (
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

	direct := MeasureSurface("direct", directTools, estimator)
	ozy := MeasureSurface("ozy", ozyTools, estimator)

	if direct.ToolsVisible != 17 {
		t.Errorf("direct tools = %d, want 17", direct.ToolsVisible)
	}
	if ozy.ToolsVisible != 3 {
		t.Errorf("ozy tools = %d, want 3", ozy.ToolsVisible)
	}
	// 6 distractor tools (time: 2, memory: 2, notes: 2)
	// Expected: current_time(25) + convert_timezone(38) + search_memory(38) + store_memory(30) + create_plan(30) + append_note(25) = 186
	expectedDistractorTokens := 186
	if direct.IrrelevantSchemaTokens != expectedDistractorTokens {
		t.Errorf("direct irrelevant tokens = %d, want %d", direct.IrrelevantSchemaTokens, expectedDistractorTokens)
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

	comparisonDir := filepath.Join(dir, "comparison")
	if err := WriteComparison(comparisonDir, comparison); err != nil {
		t.Fatalf("WriteComparison: %v", err)
	}

	// Verify files were written.
	if _, err := os.Stat(metricsPath); err != nil {
		t.Errorf("metrics.json not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(comparisonDir, "comparison.json")); err != nil {
		t.Errorf("comparison.json not found: %v", err)
	}
	if _, err := os.Stat(filepath.Join(comparisonDir, "comparison.md")); err != nil {
		t.Errorf("comparison.md not found: %v", err)
	}
}
