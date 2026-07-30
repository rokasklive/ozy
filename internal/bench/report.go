package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rokasklive/ozy/internal/eval"
)

// TokenSource values label how a run's token counts were obtained.
const (
	TokenSourceMeasured  = "measured"  // from OpenCode's agent-reported usage events
	TokenSourceEstimated = "estimated" // from transcript bytes + enumerated startup schemas
	TokenSourceMixed     = "mixed"     // an aggregate pooling both sources
	TokenSourceSkipped   = "skipped"   // surface-only invocation, no live runs
	TokenSourceNone      = "none"      // a mode with no completed runs
)

// RunMetrics is the per-run metrics artifact (metrics.json).
type RunMetrics struct {
	RunID            string  `json:"runId"`
	Mode             string  `json:"mode"`
	Success          bool    `json:"success"`
	TimedOut         bool    `json:"timedOut"`
	ParseFailed      bool    `json:"parseFailed"`
	FailureReason    string  `json:"failureReason,omitempty"` // toolset_rejected | context_overflow | timed_out
	DurationSec      float64 `json:"durationSec"`
	ToolCallCount    int     `json:"toolCallCount"`
	InputTokens      int     `json:"inputTokens"`
	OutputTokens     int     `json:"outputTokens"`
	ReasoningTokens  int     `json:"reasoningTokens,omitempty"`
	CacheReadTokens  int     `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int     `json:"cacheWriteTokens,omitempty"`
	TotalTokens      int     `json:"totalTokens"`
	TokenSource      string  `json:"tokenSource"`

	// Retrieval quality, computed from the server-side invocation log.
	CanonicalToolsHit   int `json:"canonicalToolsHit"`   // distinct required tools called
	CanonicalToolsTotal int `json:"canonicalToolsTotal"` // N required tools
	DistractorCallCount int `json:"distractorCallCount"` // calls to corpus/distractor servers
	DownstreamCallCount int `json:"downstreamCallCount"` // total server-side calls

	// Wastage: the runtime cost of distractor calls. WastedCallRatio is exact
	// (distractor/downstream); WastedTokensEst is an estimate — average tokens per
	// downstream call times the distractor count, since exact per-call token
	// attribution would need fragile transcript parsing the harness avoids (D4).
	WastedCallRatio float64 `json:"wastedCallRatio"`
	WastedTokensEst int     `json:"wastedTokensEst"`
}

// RetrievalInput carries the data needed to score a run's retrieval quality from
// the server-side invocation log.
type RetrievalInput struct {
	Calls              []ToolCallLog  // server-side invocation log records
	RequiredTools      []RequiredTool // the scenario's canonical set
	FunctionalToolsets []string       // the scenario's legit fixture servers (non-corpus)
}

// computeRetrieval scores canonical-tool hits and distractor calls from the log.
// A distractor is a call to a server that is not one of the scenario's
// functional toolsets — i.e., a corpus/distractor server.
func computeRetrieval(in RetrievalInput) (hit, total, distractor, downstream int) {
	total = len(in.RequiredTools)
	downstream = len(in.Calls)

	functional := map[string]bool{}
	for _, ts := range in.FunctionalToolsets {
		functional[ts] = true
	}
	for _, c := range in.Calls {
		if c.Server != "" && !functional[c.Server] {
			distractor++
		}
	}
	for _, rt := range in.RequiredTools {
		for _, c := range in.Calls {
			if requiredToolMatches(rt, c) {
				hit++
				break
			}
		}
	}
	return hit, total, distractor, downstream
}

// failureSignatures classify infra failures from transcript error text. The exact
// gateway wording is confirmed by the estate-scale probe (task 1.1); this list is
// the tolerant default and is trivial to extend.
//
// ponytail: heuristic substring match on the transcript — tighten the strings
// once 1.1 pins the real gateway rejection wording.
var failureSignatures = []struct {
	reason     string
	substrings []string
}{
	{"context_overflow", []string{"context length", "maximum context", "context_length_exceeded", "exceeds the maximum", "too many tokens", "reduce the length"}},
	{"toolset_rejected", []string{"too many tools", "tool definitions", "number of tools", "maximum number of tools", "tools array is too", "invalid tools"}},
}

// classifyFailure names why a run failed at the infrastructure level, distinct
// from a plain task failure. Timeouts win; otherwise the transcript is probed
// for gateway rejection or context-overflow signatures. Returns "" for a run
// that ran to completion (its success is graded separately).
//
//nolint:gosec // G304: transcript is a controlled artifact path.
func classifyFailure(result *RunResult) string {
	if result.TimedOut {
		return "timed_out"
	}
	body, err := os.ReadFile(result.Transcript)
	if err != nil {
		return ""
	}
	lower := strings.ToLower(string(body))
	for _, sig := range failureSignatures {
		for _, sub := range sig.substrings {
			if strings.Contains(lower, sub) {
				return sig.reason
			}
		}
	}
	return ""
}

// tokenUsage is the token bucket OpenCode reports per assistant step in its
// JSON output. Fields not emitted by a given version stay zero.
type tokenUsage struct {
	Input     int `json:"input"`
	Output    int `json:"output"`
	Reasoning int `json:"reasoning"`
	Cache     struct {
		Read  int `json:"read"`
		Write int `json:"write"`
	} `json:"cache"`
}

// ComputeMetrics builds a run's metrics.json. Token totals come from OpenCode's
// usage events when present (measured); otherwise they are estimated from the
// mode's startup schema tokens plus the transcript bytes (estimated). Missing
// usage never fails a run. ret supplies the server-side invocation log and the
// scenario's canonical set for retrieval-quality metrics.
func ComputeMetrics(result *RunResult, surfaceTokens int, est eval.TokenEstimator, ret RetrievalInput) *RunMetrics {
	if est == nil {
		est = eval.DefaultEstimator
	}
	hit, total, distractor, downstream := computeRetrieval(ret)
	m := &RunMetrics{
		RunID:               result.RunID,
		Mode:                result.Mode,
		Success:             result.Success,
		TimedOut:            result.TimedOut,
		ParseFailed:         result.ParseFailed,
		FailureReason:       classifyFailure(result),
		DurationSec:         round2(result.DurationSec),
		ToolCallCount:       len(result.ToolCalls),
		CanonicalToolsHit:   hit,
		CanonicalToolsTotal: total,
		DistractorCallCount: distractor,
		DownstreamCallCount: downstream,
	}

	if u, ok := parseUsage(result.Transcript); ok {
		m.InputTokens = u.Input
		m.OutputTokens = u.Output
		m.ReasoningTokens = u.Reasoning
		m.CacheReadTokens = u.Cache.Read
		m.CacheWriteTokens = u.Cache.Write
		m.TokenSource = TokenSourceMeasured
	} else {
		// Estimated: the mode's advertised startup schema tokens plus the
		// transcript bytes (tool results + agent text) approximate input; the
		// final answer approximates output.
		//nolint:gosec // G304: transcript is a controlled artifact path.
		body, _ := os.ReadFile(result.Transcript)
		m.InputTokens = surfaceTokens + est.Estimate(string(body))
		m.OutputTokens = est.Estimate(result.FinalAnswer)
		m.TokenSource = TokenSourceEstimated
	}
	m.TotalTokens = m.InputTokens + m.OutputTokens

	// Wastage from distractor calls.
	if m.DownstreamCallCount > 0 {
		m.WastedCallRatio = round2(float64(m.DistractorCallCount) / float64(m.DownstreamCallCount))
		avgPerCall := float64(m.TotalTokens) / float64(m.DownstreamCallCount)
		m.WastedTokensEst = int(math.Round(avgPerCall * float64(m.DistractorCallCount)))
	}
	return m
}

// parseUsage sums the token usage OpenCode reports across the transcript's
// assistant steps. Each step's input/output is a real request cost, so summing
// gives the run's total token spend. Returns ok=false when no usable usage
// events are present (the caller then estimates).
//
// ponytail: exact OpenCode usage-event shape is pinned-version-specific and is
// confirmed against a real transcript in task 5.2; the tolerant top/part/info
// probe below plus the estimated fallback keep every artifact flowing either way.
//
//nolint:gosec // G304: path is a controlled artifact path in bench output.
func parseUsage(path string) (tokenUsage, bool) {
	f, err := os.Open(path)
	if err != nil {
		return tokenUsage{}, false
	}
	defer func() { _ = f.Close() }()

	var total tokenUsage
	found := false
	dec := json.NewDecoder(f)
	for {
		var ev struct {
			Tokens *tokenUsage `json:"tokens"`
			Part   struct {
				Tokens *tokenUsage `json:"tokens"`
			} `json:"part"`
			Info struct {
				Tokens *tokenUsage `json:"tokens"`
			} `json:"info"`
		}
		if err := dec.Decode(&ev); err != nil {
			break
		}
		u := ev.Tokens
		if u == nil {
			u = ev.Part.Tokens
		}
		if u == nil {
			u = ev.Info.Tokens
		}
		if u == nil || (u.Input == 0 && u.Output == 0 && u.Reasoning == 0) {
			continue
		}
		total.Input += u.Input
		total.Output += u.Output
		total.Reasoning += u.Reasoning
		total.Cache.Read += u.Cache.Read
		total.Cache.Write += u.Cache.Write
		found = true
	}
	return total, found
}

// WriteMetrics writes a run's metrics.json.
func WriteMetrics(path string, m *RunMetrics) error {
	return writeJSON(path, m)
}

// Stats holds summary statistics over a run batch's samples.
type Stats struct {
	Mean  float64 `json:"mean"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Stdev float64 `json:"stdev"`
}

