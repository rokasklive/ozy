package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
)

// Thresholds are the versioned, data-driven gate thresholds. Fields left unset
// (nil) are not gated, so the gate set can be ratcheted by editing data only.
type Thresholds struct {
	Version    int                      `json:"version"`
	Note       string                   `json:"_note,omitempty"`
	Discovery  map[string]DiscoveryGate `json:"discovery"`
	Invocation *InvocationGate          `json:"invocation,omitempty"`
	Ergonomics *ErgonomicsGate          `json:"ergonomics,omitempty"`
	Tokens     *TokenGate               `json:"tokens,omitempty"`
	Cache      *CacheGate               `json:"cache,omitempty"`
}

// InvocationGate is the set of gated invocation metrics. All *Min fields gate
// accuracy (higher is better); any left unset is not gated.
type InvocationGate struct {
	ValidArgumentRateMin  *float64 `json:"validArgumentRateMin,omitempty"`
	FirstCallSuccessMin   *float64 `json:"firstCallSuccessMin,omitempty"`
	RepairSuccessMin      *float64 `json:"repairSuccessMin,omitempty"`
	SchemaErrorRateMin    *float64 `json:"schemaErrorRateMin,omitempty"`
	OfflineHandledRateMin *float64 `json:"offlineHandledRateMin,omitempty"`
	ErrorClarityMin       *float64 `json:"errorClarityMin,omitempty"`
}

// ErgonomicsGate is the set of gated ergonomics/parity metrics (all *Min).
type ErgonomicsGate struct {
	DecisionRateMin         *float64 `json:"decisionRateMin,omitempty"`
	InstructionRateMin      *float64 `json:"instructionRateMin,omitempty"`
	ErrorDispositionRateMin *float64 `json:"errorDispositionRateMin,omitempty"`
	WithinBudgetRateMin     *float64 `json:"withinBudgetRateMin,omitempty"`
	ParityRateMin           *float64 `json:"parityRateMin,omitempty"`
}

// TokenGate gates the token-economy headline (startup reduction ratio).
type TokenGate struct {
	StartupReductionMin *float64 `json:"startupReductionMin,omitempty"`
}

// CacheGate gates the cache-effectiveness headline (redundant-call reduction).
type CacheGate struct {
	RedundantCallReductionMin *float64 `json:"redundantCallReductionMin,omitempty"`
}

// DiscoveryGate is the set of gated metrics for one discovery scope (a category
// key such as "lexical", or "overall"). *Min fields gate accuracy (higher is
// better); *Max fields gate rates (lower is better).
type DiscoveryGate struct {
	Top1Min               *float64 `json:"top1Min,omitempty"`
	Top3Min               *float64 `json:"top3Min,omitempty"`
	MRRMin                *float64 `json:"mrrMin,omitempty"`
	WrongServerRateMax    *float64 `json:"wrongServerRateMax,omitempty"`
	NoMatchCorrectnessMin *float64 `json:"noMatchCorrectnessMin,omitempty"`
	RequiresSemanticLeg   bool     `json:"requiresSemanticLeg,omitempty"`
}

// GateResult is the outcome of one threshold check.
type GateResult struct {
	Name       string  `json:"name"`
	Comparator string  `json:"comparator"` // "min" or "max"
	Threshold  float64 `json:"threshold"`
	Actual     float64 `json:"actual"`
	Pass       bool    `json:"pass"`
	Skipped    bool    `json:"skipped,omitempty"`
	SkipReason string  `json:"skipReason,omitempty"`
}

// LoadThresholds reads and parses the gate thresholds from fsys.
func LoadThresholds(fsys fs.FS) (*Thresholds, error) {
	data, err := fs.ReadFile(fsys, "thresholds.json")
	if err != nil {
		return nil, fmt.Errorf("thresholds.json: %w", err)
	}
	var t Thresholds
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("thresholds.json: %w", err)
	}
	return &t, nil
}

