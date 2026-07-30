package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/eval"
)

// weatherFixtureDir is the checked-in baked fixture for the weather scenario,
// relative to this package directory.
const weatherFixtureDir = "../../bench/scenarios/historical-weather-report/fixture"

// callFixtureTool connects an in-memory client to a fixture toolset and calls one
// tool, returning the decoded JSON result object.
func callFixtureTool(t *testing.T, toolset, fixtureDir, tool string, args map[string]any) map[string]any {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv, err := newMCPServer(toolset, fixtureDir)
	if err != nil {
		t.Fatalf("newMCPServer(%s): %v", toolset, err)
	}
	serverT, clientT := mcpsdk.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = cs.Close() }()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("call %s/%s: %v", toolset, tool, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("call %s/%s: empty content", toolset, tool)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("call %s/%s: non-text content", toolset, tool)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("call %s/%s: result not JSON object: %v (%q)", toolset, tool, err, tc.Text)
	}
	return out
}

func toolNames(t *testing.T, toolset, fixtureDir string) []string {
	t.Helper()
	srv, err := newMCPServer(toolset, fixtureDir)
	if err != nil {
		t.Fatalf("newMCPServer(%s): %v", toolset, err)
	}
	tools, err := enumerateServerTools(context.Background(), srv, toolset, eval.DefaultEstimator)
	if err != nil {
		t.Fatalf("enumerate %s: %v", toolset, err)
	}
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.Name
	}
	return names
}

func TestMirroredSurfaceCounts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		toolset string
		want    int
		must    []string
	}{
		{"weather", 17, []string{"get_historical_weather", "search_location", "get_forecast"}},
		{"duckduckgo", 2, []string{"search", "fetch_content"}},
		{"wikipedia", 11, []string{"search_wikipedia", "get_article", "get_summary"}},
		{"pdf-toolkit", 22, []string{"pdf_create", "pdf_create_from_markdown", "pdf_extract_text", "pdf_get_metadata", "pdf_search", "pdf_encrypt"}},
	}
	for _, c := range cases {
		names := toolNames(t, c.toolset, t.TempDir())
		if len(names) != c.want {
			t.Errorf("%s advertises %d tools, want %d: %v", c.toolset, len(names), c.want, names)
		}
		set := map[string]bool{}
		for _, n := range names {
			set[n] = true
		}
		for _, m := range c.must {
			if !set[m] {
				t.Errorf("%s missing expected tool %q", c.toolset, m)
			}
		}
	}
}

func TestWeatherFunctionalDeterministic(t *testing.T) {
	t.Parallel()

	args := map[string]any{"latitude": 54.6872, "longitude": 25.2797, "start_date": "2024-07-15", "end_date": "2024-07-15"}
	r1 := callFixtureTool(t, "weather", weatherFixtureDir, "get_historical_weather", args)
	r2 := callFixtureTool(t, "weather", weatherFixtureDir, "get_historical_weather", args)

	b1, _ := json.Marshal(r1)
	b2, _ := json.Marshal(r2)
	if string(b1) != string(b2) {
		t.Error("get_historical_weather is not deterministic")
	}
	if !strings.Contains(string(b1), "25.3") {
		t.Errorf("baked max temp 25.3 not returned: %s", b1)
	}
}

func TestSearchPhrasingRobust(t *testing.T) {
	t.Parallel()

	for _, q := range []string{"Vilnius capital Lithuania", "what country is Vilnius the capital of"} {
		res := callFixtureTool(t, "duckduckgo", weatherFixtureDir, "search", map[string]any{"query": q})
		results, ok := res["results"].([]any)
		if !ok || len(results) == 0 {
			t.Fatalf("search %q returned no results: %v", q, res)
		}
		top, _ := json.Marshal(results[0])
		if !strings.Contains(strings.ToLower(string(top)), "vilnius") {
			t.Errorf("search %q top result not about Vilnius: %s", q, top)
		}
	}
}