// Aggregate is the per-mode aggregate artifact (aggregate.json).
type Aggregate struct {
	Mode         string `json:"mode"`
	N            int    `json:"n"`
	SuccessK     int    `json:"successK"`
	TimedOut     int    `json:"timedOut"`
	ParseFailed  int    `json:"parseFailed"`
	TokenSource  string `json:"tokenSource"`
	Duration     Stats  `json:"durationSec"`
	ToolCalls    Stats  `json:"toolCalls"`
	InputTokens  Stats  `json:"inputTokens"`
	OutputTokens Stats  `json:"outputTokens"`
	TotalTokens  Stats  `json:"totalTokens"`

	// Retrieval quality (means over the batch) and named-failure counts.
	CanonicalHit    Stats          `json:"canonicalHit"`    // mean distinct required tools called
	CanonicalTotal  int            `json:"canonicalTotal"`  // N required tools (constant per scenario)
	DistractorCalls Stats          `json:"distractorCalls"` // mean distractor calls/run
	DownstreamCalls Stats          `json:"downstreamCalls"` // mean server-side calls/run
	WastedTokensEst Stats          `json:"wastedTokensEst"` // mean estimated tokens wasted on distractor calls
	FailureReasons  map[string]int `json:"failureReasons,omitempty"`
}