// EvaluateDiscovery turns the discovery thresholds into gate results against the
// report. Gates marked requiresSemanticLeg are skipped (not failed) when the run
// did not exercise the real embedding model. Results are returned in a stable
// (scope-sorted) order for deterministic snapshots.
func (t *Thresholds) EvaluateDiscovery(report *DiscoveryReport, semanticRan bool) []GateResult {
	if report == nil {
		return nil
	}
	scopes := make([]string, 0, len(t.Discovery))
	for scope := range t.Discovery {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)

	var out []GateResult
	for _, scope := range scopes {
		gate := t.Discovery[scope]
		m, ok := metricsForScope(report, scope)
		skip := gate.RequiresSemanticLeg && !semanticRan
		add := func(name, comparator string, threshold *float64, actual float64) {
			if threshold == nil {
				return
			}
			g := GateResult{
				Name:       "discovery." + scope + "." + name,
				Comparator: comparator,
				Threshold:  *threshold,
				Actual:     actual,
			}
			switch {
			case skip:
				g.Skipped = true
				g.SkipReason = "semantic leg did not run"
			case !ok:
				g.Skipped = true
				g.SkipReason = "no cases for scope " + scope
			case comparator == "max":
				g.Pass = actual <= *threshold
			default:
				g.Pass = actual >= *threshold
			}
			out = append(out, g)
		}
		add("top1", "min", gate.Top1Min, m.Top1)
		add("top3", "min", gate.Top3Min, m.Top3)
		add("mrr", "min", gate.MRRMin, m.MRR)
		add("wrongServerRate", "max", gate.WrongServerRateMax, m.WrongServerRate)
		add("noMatchCorrectness", "min", gate.NoMatchCorrectnessMin, m.NoMatchCorrectness)
	}
	return out
}

// addMinGate appends a "higher is better" gate to out when the threshold is set.
func addMinGate(out *[]GateResult, name string, threshold *float64, actual float64) {
	if threshold == nil {
		return
	}
	*out = append(*out, GateResult{Name: name, Comparator: "min", Threshold: *threshold, Actual: actual, Pass: actual >= *threshold})
}

// EvaluateInvocation turns the invocation thresholds into gate results.
func (t *Thresholds) EvaluateInvocation(r *InvocationReport) []GateResult {
	if t.Invocation == nil || r == nil {
		return nil
	}
	g, m := t.Invocation, r.Overall
	var out []GateResult
	addMinGate(&out, "invocation.validArgumentRate", g.ValidArgumentRateMin, m.ValidArgumentRate)
	addMinGate(&out, "invocation.firstCallSuccess", g.FirstCallSuccessMin, m.FirstCallSuccess)
	addMinGate(&out, "invocation.repairSuccess", g.RepairSuccessMin, m.RepairSuccess)
	addMinGate(&out, "invocation.schemaErrorRate", g.SchemaErrorRateMin, m.SchemaErrorRate)
	addMinGate(&out, "invocation.offlineHandledRate", g.OfflineHandledRateMin, m.OfflineHandledRate)
	addMinGate(&out, "invocation.errorClarity", g.ErrorClarityMin, m.ErrorClarity)
	return out
}

// EvaluateErgonomics turns the ergonomics thresholds into gate results.
func (t *Thresholds) EvaluateErgonomics(r *ErgonomicsReport) []GateResult {
	if t.Ergonomics == nil || r == nil {
		return nil
	}
	g, m := t.Ergonomics, r.Overall
	var out []GateResult
	addMinGate(&out, "ergonomics.decisionRate", g.DecisionRateMin, m.DecisionRate)
	addMinGate(&out, "ergonomics.instructionRate", g.InstructionRateMin, m.InstructionRate)
	addMinGate(&out, "ergonomics.errorDispositionRate", g.ErrorDispositionRateMin, m.ErrorDispositionRate)
	addMinGate(&out, "ergonomics.withinBudgetRate", g.WithinBudgetRateMin, m.WithinBudgetRate)
	addMinGate(&out, "ergonomics.parityRate", g.ParityRateMin, m.ParityRate)
	return out
}

// EvaluateTokens turns the token-economy threshold into a gate result.
func (t *Thresholds) EvaluateTokens(m *TokenEconomyMetrics) []GateResult {
	if t.Tokens == nil || m == nil {
		return nil
	}
	var out []GateResult
	addMinGate(&out, "tokens.startupReduction", t.Tokens.StartupReductionMin, m.StartupReductionRatio)
	return out
}

// EvaluateCache turns the cache-effectiveness threshold into a gate result.
func (t *Thresholds) EvaluateCache(m *CacheEffectivenessMetrics) []GateResult {
	if t.Cache == nil || m == nil {
		return nil
	}
	var out []GateResult
	addMinGate(&out, "cache.redundantCallReduction", t.Cache.RedundantCallReductionMin, m.RedundantCallReduction)
	return out
}

func metricsForScope(report *DiscoveryReport, scope string) (DiscoveryMetrics, bool) {
	if scope == "overall" {
		return report.Overall, report.Overall.N > 0
	}
	m, ok := report.ByCategory[scope]
	return m, ok
}

// verdict returns VerdictPass unless any non-skipped gate failed.
func verdict(gates []GateResult) string {
	for _, g := range gates {
		if !g.Skipped && !g.Pass {
			return VerdictFail
		}
	}
	return VerdictPass
}
