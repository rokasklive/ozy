package eval

import (
	"fmt"
	"strings"
	"time"
)

// SchemaVersion stamps the run-result shape so snapshots from different builds can
// be compared and migrated.
const SchemaVersion = "ozy-eval/v1"

// Verdict values.
const (
	VerdictPass = "pass"
	VerdictFail = "fail"
)

// Provenance records exactly what produced a run so any number is attributable and
// reproducible (SPEC.md §14 grading discipline).
type Provenance struct {
	CorpusVersion  int    `json:"corpusVersion"`
	Model          string `json:"model"`
	SemanticRan    bool   `json:"semanticRan"`
	TokenEstimator string `json:"tokenEstimator,omitempty"`
	GitCommit      string `json:"gitCommit"`
	Host           string `json:"host,omitempty"`
}

// RunResult is the single structured object a harness run emits: every computed
// metric, the provenance, the gate outcomes, and the overall verdict. It is both
// the JSON snapshot and the source the Markdown scoreboard is generated from.
type RunResult struct {
	Schema       string                     `json:"schema"`
	GeneratedAt  time.Time                  `json:"generatedAt"`
	Provenance   Provenance                 `json:"provenance"`
	Discovery    *DiscoveryReport           `json:"discovery,omitempty"`
	Invocation   *InvocationReport          `json:"invocation,omitempty"`
	Ergonomics   *ErgonomicsReport          `json:"ergonomics,omitempty"`
	TokenEconomy *TokenEconomyMetrics       `json:"tokenEconomy,omitempty"`
	Performance  *LatencyReport             `json:"performance,omitempty"`
	Cache        *CacheEffectivenessMetrics `json:"cache,omitempty"`
	Hygiene      []HygieneFinding           `json:"hygiene,omitempty"`
	Gates        []GateResult               `json:"gates"`
	Verdict      string                     `json:"verdict"`
}

// Failed reports whether the run's verdict is fail (used for the process exit
// status so CI can gate on `ozy eval run`).
func (r *RunResult) Failed() bool { return r.Verdict == VerdictFail }

// gateTally counts gate outcomes for summaries.
func (r *RunResult) gateTally() (passed, failed, skipped int) {
	for _, g := range r.Gates {
		switch {
		case g.Skipped:
			skipped++
		case g.Pass:
			passed++
		default:
			failed++
		}
	}
	return
}

// Render produces the human/concise text form of a run result so `ozy eval`
// output is readable without --format json. JSON mode marshals the struct.
func (r *RunResult) Render(format string) string {
	passed, failed, skipped := r.gateTally()
	if format == "concise" {
		top1 := 0.0
		if r.Discovery != nil {
			top1 = r.Discovery.Overall.Top1
		}
		return fmt.Sprintf("eval %s top1=%.3f gates=%d/%d", strings.ToUpper(r.Verdict), top1, passed, passed+failed)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "eval: %s  (model=%s, semantic=%s)\n", strings.ToUpper(r.Verdict), r.Provenance.Model, ranWord(r.Provenance.SemanticRan))
	if r.Discovery != nil {
		o := r.Discovery.Overall
		fmt.Fprintf(&b, "discovery overall: top1=%.1f%% top3=%.1f%% mrr=%.3f (N=%d)\n", o.Top1*100, o.Top3*100, o.MRR, o.N)
	}
	if r.Invocation != nil {
		m := r.Invocation.Overall
		fmt.Fprintf(&b, "invocation: firstCall=%.1f%% repair=%.1f%% errorClarity=%.1f%% (N=%d)\n", m.FirstCallSuccess*100, m.RepairSuccess*100, m.ErrorClarity*100, m.N)
	}
	if r.Ergonomics != nil {
		m := r.Ergonomics.Overall
		fmt.Fprintf(&b, "ergonomics: decision=%.1f%% instruction=%.1f%% parity=%.1f%% (N=%d)\n", m.DecisionRate*100, m.InstructionRate*100, m.ParityRate*100, m.N)
	}
	if r.TokenEconomy != nil {
		fmt.Fprintf(&b, "tokens: startup %d→%d (−%.0f%%)\n", r.TokenEconomy.DirectStartupTokens, r.TokenEconomy.OzyStartupTokens, r.TokenEconomy.StartupReductionRatio*100)
	}
	if r.Cache != nil {
		fmt.Fprintf(&b, "cache: redundant-call reduction %.1f%% (%d/%d served), %d tokens avoided\n",
			r.Cache.RedundantCallReduction*100, r.Cache.ServedFromCache, r.Cache.CacheableOps, r.Cache.TokensAvoided)
	}
	fmt.Fprintf(&b, "gates: %d passed, %d failed, %d skipped", passed, failed, skipped)
	if len(r.Hygiene) > 0 {
		fmt.Fprintf(&b, "\nhygiene warnings: %d (see snapshot)", len(r.Hygiene))
	}
	return strings.TrimRight(b.String(), "\n")
}
