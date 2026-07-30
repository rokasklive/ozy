package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// benchCorpusDir is the checked-in corpus, relative to this package directory.
const benchCorpusDir = "../../bench/corpus"

func TestCorpusFloor(t *testing.T) {
	t.Parallel()

	corpus, err := LoadCorpus(benchCorpusDir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}

	// Server and tool floors.
	if len(corpus) < 25 {
		t.Errorf("corpus has %d servers, want >= 25", len(corpus))
	}
	total := 0
	for _, s := range corpus {
		total += len(s.Tools)
	}
	if total < 500 {
		t.Errorf("corpus has %d tools, want >= 500", total)
	}

	for _, s := range corpus {
		// Mirror provenance recorded (loader also enforces).
		if strings.TrimSpace(s.MirrorSource) == "" {
			t.Errorf("server %q missing mirrorSource", s.Server)
		}
		names := map[string]bool{}
		for _, tl := range s.Tools {
			if names[tl.Name] {
				t.Errorf("server %q: duplicate tool %q", s.Server, tl.Name)
			}
			names[tl.Name] = true
			// Description length bounds (production-tone, one or two sentences).
			if n := len(tl.Description); n < 12 || n > 400 {
				t.Errorf("server %q tool %q: description length %d out of bounds [12,400]", s.Server, tl.Name, n)
			}
		}
	}
}

func TestCorpusDistractorFamilies(t *testing.T) {
	t.Parallel()

	corpus, err := LoadCorpus(benchCorpusDir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}

	families := map[string]int{"duckduckgo": 0, "weather": 0, "pdf-toolkit": 0}
	for _, s := range corpus {
		for canonical := range families {
			if strings.Contains(s.MirrorSource, "near-miss for the canonical "+canonical) {
				families[canonical]++
			}
		}
	}
	for canonical, n := range families {
		if n < 3 {
			t.Errorf("only %d near-miss servers for canonical %q, want >= 3", n, canonical)
		}
	}
}

func TestCorpusHasNoScenarioFacts(t *testing.T) {
	t.Parallel()

	corpus, err := LoadCorpus(benchCorpusDir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	// A wrong-tool path must not be able to satisfy grading: the ground-truth
	// facts of the weather scenario must never appear in corpus tool text.
	forbidden := []string{"25.3", "14.9", "Neris", "capital of Lithuania", "Vilnius"}
	for _, s := range corpus {
		for _, tl := range s.Tools {
			hay := tl.Name + " " + tl.Description + " " + string(tl.InputSchema)
			for _, f := range forbidden {
				if strings.Contains(hay, f) {
					t.Errorf("server %q tool %q leaks scenario fact %q", s.Server, tl.Name, f)
				}
			}
		}
	}
}

func TestCorpusServesDeterministically(t *testing.T) {
	t.Parallel()

	// A corpus stub returns byte-identical output for the same tool, regardless
	// of arguments, and never the scenario facts.
	srv, err := newCorpusServer("github", benchCorpusDir, "")
	if err != nil {
		t.Fatalf("newCorpusServer: %v", err)
	}
	_ = srv // constructed successfully; serving determinism is covered by corpusResponse.

	tool := CorpusTool{Name: "create_issue", Description: "x"}
	r1 := corpusResponse(tool)
	r2 := corpusResponse(tool)
	if r1 != r2 {
		t.Errorf("corpus response not deterministic: %q vs %q", r1, r2)
	}
	if strings.Contains(r1, "Vilnius") || strings.Contains(r1, "25.3") {
		t.Errorf("corpus response leaks scenario fact: %q", r1)
	}
}

// callArgs builds a corpus tool-call request with the given JSON arguments.
func callArgs(t *testing.T, args map[string]any) *mcpsdk.CallToolRequest {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	return &mcpsdk.CallToolRequest{Params: &mcpsdk.CallToolParamsRaw{Arguments: raw}}
}

func toolText(t *testing.T, res *mcpsdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("tool result has no content")
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("tool result content is not text: %T", res.Content[0])
	}
	return tc.Text
}