// SuccessRate returns k/N as a fraction (0 when N==0).
func (a *Aggregate) SuccessRate() float64 {
	if a == nil || a.N == 0 {
		return 0
	}
	return float64(a.SuccessK) / float64(a.N)
}

// ComputeAggregate summarizes a mode's run batch. When runs carry different
// token sources the aggregate is labeled "mixed" rather than silently pooling.
func ComputeAggregate(mode string, runs []*RunMetrics) *Aggregate {
	a := &Aggregate{Mode: mode, N: len(runs), TokenSource: TokenSourceNone}
	if len(runs) == 0 {
		return a
	}

	var durations, toolCalls, inputs, outputs, totals []float64
	var canonHits, distractors, downstreams, wasted []float64
	sources := map[string]bool{}
	reasons := map[string]int{}
	for _, r := range runs {
		if r.Success {
			a.SuccessK++
		}
		if r.TimedOut {
			a.TimedOut++
		}
		if r.ParseFailed {
			a.ParseFailed++
		}
		if r.FailureReason != "" {
			reasons[r.FailureReason]++
		}
		durations = append(durations, r.DurationSec)
		toolCalls = append(toolCalls, float64(r.ToolCallCount))
		inputs = append(inputs, float64(r.InputTokens))
		outputs = append(outputs, float64(r.OutputTokens))
		totals = append(totals, float64(r.TotalTokens))
		canonHits = append(canonHits, float64(r.CanonicalToolsHit))
		distractors = append(distractors, float64(r.DistractorCallCount))
		downstreams = append(downstreams, float64(r.DownstreamCallCount))
		wasted = append(wasted, float64(r.WastedTokensEst))
		a.CanonicalTotal = r.CanonicalToolsTotal
		sources[r.TokenSource] = true
	}

	a.Duration = computeStats(durations)
	a.ToolCalls = computeStats(toolCalls)
	a.InputTokens = computeStats(inputs)
	a.OutputTokens = computeStats(outputs)
	a.TotalTokens = computeStats(totals)
	a.CanonicalHit = computeStats(canonHits)
	a.DistractorCalls = computeStats(distractors)
	a.DownstreamCalls = computeStats(downstreams)
	a.WastedTokensEst = computeStats(wasted)
	if len(reasons) > 0 {
		a.FailureReasons = reasons
	}

	switch {
	case len(sources) == 1:
		for s := range sources {
			a.TokenSource = s
		}
	default:
		a.TokenSource = TokenSourceMixed
	}
	return a
}

