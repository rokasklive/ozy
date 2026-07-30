package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/eval"
)

func TestComputeAggregate(t *testing.T) {
	t.Parallel()

	runs := []*RunMetrics{
		{Success: true, DurationSec: 10, ToolCallCount: 4, InputTokens: 100, OutputTokens: 20, TotalTokens: 120, TokenSource: TokenSourceMeasured},
		{Success: true, DurationSec: 20, ToolCallCount: 6, InputTokens: 200, OutputTokens: 40, TotalTokens: 240, TokenSource: TokenSourceMeasured},
		{Success: false, TimedOut: true, DurationSec: 30, ToolCallCount: 2, InputTokens: 300, OutputTokens: 0, TotalTokens: 300, TokenSource: TokenSourceMeasured},
	}
	a := ComputeAggregate("ozy", runs)

	if a.N != 3 || a.SuccessK != 2 {
		t.Errorf("success = %d/%d, want 2/3", a.SuccessK, a.N)
	}
	if a.TimedOut != 1 {
		t.Errorf("timedOut = %d, want 1", a.TimedOut)
	}
	if a.Duration.Mean != 20 || a.Duration.Min != 10 || a.Duration.Max != 30 {
		t.Errorf("duration stats = %+v, want mean 20 min 10 max 30", a.Duration)
	}
	// population stdev of {10,20,30} = sqrt(200/3) ≈ 8.16
	if a.Duration.Stdev < 8.1 || a.Duration.Stdev > 8.2 {
		t.Errorf("duration stdev = %.2f, want ≈8.16", a.Duration.Stdev)
	}
	if a.TokenSource != TokenSourceMeasured {
		t.Errorf("tokenSource = %q, want measured", a.TokenSource)
	}
	if a.SuccessRate() < 0.66 || a.SuccessRate() > 0.67 {
		t.Errorf("success rate = %.3f, want ≈0.667", a.SuccessRate())
	}
}

func TestComputeAggregateMixedSource(t *testing.T) {
	t.Parallel()

	a := ComputeAggregate("direct", []*RunMetrics{
		{Success: true, TokenSource: TokenSourceMeasured},
		{Success: true, TokenSource: TokenSourceEstimated},
	})
	if a.TokenSource != TokenSourceMixed {
		t.Errorf("tokenSource = %q, want mixed", a.TokenSource)
	}
}

func TestComputeAggregateEmpty(t *testing.T) {
	t.Parallel()

	a := ComputeAggregate("ozy", nil)
	if a.N != 0 || a.TokenSource != TokenSourceNone {
		t.Errorf("empty aggregate = %+v, want N=0 source=none", a)
	}
}

