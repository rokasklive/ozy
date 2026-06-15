package bench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rokasklive/ozy/internal/eval"
)

// ToolSurface describes a single advertised tool at startup.
type ToolSurface struct {
	Name        string `json:"name"`
	Server      string `json:"server"`
	Description string `json:"description"`
	SchemaBytes int    `json:"schemaBytes"`
	SchemaTokens int   `json:"schemaTokens"`
}

// SurfaceMetrics captures the startup tool surface for one mode.
type SurfaceMetrics struct {
	Mode                  string        `json:"mode"`
	ToolsVisible          int           `json:"toolsVisible"`
	SchemaBytes           int           `json:"schemaBytes"`
	SchemaTokens          int           `json:"schemaTokens"`
	IrrelevantSchemaTokens int          `json:"irrelevantSchemaTokens"`
	Tools                 []ToolSurface `json:"tools"`
}

// SurfaceComparison compares the startup surfaces of direct and ozy modes.
type SurfaceComparison struct {
	Direct    SurfaceMetrics `json:"direct"`
	Ozy       SurfaceMetrics `json:"ozy"`
	Reduction struct {
		ToolCount     int     `json:"toolCount"`
		SchemaTokens  int     `json:"schemaTokens"`
		Ratio         float64 `json:"ratio"`
	} `json:"reduction"`
}

// MeasureSurface enumerates the tools advertised at startup for the given mode.
// It takes the list of tool surfaces discovered during MCP initialization.
func MeasureSurface(mode string, tools []ToolSurface, estimator eval.TokenEstimator) *SurfaceMetrics {
	m := &SurfaceMetrics{
		Mode:  mode,
		Tools: tools,
	}

	// Compute schema bytes and tokens for each tool.
	for i, t := range tools {
		m.SchemaBytes += t.SchemaBytes
		m.SchemaTokens += t.SchemaTokens
		// Distractor tools are irrelevant schema tokens.
		if isDistractorTool(t.Name) {
			m.IrrelevantSchemaTokens += t.SchemaTokens
		}
		tools[i] = t
	}

	m.ToolsVisible = len(tools)
	return m
}

// CompareSurfaces computes the reduction ratio between direct and ozy surfaces.
func CompareSurfaces(direct, ozy *SurfaceMetrics) *SurfaceComparison {
	c := &SurfaceComparison{
		Direct: *direct,
		Ozy:    *ozy,
	}

	if direct.SchemaTokens > 0 {
		c.Reduction.ToolCount = direct.ToolsVisible - ozy.ToolsVisible
		c.Reduction.SchemaTokens = direct.SchemaTokens - ozy.SchemaTokens
		c.Reduction.Ratio = float64(ozy.SchemaTokens) / float64(direct.SchemaTokens)
	}

	return c
}

// isDistractorTool reports whether a tool is a distractor (time, memory, notes).
func isDistractorTool(name string) bool {
	distractors := map[string]bool{
		"current_time":     true,
		"convert_timezone": true,
		"search_memory":    true,
		"store_memory":     true,
		"create_plan":      true,
		"append_note":      true,
	}
	return distractors[name]
}

// WriteSurfaceMetrics writes surface metrics to path.
func WriteSurfaceMetrics(path string, m *SurfaceMetrics) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create metrics dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create metrics file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return fmt.Errorf("encode metrics: %w", err)
	}
	return nil
}

// WriteComparison writes the surface comparison as JSON and Markdown.
func WriteComparison(dir string, c *SurfaceComparison) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create comparison dir: %w", err)
	}

	// JSON
	jsonPath := filepath.Join(dir, "comparison.json")
	fj, err := os.Create(jsonPath)
	if err != nil {
		return fmt.Errorf("create comparison.json: %w", err)
	}
	enc := json.NewEncoder(fj)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		fj.Close()
		return fmt.Errorf("encode comparison: %w", err)
	}
	fj.Close()

	// Markdown
	mdPath := filepath.Join(dir, "comparison.md")
	fm, err := os.Create(mdPath)
	if err != nil {
		return fmt.Errorf("create comparison.md: %w", err)
	}
	defer fm.Close()

	fmt.Fprintf(fm, "# Benchmark Comparison: %s vs %s\n\n", c.Direct.Mode, c.Ozy.Mode)
	fmt.Fprintf(fm, "## Startup Surface\n\n")
	fmt.Fprintf(fm, "| Metric | Direct | Ozy | Delta |\n")
	fmt.Fprintf(fm, "|--------|--------|-----|-------|\n")
	fmt.Fprintf(fm, "| Tools visible | %d | %d | %d |\n", c.Direct.ToolsVisible, c.Ozy.ToolsVisible, c.Reduction.ToolCount)
	fmt.Fprintf(fm, "| Schema tokens | %d | %d | %d |\n", c.Direct.SchemaTokens, c.Ozy.SchemaTokens, c.Reduction.SchemaTokens)
	fmt.Fprintf(fm, "| Irrelevant schema tokens | %d | %d | — |\n", c.Direct.IrrelevantSchemaTokens, c.Ozy.IrrelevantSchemaTokens)
	fmt.Fprintf(fm, "\n**Reduction ratio**: %.2f (%.0f%% of direct surface)\n", c.Reduction.Ratio, c.Reduction.Ratio*100)

	return nil
}