func TestWikipediaFunctional(t *testing.T) {
	t.Parallel()

	res := callFixtureTool(t, "wikipedia", weatherFixtureDir, "get_summary", map[string]any{"title": "Vilnius"})
	if s, _ := res["summary"].(string); !strings.Contains(s, "capital of Lithuania") {
		t.Errorf("Vilnius summary missing the ground-truth fact: %v", res)
	}
	art := callFixtureTool(t, "wikipedia", weatherFixtureDir, "get_article", map[string]any{"title": "Vilnius"})
	if c, _ := art["content"].(string); !strings.Contains(c, "Neris") {
		t.Errorf("Vilnius article missing Neris: %v", art)
	}
}

func TestPDFToolkitRoundTrip(t *testing.T) {
	out := t.TempDir()
	t.Setenv(outputDirEnv, out)

	create := callFixtureTool(t, "pdf-toolkit", "", "pdf_create_from_markdown", map[string]any{
		"markdown":   "# Report\n\nMax temperature was 24.9 C in Vilnius on the Neris river.",
		"outputPath": "output/report.pdf",
		"title":      "Weather Report",
	})
	if ok, _ := create["ok"].(bool); !ok {
		t.Fatalf("pdf_create_from_markdown failed: %v", create)
	}
	if _, err := os.Stat(filepath.Join(out, "output", "report.pdf")); err != nil {
		t.Fatalf("PDF not written: %v", err)
	}

	extract := callFixtureTool(t, "pdf-toolkit", "", "pdf_extract_text", map[string]any{"filePath": "output/report.pdf"})
	text, _ := extract["text"].(string)
	for _, want := range []string{"24.9", "Vilnius", "Neris"} {
		if !strings.Contains(text, want) {
			t.Errorf("extracted text missing %q: %s", want, text)
		}
	}
}

func TestOutputDirResolution(t *testing.T) {
	t.Setenv(outputDirEnv, "/tmp/run-x/workspace")
	got, err := resolveOutputPath("output/report.pdf")
	if err != nil || got != "/tmp/run-x/workspace/output/report.pdf" {
		t.Errorf("resolveOutputPath = %q, %v", got, err)
	}
	if _, err := resolveOutputPath("../escape.pdf"); err == nil {
		t.Error("path traversal not rejected")
	}
}

func TestWeatherScenarioEstateSurface(t *testing.T) {
	t.Parallel()

	// The weather scenario's four functional toolsets plus the full corpus form
	// the estate both modes face. Direct advertises them all; ozy advertises 3.
	servers, err := scenarioServers(
		[]string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
		true, weatherFixtureDir, benchCorpusDir)
	if err != nil {
		t.Fatalf("scenarioServers: %v", err)
	}
	canonical := []RequiredTool{
		{Server: "weather", Tool: "get_historical_weather"},
		{Server: "duckduckgo", Tool: "search"},
		{Server: "wikipedia", Tool: ""},
		{Server: "pdf-toolkit", Tool: ""},
	}
	c, err := ComputeSurfaceComparison(servers, weatherFixtureDir, benchCorpusDir, canonical, eval.DefaultEstimator)
	if err != nil {
		t.Fatalf("ComputeSurfaceComparison: %v", err)
	}
	// 4 functional toolsets (17+2+11+22=52) + >=500 corpus tools.
	if c.Direct.ToolsVisible < 550 {
		t.Errorf("direct surface = %d tools, want >= 550 (estate not attached)", c.Direct.ToolsVisible)
	}
	if c.Ozy.ToolsVisible != 3 {
		t.Errorf("ozy surface = %d tools, want 3", c.Ozy.ToolsVisible)
	}
	// The vast majority of direct's schema tokens are irrelevant (non-canonical).
	if c.Direct.IrrelevantSchemaTokens <= c.Direct.SchemaTokens/2 {
		t.Errorf("irrelevant tokens = %d of %d, expected most of the estate", c.Direct.IrrelevantSchemaTokens, c.Direct.SchemaTokens)
	}
}