// WriteAggregate writes a mode's aggregate.json.
func WriteAggregate(path string, a *Aggregate) error {
	return writeJSON(path, a)
}

// computeStats returns mean/min/max/population-stdev over vals.
func computeStats(vals []float64) Stats {
	if len(vals) == 0 {
		return Stats{}
	}
	minV, maxV, sum := vals[0], vals[0], 0.0
	for _, v := range vals {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
		sum += v
	}
	mean := sum / float64(len(vals))
	var sq float64
	for _, v := range vals {
		d := v - mean
		sq += d * d
	}
	return Stats{
		Mean:  round2(mean),
		Min:   round2(minV),
		Max:   round2(maxV),
		Stdev: round2(math.Sqrt(sq / float64(len(vals)))),
	}
}

// usageSourceOf reduces the per-mode aggregates to one invocation-level usage
// source for provenance: measured, estimated, mixed, or skipped.
func usageSourceOf(aggregates map[string]*Aggregate) string {
	sources := map[string]bool{}
	for _, a := range aggregates {
		if a == nil || a.N == 0 {
			continue
		}
		sources[a.TokenSource] = true
	}
	switch {
	case len(sources) == 0:
		return TokenSourceSkipped
	case len(sources) == 1:
		for s := range sources {
			return s
		}
	}
	return TokenSourceMixed
}

// writeJSON writes v as indented JSON to path, creating parent dirs.
//
//nolint:gosec // G301,G304: paths are controlled bench artifact paths.
func writeJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode %s: %w", path, err)
	}
	return nil
}

// round2 rounds to two decimal places for stable, readable JSON.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// Comparison is the cross-mode comparison artifact (comparison.json), combining
// the always-on surface tier with both modes' live aggregates and a descriptive
// verdict. It applies no pass/fail thresholds — it states facts.
type Comparison struct {
	Scenario       string                `json:"scenario"`
	Model          string                `json:"model"`
	Estimator      string                `json:"estimator"`
	Retrieval      string                `json:"retrieval,omitempty"`
	ProvenanceRef  string                `json:"provenanceRef"`
	Surface        *SurfaceComparison    `json:"surface"`
	LiveSkipped    bool                  `json:"liveSkipped"`
	TokenSource    string                `json:"tokenSource,omitempty"`
	RequestedModes []string              `json:"requestedModes,omitempty"`
	Modes          map[string]*Aggregate `json:"modes,omitempty"`
	Verdict        []string              `json:"verdict"`
}

// modeSetting reconstructs the invocation's MODE setting from the requested
// modes, for the "not run (MODE=…)" label.
func (c *Comparison) modeSetting() string {
	switch len(c.RequestedModes) {
	case 3:
		return "all"
	case 2:
		return "both"
	case 1:
		return c.RequestedModes[0]
	}
	return "?"
}

// modeRan reports whether the given mode was part of the invocation.
func (c *Comparison) modeRan(mode string) bool {
	for _, m := range c.RequestedModes {
		if m == mode {
			return true
		}
	}
	return false
}