func TestComputeMetricsMeasured(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	// A usage event under the top-level "tokens" key plus one under "part".
	lines := `{"type":"text","part":{"text":"hi"}}
{"type":"step-finish","tokens":{"input":100,"output":40,"reasoning":5,"cache":{"read":10,"write":2}}}
{"type":"step-finish","part":{"tokens":{"input":50,"output":10}}}
`
	if err := os.WriteFile(transcript, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &RunResult{RunID: "run-1", Mode: "ozy", Success: true, Transcript: transcript, FinalAnswer: "answer"}
	m := ComputeMetrics(result, 999, eval.DefaultEstimator, RetrievalInput{})

	if m.TokenSource != TokenSourceMeasured {
		t.Fatalf("tokenSource = %q, want measured", m.TokenSource)
	}
	if m.InputTokens != 150 || m.OutputTokens != 50 {
		t.Errorf("tokens = in %d out %d, want in 150 out 50", m.InputTokens, m.OutputTokens)
	}
	if m.ReasoningTokens != 5 || m.CacheReadTokens != 10 || m.CacheWriteTokens != 2 {
		t.Errorf("fine buckets = reasoning %d cacheR %d cacheW %d", m.ReasoningTokens, m.CacheReadTokens, m.CacheWriteTokens)
	}
	if m.TotalTokens != 200 {
		t.Errorf("total = %d, want 200", m.TotalTokens)
	}
}

func TestComputeMetricsEstimatedFallback(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "transcript.jsonl")
	// No usage events — only text/tool_use.
	body := `{"type":"text","part":{"text":"some agent output here"}}` + "\n"
	if err := os.WriteFile(transcript, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	result := &RunResult{RunID: "run-1", Mode: "direct", Success: false, Transcript: transcript, FinalAnswer: "final"}
	surfaceTokens := 500
	m := ComputeMetrics(result, surfaceTokens, eval.DefaultEstimator, RetrievalInput{})

	if m.TokenSource != TokenSourceEstimated {
		t.Fatalf("tokenSource = %q, want estimated", m.TokenSource)
	}
	if m.InputTokens <= surfaceTokens {
		t.Errorf("input tokens = %d, want > surface tokens (%d)", m.InputTokens, surfaceTokens)
	}
	if m.OutputTokens == 0 {
		t.Error("estimated output tokens should be > 0 for a non-empty final answer")
	}
}

func TestParseTranscriptSelfCheck(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Non-empty transcript whose lines never parse into recognized events.
	garbage := filepath.Join(dir, "garbage.jsonl")
	if err := os.WriteFile(garbage, []byte("not json\n{\"type\":\"unknown\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, events := parseTranscript(garbage)
	if events != 0 {
		t.Errorf("garbage transcript parsed %d events, want 0 (parse-failure signal)", events)
	}

	// A valid text event parses.
	good := filepath.Join(dir, "good.jsonl")
	if err := os.WriteFile(good, []byte(`{"type":"text","part":{"text":"ok"}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	answer, _, events := parseTranscript(good)
	if events == 0 || answer != "ok" {
		t.Errorf("good transcript: events=%d answer=%q, want events>0 answer=ok", events, answer)
	}
}

func TestWriteComparisonBoth(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	aggregates := map[string]*Aggregate{
		"direct": ComputeAggregate("direct", []*RunMetrics{{Success: true, DurationSec: 12, TotalTokens: 5000, ToolCallCount: 8, TokenSource: TokenSourceMeasured}}),
		"ozy":    ComputeAggregate("ozy", []*RunMetrics{{Success: true, DurationSec: 14, TotalTokens: 2000, ToolCallCount: 10, TokenSource: TokenSourceMeasured}}),
	}
	prov := &EnvironmentRecord{ModelID: "opencode/x", TokenEstimator: surface.Estimator, ScenarioHash: "abc"}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatalf("WriteComparison: %v", err)
	}
	md, err := os.ReadFile(filepath.Join(dir, "comparison.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(md)
	for _, want := range []string{"## Verdict", "Startup surface", "Live tier", "Success k/N", "opencode/x"} {
		if !strings.Contains(text, want) {
			t.Errorf("comparison.md missing %q", want)
		}
	}
	// The JSON must decode and carry both modes.
	var c Comparison
	jb, _ := os.ReadFile(filepath.Join(dir, "comparison.json"))
	if err := json.Unmarshal(jb, &c); err != nil {
		t.Fatalf("comparison.json invalid: %v", err)
	}
	if c.LiveSkipped || len(c.Modes) != 2 {
		t.Errorf("comparison.json: liveSkipped=%v modes=%d, want false/2", c.LiveSkipped, len(c.Modes))
	}
}

func TestWriteComparisonSurfaceOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	prov := &EnvironmentRecord{ModelID: benchModel(), TokenEstimator: surface.Estimator, UsageSource: TokenSourceSkipped}

	if err := WriteComparison(dir, surface, nil, prov); err != nil {
		t.Fatalf("WriteComparison surface-only: %v", err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "comparison.md"))
	if !strings.Contains(string(md), "Skipped") {
		t.Errorf("surface-only comparison.md should mark live tier skipped:\n%s", md)
	}
	var c Comparison
	jb, _ := os.ReadFile(filepath.Join(dir, "comparison.json"))
	if err := json.Unmarshal(jb, &c); err != nil {
		t.Fatal(err)
	}
	if !c.LiveSkipped {
		t.Error("surface-only comparison.json should set liveSkipped=true")
	}
}

// TestProvenanceCredentialFree asserts environment.json never carries a
// credential — no key or value that looks like a secret (scenario-bench:
// "Provenance is recorded and credential-free").
func TestProvenanceCredentialFree(t *testing.T) {
	// Not parallel — uses t.Setenv.
	t.Setenv("BENCH_MODEL", "opencode/free-model")
	// Even if a credential-shaped env is set, it must not leak into provenance.
	t.Setenv("MODEL_API_KEY", "sk-should-not-appear")
	t.Setenv("OPENCODE_VERSION", "1.17.7")

	cfg := &ScenarioConfig{Name: "s", TaskFile: "task.md", BaseDir: t.TempDir()}
	if err := os.WriteFile(filepath.Join(cfg.BaseDir, "task.md"), []byte("do the thing"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec, err := BuildProvenance(cfg, []string{"direct", "ozy"}, 5, "chars/4 heuristic", TokenSourceMeasured)
	if err != nil {
		t.Fatal(err)
	}
	blob, _ := json.Marshal(rec)
	lower := strings.ToLower(string(blob))
	for _, banned := range []string{"apikey", "api_key", "secret", "token\":", "password", "sk-should-not-appear", "authorization", "credential"} {
		if strings.Contains(lower, banned) {
			t.Errorf("provenance leaked credential-shaped content %q:\n%s", banned, blob)
		}
	}
	if rec.ModelID != "opencode/free-model" || rec.OpenCodeVersion != "1.17.7" {
		t.Errorf("provenance = %+v, want model/version populated", rec)
	}
}

func TestComparisonLiveDeltasBothModes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	direct := ComputeAggregate("direct", []*RunMetrics{
		{Success: false, DurationSec: 30, TotalTokens: 40000, CanonicalToolsHit: 1, CanonicalToolsTotal: 4, DistractorCallCount: 6, DownstreamCallCount: 8, TokenSource: TokenSourceMeasured},
	})
	ozy := ComputeAggregate("ozy", []*RunMetrics{
		{Success: true, DurationSec: 20, TotalTokens: 6000, CanonicalToolsHit: 4, CanonicalToolsTotal: 4, DistractorCallCount: 0, DownstreamCallCount: 5, TokenSource: TokenSourceMeasured},
	})
	aggregates := map[string]*Aggregate{"direct": direct, "ozy": ozy}
	prov := &EnvironmentRecord{ModelID: "opencode/x", Retrieval: "lexical", Modes: []string{"direct", "ozy"}}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatal(err)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "comparison.md"))
	text := string(md)
	for _, want := range []string{
		"Delta (ozy − direct)",               // delta column header
		"Total tokens mean±stdev",            // efficiency rows lead
		"Tool calls/run mean±stdev",          // new tool-call row
		"_Informational (tool attribution)_", // demoted section header
		"Canonical tools hit",                // retrieval rows (now informational)
		"Distractor calls/run",
		"Downstream calls/run",
		"**Retrieval:** lexical", // provenance display
		"pp",                     // success delta in percentage points
	} {
		if !strings.Contains(text, want) {
			t.Errorf("comparison.md missing %q\n%s", want, text)
		}
	}
	// Efficiency leads: the total-tokens row and the informational section header
	// both render, with efficiency above attribution (D2).
	if strings.Index(text, "Total tokens mean±stdev") > strings.Index(text, "_Informational (tool attribution)_") {
		t.Errorf("efficiency rows must lead the informational section:\n%s", text)
	}
	// The verdict delta line leads with total tokens, not attribution.
	if di := strings.Index(text, "Delta (ozy − direct):"); di >= 0 {
		if !strings.Contains(text[di:di+40], "total tokens") {
			t.Errorf("verdict delta should lead with total tokens:\n%s", text[di:di+80])
		}
	}
}

func TestComparisonThreeModes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	direct := ComputeAggregate("direct", []*RunMetrics{
		{Success: false, DurationSec: 30, TotalTokens: 40000, CanonicalToolsHit: 1, CanonicalToolsTotal: 4, DistractorCallCount: 6, DownstreamCallCount: 8, TokenSource: TokenSourceMeasured},
	})
	lean := ComputeAggregate("direct-lean", []*RunMetrics{
		{Success: true, DurationSec: 22, TotalTokens: 12000, CanonicalToolsHit: 4, CanonicalToolsTotal: 4, DistractorCallCount: 0, DownstreamCallCount: 6, TokenSource: TokenSourceMeasured},
	})
	ozy := ComputeAggregate("ozy", []*RunMetrics{
		{Success: true, DurationSec: 20, TotalTokens: 6000, CanonicalToolsHit: 4, CanonicalToolsTotal: 4, DistractorCallCount: 0, DownstreamCallCount: 5, TokenSource: TokenSourceMeasured},
	})
	aggregates := map[string]*Aggregate{"direct": direct, "direct-lean": lean, "ozy": ozy}
	prov := &EnvironmentRecord{ModelID: "opencode/x", Retrieval: "lexical", Modes: []string{"direct", "direct-lean", "ozy"}}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, filepath.Join(dir, "comparison.md")))
	for _, want := range []string{
		"Direct-lean",                             // mode column header
		"Δ (ozy − direct)",                        // secondary delta column
		"Δ (ozy − direct-lean)",                   // primary delta column
		"Delta (ozy − direct-lean)",               // verdict primary delta line
		"Delta (ozy − direct)",                    // verdict secondary delta line
		"distractor calls are ~0 by construction", // lean structural note
	} {
		if !strings.Contains(text, want) {
			t.Errorf("three-mode comparison.md missing %q\n%s", want, text)
		}
	}

	var c Comparison
	jb, _ := os.ReadFile(filepath.Join(dir, "comparison.json"))
	if err := json.Unmarshal(jb, &c); err != nil {
		t.Fatalf("comparison.json invalid: %v", err)
	}
	if len(c.Modes) != 3 {
		t.Errorf("comparison.json modes=%d, want 3", len(c.Modes))
	}
	if c.Surface == nil || c.Surface.DirectLean == nil {
		t.Errorf("comparison.json surface missing direct-lean")
	}
}

// TestComparisonLeanNotRunLabeled asserts direct-lean is labeled "not run" when
// the invocation did not include it (a legacy `both` run of direct + ozy).
func TestComparisonLeanNotRunLabeled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	direct := ComputeAggregate("direct", []*RunMetrics{{Success: true, DurationSec: 10, TotalTokens: 30000, TokenSource: TokenSourceMeasured}})
	ozy := ComputeAggregate("ozy", []*RunMetrics{{Success: true, DurationSec: 12, TotalTokens: 5000, TokenSource: TokenSourceMeasured}})
	aggregates := map[string]*Aggregate{"direct": direct, "ozy": ozy}
	prov := &EnvironmentRecord{ModelID: "opencode/x", Modes: []string{"direct", "ozy"}}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, filepath.Join(dir, "comparison.md")))
	if !strings.Contains(text, "Direct-lean (not run, MODE=both)") {
		t.Errorf("missing lean not-run label for a both-mode run:\n%s", text)
	}
}

func TestComparisonNotRunLabeled(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	// Only ozy ran.
	ozy := ComputeAggregate("ozy", []*RunMetrics{
		{Success: true, DurationSec: 20, TotalTokens: 6000, CanonicalToolsHit: 4, CanonicalToolsTotal: 4, TokenSource: TokenSourceMeasured},
	})
	aggregates := map[string]*Aggregate{"ozy": ozy}
	prov := &EnvironmentRecord{ModelID: "opencode/x", Modes: []string{"ozy"}}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, filepath.Join(dir, "comparison.md")))
	if !strings.Contains(text, "not run, MODE=ozy") {
		t.Errorf("missing not-run label:\n%s", text)
	}
	if !strings.Contains(text, "no baseline mode (direct or direct-lean) was run") {
		t.Errorf("verdict missing not-run reason:\n%s", text)
	}
	// Never a bare em-dash for the direct column header.
	if strings.Contains(text, "| — |") && !strings.Contains(text, "not run") {
		t.Errorf("bare em-dash header without explanation:\n%s", text)
	}
}

func TestComparisonFailureReasonsSurfaced(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	surface, err := computeTestSurface(t)
	if err != nil {
		t.Fatal(err)
	}
	// Direct rejected at estate scale on every run.
	direct := ComputeAggregate("direct", []*RunMetrics{
		{Success: false, FailureReason: "toolset_rejected", DurationSec: 5, TokenSource: TokenSourceEstimated},
		{Success: false, FailureReason: "toolset_rejected", DurationSec: 5, TokenSource: TokenSourceEstimated},
	})
	ozy := ComputeAggregate("ozy", []*RunMetrics{
		{Success: true, DurationSec: 20, CanonicalToolsHit: 4, CanonicalToolsTotal: 4, TokenSource: TokenSourceEstimated},
	})
	aggregates := map[string]*Aggregate{"direct": direct, "ozy": ozy}
	prov := &EnvironmentRecord{ModelID: "opencode/x", Modes: []string{"direct", "ozy"}}

	if err := WriteComparison(dir, surface, aggregates, prov); err != nil {
		t.Fatal(err)
	}
	text := string(mustRead(t, filepath.Join(dir, "comparison.md")))
	if !strings.Contains(text, "toolset_rejected") {
		t.Errorf("failure reason not surfaced:\n%s", text)
	}
	if direct.FailureReasons["toolset_rejected"] != 2 {
		t.Errorf("aggregate failure count = %d, want 2", direct.FailureReasons["toolset_rejected"])
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestComputeRetrievalServerLevelRequired(t *testing.T) {
	t.Parallel()

	// Server-level required tools ({server, tool:""}) must count as canonical
	// hits when any tool on that server was called — matching the grader.
	in := RetrievalInput{
		Calls: []ToolCallLog{
			{Server: "weather", Tool: "get_historical_weather"},
			{Server: "duckduckgo", Tool: "search"},
			{Server: "wikipedia", Tool: "get_article"},             // satisfies {wikipedia, ""}
			{Server: "pdf-toolkit", Tool: "pdf_create"},            // satisfies {pdf-toolkit, ""}
			{Server: "doc-converter", Tool: "convert_html_to_pdf"}, // distractor
		},
		RequiredTools: []RequiredTool{
			{Server: "weather", Tool: "get_historical_weather"},
			{Server: "duckduckgo", Tool: "search"},
			{Server: "wikipedia", Tool: ""},
			{Server: "pdf-toolkit", Tool: ""},
		},
		FunctionalToolsets: []string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
	}
	hit, total, distractor, downstream := computeRetrieval(in)
	if hit != 4 || total != 4 {
		t.Errorf("canonical hit = %d/%d, want 4/4 (server-level requireds must count)", hit, total)
	}
	if distractor != 1 {
		t.Errorf("distractor = %d, want 1 (only doc-converter)", distractor)
	}
	if downstream != 5 {
		t.Errorf("downstream = %d, want 5", downstream)
	}
}

func TestWastageMetrics(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	transcript := filepath.Join(dir, "t.jsonl")
	// 800 measured tokens total (600 in + 200 out).
	os.WriteFile(transcript, []byte(`{"type":"step-finish","tokens":{"input":600,"output":200}}`+"\n"), 0o644)

	// 8 downstream calls, 2 of them distractors → wasted ≈ (800/8)*2 = 200.
	calls := make([]ToolCallLog, 0, 8)
	for i := 0; i < 6; i++ {
		calls = append(calls, ToolCallLog{Server: "weather", Tool: "get_historical_weather"})
	}
	calls = append(calls, ToolCallLog{Server: "brave-search", Tool: "web_search"})
	calls = append(calls, ToolCallLog{Server: "doc-converter", Tool: "convert_html_to_pdf"})

	result := &RunResult{RunID: "run-1", Mode: "direct", Transcript: transcript}
	m := ComputeMetrics(result, 0, eval.DefaultEstimator, RetrievalInput{
		Calls:              calls,
		FunctionalToolsets: []string{"weather", "duckduckgo", "wikipedia", "pdf-toolkit"},
	})
	if m.DistractorCallCount != 2 || m.DownstreamCallCount != 8 {
		t.Fatalf("distractor/downstream = %d/%d, want 2/8", m.DistractorCallCount, m.DownstreamCallCount)
	}
	if m.WastedCallRatio != 0.25 {
		t.Errorf("wasted call ratio = %v, want 0.25", m.WastedCallRatio)
	}
	if m.WastedTokensEst != 200 {
		t.Errorf("wasted tokens est = %d, want 200", m.WastedTokensEst)
	}
}