// TestCorpusBehaviorAssigned asserts the checked-in corpus carries the D3
// behavior tiers: auth-gated search/climate rivals error, and the no-auth
// document rivals expose a functional:pdf tool.
func TestCorpusBehaviorAssigned(t *testing.T) {
	t.Parallel()

	corpus, err := LoadCorpus(benchCorpusDir)
	if err != nil {
		t.Fatalf("LoadCorpus: %v", err)
	}
	byName := map[string]CorpusServer{}
	for _, s := range corpus {
		byName[s.Server] = s
	}
	for _, srv := range []string{"brave-search", "bing-search", "kagi-search", "climate-analytics"} {
		s, ok := byName[srv]
		if !ok {
			t.Fatalf("corpus missing auth-gated rival %q", srv)
		}
		for _, tl := range s.Tools {
			if tl.Behavior != "auth_error" {
				t.Errorf("%s/%s behavior = %q, want auth_error", srv, tl.Name, tl.Behavior)
			}
		}
	}
	hasFunctional := func(srv, tool string) bool {
		for _, tl := range byName[srv].Tools {
			if tl.Name == tool {
				return tl.Behavior == "functional:pdf"
			}
		}
		return false
	}
	if !hasFunctional("doc-converter", "convert_html_to_pdf") {
		t.Error("doc-converter/convert_html_to_pdf should be functional:pdf")
	}
	if !hasFunctional("office-export", "export_to_pdf") {
		t.Error("office-export/export_to_pdf should be functional:pdf")
	}
}

// TestCorpusAuthErrorHandler asserts an auth_error tool returns a legible,
// byte-identical authentication error and never a fake success.
func TestCorpusAuthErrorHandler(t *testing.T) {
	t.Parallel()

	h := corpusHandler("brave-search", CorpusTool{Name: "web_search", Behavior: "auth_error"}, "")
	r1, err := h(context.Background(), callArgs(t, map[string]any{"query": "vilnius"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !r1.IsError {
		t.Error("auth_error result should set IsError")
	}
	body := toolText(t, r1)
	if !strings.Contains(body, "authentication required") {
		t.Errorf("auth error body = %q, want authentication-required message", body)
	}
	// Deterministic regardless of arguments.
	r2, _ := h(context.Background(), callArgs(t, map[string]any{"query": "something else"}))
	if toolText(t, r2) != body {
		t.Errorf("auth error not byte-identical: %q vs %q", toolText(t, r2), body)
	}
}

// TestCorpusFunctionalSearchHandler asserts a functional:search rival returns
// real ranked results from the baked fixture corpus.
func TestCorpusFunctionalSearchHandler(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	corpus := []map[string]any{
		{"title": "Vilnius facts", "url": "https://x/vilnius", "snippet": "Vilnius sits on the Neris river in Lithuania."},
		{"title": "Unrelated", "url": "https://x/other", "snippet": "nothing to see"},
	}
	data, _ := json.Marshal(corpus)
	if err := os.WriteFile(filepath.Join(dir, "search.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	h := corpusHandler("altweb", CorpusTool{Name: "search", Behavior: "functional:search"}, dir)
	res, err := h(context.Background(), callArgs(t, map[string]any{"query": "Neris Vilnius"}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	body := toolText(t, res)
	if res.IsError || !strings.Contains(body, "Neris") {
		t.Errorf("functional search should return real results with facts, got %q", body)
	}
}

// TestCorpusFunctionalPDFHandler asserts a functional:pdf rival actually writes
// the report file to the agent workspace, so the outcome grader finds it.
func TestCorpusFunctionalPDFHandler(t *testing.T) {
	// Not parallel — sets OZY_BENCH_OUTPUT_DIR.
	dir := t.TempDir()
	t.Setenv(outputDirEnv, dir)

	h := corpusHandler("doc-converter", CorpusTool{Name: "convert_html_to_pdf", Behavior: "functional:pdf"}, "")
	res, err := h(context.Background(), callArgs(t, map[string]any{
		"html":        "Max 25.3 C, min 14.9 C in Vilnius on the Neris, Lithuania.",
		"output_path": "output/report.pdf",
	}))
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if res.IsError {
		t.Fatalf("functional pdf returned error: %s", toolText(t, res))
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "output", "report.pdf"))
	if err != nil {
		t.Fatalf("report not written: %v", err)
	}
	if !IsPDF(pdf) {
		t.Error("written file is not a valid PDF")
	}
	if txt := ExtractPDFText(pdf); !strings.Contains(txt, "25.3") {
		t.Errorf("PDF missing the supplied fact, got %q", txt)
	}
}

// TestCorpusInvalidBehaviorRejected asserts the loader rejects an unknown
// behavior value.
func TestCorpusInvalidBehaviorRejected(t *testing.T) {
	t.Parallel()

	s := &CorpusServer{
		Server:       "x",
		MirrorSource: "test",
		Tools: []CorpusTool{{
			Name:        "t",
			Description: "a tool",
			Behavior:    "functional:bogus",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
		}},
	}
	if err := s.Validate(); err == nil {
		t.Error("Validate should reject an unknown functional backend")
	}
}