// WriteComparison writes comparison.json and comparison.md into dir. When
// aggregates is nil (surface-only), the live sections are marked skipped and the
// comparison is built from the surface tier alone.
func WriteComparison(dir string, surface *SurfaceComparison, aggregates map[string]*Aggregate, prov *EnvironmentRecord) error {
	c := &Comparison{
		Surface:       surface,
		ProvenanceRef: "environment.json",
		LiveSkipped:   len(aggregates) == 0,
	}
	if prov != nil {
		c.Scenario = prov.ScenarioHash
		c.Model = prov.ModelID
		c.Estimator = prov.TokenEstimator
		c.Retrieval = prov.Retrieval
		c.RequestedModes = prov.Modes
	}
	if len(aggregates) > 0 {
		c.Modes = aggregates
		c.TokenSource = usageSourceOf(aggregates)
	}
	c.Verdict = buildVerdict(c, surface, aggregates)

	if err := writeJSON(filepath.Join(dir, "comparison.json"), c); err != nil {
		return err
	}
	return writeComparisonMarkdown(filepath.Join(dir, "comparison.md"), c)
}

// buildVerdict states the headline facts a reader can quote without opening any
// other file: the surface delta always, plus live success/token/retrieval deltas
// when the live tier ran, and an explicit reason when a live delta is unavailable.
func buildVerdict(c *Comparison, surface *SurfaceComparison, aggregates map[string]*Aggregate) []string {
	var v []string
	if surface != nil {
		pct := 0.0
		if surface.Direct.SchemaTokens > 0 {
			pct = (1 - float64(surface.Ozy.SchemaTokens)/float64(surface.Direct.SchemaTokens)) * 100
		}
		v = append(v, fmt.Sprintf("Startup surface: ozy advertises %d tools vs direct's %d (%d fewer), %.0f%% fewer schema tokens (%d vs %d).",
			surface.Ozy.ToolsVisible, surface.Direct.ToolsVisible, surface.Reduction.ToolCount,
			pct, surface.Ozy.SchemaTokens, surface.Direct.SchemaTokens))
	}
	if len(aggregates) == 0 {
		v = append(v, "Live tier skipped (surface-only): success and token-economy deltas not measured.")
		return v
	}
	direct, lean, ozy := aggregates["direct"], aggregates["direct-lean"], aggregates["ozy"]

	// Efficiency-first: each mode's line leads with the broker-value metrics —
	// total tokens, tool calls, duration — then success k/N (D2).
	modeLine := func(label string, a *Aggregate) {
		if a == nil {
			return
		}
		v = append(v, fmt.Sprintf("%s: %.0f total tokens/run, %.1f tool calls/run, %.1fs/run, %d/%d success (%s)%s.",
			label, a.TotalTokens.Mean, a.ToolCalls.Mean, a.Duration.Mean, a.SuccessK, a.N, a.TokenSource, failureNote(a)))
	}
	modeLine("Direct", direct)
	modeLine("Direct-lean", lean)
	modeLine("Ozy", ozy)

	// ozy − direct-lean is the primary real-world comparison; ozy − direct is the
	// worst-case (full-corpus) baseline. Emit lean first when present. The delta
	// leads with efficiency and closes with success — attribution is a footer.
	deltaLine := func(base string, b *Aggregate) {
		v = append(v, fmt.Sprintf("Delta (ozy − %s): total tokens %+.0f/run, tool calls %+.1f/run, duration %+.1fs/run, success %+.0f pp.",
			base,
			ozy.TotalTokens.Mean-b.TotalTokens.Mean,
			ozy.ToolCalls.Mean-b.ToolCalls.Mean,
			ozy.Duration.Mean-b.Duration.Mean,
			(ozy.SuccessRate()-b.SuccessRate())*100))
	}
	switch {
	case ozy == nil:
		v = append(v, fmt.Sprintf("Live delta unavailable: ozy was not run (MODE=%s).", c.modeSetting()))
	case lean == nil && direct == nil:
		v = append(v, fmt.Sprintf("Live delta unavailable: no baseline mode (direct or direct-lean) was run (MODE=%s).", c.modeSetting()))
	default:
		if lean != nil {
			deltaLine("direct-lean", lean)
		}
		if direct != nil {
			deltaLine("direct", direct)
		}
	}
	if usageSourceOf(aggregates) == TokenSourceMixed {
		v = append(v, "Token sources differ across runs (mixed measured/estimated) — token deltas are indicative, not exact.")
	}

	// Informational footer: tool-attribution telemetry, demoted below the verdict.
	var attrib []string
	attribLine := func(label string, a *Aggregate) {
		if a == nil {
			return
		}
		attrib = append(attrib, fmt.Sprintf("%s %.1f/%d canonical, %.1f distractor calls/run",
			label, a.CanonicalHit.Mean, a.CanonicalTotal, a.DistractorCalls.Mean))
	}
	attribLine("direct", direct)
	attribLine("direct-lean", lean)
	attribLine("ozy", ozy)
	if len(attrib) > 0 {
		v = append(v, "Informational (tool attribution, not scored): "+strings.Join(attrib, "; ")+".")
	}
	if lean != nil {
		v = append(v, "Direct-lean wires no corpus servers, so its distractor calls are ~0 by construction — its cost is the full tool surface of the MCPs the task needs, not corpus-call waste.")
	}
	return v
}

// failureNote summarizes named infra-failure reasons for a mode's verdict line.
func failureNote(a *Aggregate) string {
	if len(a.FailureReasons) == 0 {
		return ""
	}
	var parts []string
	for reason, n := range a.FailureReasons {
		parts = append(parts, fmt.Sprintf("%d×%s", n, reason))
	}
	sort.Strings(parts)
	return " [" + strings.Join(parts, ", ") + "]"
}

// ToolCount returns the mean tool-call count for the mode.
func (a *Aggregate) ToolCount() float64 { return a.ToolCalls.Mean }

// writeComparisonMarkdown renders the human-readable comparison.md.
//
//nolint:gosec // G304: path is a controlled artifact path in bench output.
func writeComparisonMarkdown(path string, c *Comparison) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create comparison.md: %w", err)
	}
	defer func() { _ = f.Close() }()

	fmt.Fprintf(f, "# Benchmark Comparison\n\n")
	if c.Model != "" {
		retrieval := c.Retrieval
		if retrieval == "" {
			retrieval = "unknown"
		}
		fmt.Fprintf(f, "**Model:** `%s`  •  **Retrieval:** %s  •  **Estimator:** %s  •  **Provenance:** [`%s`](%s)\n\n",
			c.Model, retrieval, c.Estimator, c.ProvenanceRef, c.ProvenanceRef)
	}

	fmt.Fprintf(f, "## Verdict\n\n")
	for _, line := range c.Verdict {
		fmt.Fprintf(f, "- %s\n", line)
	}
	fmt.Fprintf(f, "\n")

	if c.Surface != nil {
		s := c.Surface
		leanTools, leanTok, leanIrr := "—", "—", "—"
		leanRedTools, leanRedTok := "—", "—"
		if s.DirectLean != nil {
			leanTools = fmt.Sprintf("%d", s.DirectLean.ToolsVisible)
			leanTok = fmt.Sprintf("%d", s.DirectLean.SchemaTokens)
			leanIrr = fmt.Sprintf("%d", s.DirectLean.IrrelevantSchemaTokens)
		}
		if s.LeanReduction != nil {
			leanRedTools = fmt.Sprintf("%d", s.LeanReduction.ToolCount)
			leanRedTok = fmt.Sprintf("%d", s.LeanReduction.SchemaTokens)
		}
		fmt.Fprintf(f, "## Startup surface (deterministic, no model)\n\n")
		fmt.Fprintf(f, "| Metric | Direct | Direct-lean | Ozy | Δ vs direct | Δ vs direct-lean |\n|---|---:|---:|---:|---:|---:|\n")
		fmt.Fprintf(f, "| Tools visible | %d | %s | %d | %d | %s |\n", s.Direct.ToolsVisible, leanTools, s.Ozy.ToolsVisible, s.Reduction.ToolCount, leanRedTools)
		fmt.Fprintf(f, "| Schema tokens | %d | %s | %d | %d | %s |\n", s.Direct.SchemaTokens, leanTok, s.Ozy.SchemaTokens, s.Reduction.SchemaTokens, leanRedTok)
		fmt.Fprintf(f, "| Irrelevant schema tokens | %d | %s | %d | — | — |\n\n", s.Direct.IrrelevantSchemaTokens, leanIrr, s.Ozy.IrrelevantSchemaTokens)
	}

	if c.LiveSkipped {
		fmt.Fprintf(f, "## Live tier\n\n_Skipped (surface-only invocation)._\n")
		return nil
	}

	direct, lean, ozy := c.Modes["direct"], c.Modes["direct-lean"], c.Modes["ozy"]
	vsDirect := direct != nil && ozy != nil
	vsLean := lean != nil && ozy != nil

	fmt.Fprintf(f, "## Live tier (token source: %s)\n\n", c.TokenSource)
	fmt.Fprintf(f, "| Metric | %s | %s | %s | %s | %s |\n|---|---:|---:|---:|---:|---:|\n",
		modeHeader(c, "direct", direct), modeHeader(c, "direct-lean", lean), modeHeader(c, "ozy", ozy),
		"Δ (ozy − direct)", "Δ (ozy − direct-lean)")

	row := func(label, d, l, o, dDirect, dLean string) {
		fmt.Fprintf(f, "| %s | %s | %s | %s | %s | %s |\n", label, d, l, o, dDirect, dLean)
	}
	// mean/int delta cells against each baseline (ozy − base).
	meanD := func(get func(*Aggregate) float64) (string, string) {
		return meanDelta(vsDirect, direct, ozy, get), meanDelta(vsLean, lean, ozy, get)
	}
	intD := func(get func(*Aggregate) int) (string, string) {
		return intDelta(vsDirect, direct, ozy, get), intDelta(vsLean, lean, ozy, get)
	}
	successD := func(both bool, base *Aggregate) string {
		if !both {
			return "—"
		}
		return fmt.Sprintf("%+.0f pp", (ozy.SuccessRate()-base.SuccessRate())*100)
	}

	// Efficiency + outcome rows lead: total tokens → tool calls → duration →
	// success k/N → input/output token means (D2).
	totDD, totDL := meanD(func(a *Aggregate) float64 { return a.TotalTokens.Mean })
	row("Total tokens mean±stdev", stat(direct, func(a *Aggregate) Stats { return a.TotalTokens }), stat(lean, func(a *Aggregate) Stats { return a.TotalTokens }), stat(ozy, func(a *Aggregate) Stats { return a.TotalTokens }), totDD, totDL)
	tcDD, tcDL := meanD(func(a *Aggregate) float64 { return a.ToolCalls.Mean })
	row("Tool calls/run mean±stdev", stat(direct, func(a *Aggregate) Stats { return a.ToolCalls }), stat(lean, func(a *Aggregate) Stats { return a.ToolCalls }), stat(ozy, func(a *Aggregate) Stats { return a.ToolCalls }), tcDD, tcDL)
	durDD, durDL := meanD(func(a *Aggregate) float64 { return a.Duration.Mean })
	row("Duration mean±stdev (s)", stat(direct, func(a *Aggregate) Stats { return a.Duration }), stat(lean, func(a *Aggregate) Stats { return a.Duration }), stat(ozy, func(a *Aggregate) Stats { return a.Duration }), durDD, durDL)
	row("Success k/N", kn(direct), kn(lean), kn(ozy), successD(vsDirect, direct), successD(vsLean, lean))
	inDD, inDL := meanD(func(a *Aggregate) float64 { return a.InputTokens.Mean })
	row("Input tokens mean", statMean(direct, func(a *Aggregate) Stats { return a.InputTokens }), statMean(lean, func(a *Aggregate) Stats { return a.InputTokens }), statMean(ozy, func(a *Aggregate) Stats { return a.InputTokens }), inDD, inDL)
	outDD, outDL := meanD(func(a *Aggregate) float64 { return a.OutputTokens.Mean })
	row("Output tokens mean", statMean(direct, func(a *Aggregate) Stats { return a.OutputTokens }), statMean(lean, func(a *Aggregate) Stats { return a.OutputTokens }), statMean(ozy, func(a *Aggregate) Stats { return a.OutputTokens }), outDD, outDL)

	// Informational footer: tool attribution + infra-failure counts, demoted
	// below the efficiency/outcome rows — no metric removed, only the order (D2).
	row("_Informational (tool attribution)_", "", "", "", "", "")
	canonDD, canonDL := meanD(func(a *Aggregate) float64 { return a.CanonicalHit.Mean })
	row("Canonical tools hit", canonCell(direct), canonCell(lean), canonCell(ozy), canonDD, canonDL)
	distDD, distDL := meanD(func(a *Aggregate) float64 { return a.DistractorCalls.Mean })
	row("Distractor calls/run", meanCell(direct, func(a *Aggregate) Stats { return a.DistractorCalls }), meanCell(lean, func(a *Aggregate) Stats { return a.DistractorCalls }), meanCell(ozy, func(a *Aggregate) Stats { return a.DistractorCalls }), distDD, distDL)
	downDD, downDL := meanD(func(a *Aggregate) float64 { return a.DownstreamCalls.Mean })
	row("Downstream calls/run", meanCell(direct, func(a *Aggregate) Stats { return a.DownstreamCalls }), meanCell(lean, func(a *Aggregate) Stats { return a.DownstreamCalls }), meanCell(ozy, func(a *Aggregate) Stats { return a.DownstreamCalls }), downDD, downDL)
	wasteDD, wasteDL := meanD(func(a *Aggregate) float64 { return a.WastedTokensEst.Mean })
	row("Wasted tokens/run (est)", statMean(direct, func(a *Aggregate) Stats { return a.WastedTokensEst }), statMean(lean, func(a *Aggregate) Stats { return a.WastedTokensEst }), statMean(ozy, func(a *Aggregate) Stats { return a.WastedTokensEst }), wasteDD, wasteDL)
	toDD, toDL := intD(func(a *Aggregate) int { return a.TimedOut })
	row("Timed out", intCell(direct, func(a *Aggregate) int { return a.TimedOut }), intCell(lean, func(a *Aggregate) int { return a.TimedOut }), intCell(ozy, func(a *Aggregate) int { return a.TimedOut }), toDD, toDL)
	pfDD, pfDL := intD(func(a *Aggregate) int { return a.ParseFailed })
	row("Parse failed", intCell(direct, func(a *Aggregate) int { return a.ParseFailed }), intCell(lean, func(a *Aggregate) int { return a.ParseFailed }), intCell(ozy, func(a *Aggregate) int { return a.ParseFailed }), pfDD, pfDL)
	row("Failure reasons", reasonCell(direct), reasonCell(lean), reasonCell(ozy), "—", "—")
	fmt.Fprintf(f, "\n_No pass/fail thresholds are applied — the live tier is informational._\n")
	return nil
}

// modeHeader labels a mode's column: its name, or "not run (MODE=…)" when the
// mode was not part of the invocation — never a bare "—".
func modeHeader(c *Comparison, mode string, a *Aggregate) string {
	if a == nil && !c.modeRan(mode) {
		return fmt.Sprintf("%s (not run, MODE=%s)", capitalize(mode), c.modeSetting())
	}
	return capitalize(mode)
}

// capitalize upper-cases the first byte (ASCII mode names only).
func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func kn(a *Aggregate) string {
	if a == nil {
		return "—"
	}
	return fmt.Sprintf("%d/%d", a.SuccessK, a.N)
}

func canonCell(a *Aggregate) string {
	if a == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f/%d", a.CanonicalHit.Mean, a.CanonicalTotal)
}

func reasonCell(a *Aggregate) string {
	if a == nil {
		return "—"
	}
	if len(a.FailureReasons) == 0 {
		return "none"
	}
	return strings.TrimPrefix(failureNote(a), " ")
}

func intCell(a *Aggregate, get func(*Aggregate) int) string {
	if a == nil {
		return "—"
	}
	return fmt.Sprintf("%d", get(a))
}

func meanCell(a *Aggregate, get func(*Aggregate) Stats) string {
	if a == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f", get(a).Mean)
}

func stat(a *Aggregate, get func(*Aggregate) Stats) string {
	if a == nil {
		return "—"
	}
	s := get(a)
	return fmt.Sprintf("%.1f±%.1f", s.Mean, s.Stdev)
}

func statMean(a *Aggregate, get func(*Aggregate) Stats) string {
	if a == nil {
		return "—"
	}
	return fmt.Sprintf("%.0f", get(a).Mean)
}

// meanDelta renders a signed ozy−direct mean difference, or "—" when a mode is
// absent.
func meanDelta(both bool, direct, ozy *Aggregate, get func(*Aggregate) float64) string {
	if !both {
		return "—"
	}
	return fmt.Sprintf("%+.1f", get(ozy)-get(direct))
}

func intDelta(both bool, direct, ozy *Aggregate, get func(*Aggregate) int) string {
	if !both {
		return "—"
	}
	return fmt.Sprintf("%+d", get(ozy)-get(direct))
}
