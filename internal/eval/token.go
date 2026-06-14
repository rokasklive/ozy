package eval

import "encoding/json"

// TokenEstimator approximates how many model tokens a string costs. Agent context
// tokens are model-specific, so the estimator is an explicit, swappable seam; the
// committed numbers record which estimator produced them (SPEC.md §13). A real BPE
// tokenizer can replace the default without touching the metric code.
type TokenEstimator interface {
	// Name identifies the estimator in run provenance.
	Name() string
	// Estimate returns the approximate token count of text.
	Estimate(text string) int
}

// heuristicEstimator approximates tokens at ~4 characters per token — the common
// rule of thumb for English prose and JSON payloads. The direct-MCP-vs-Ozy
// comparison §13 cares about is a ratio, which is robust to this approximation.
type heuristicEstimator struct{}

func (heuristicEstimator) Name() string { return "chars/4 heuristic" }

func (heuristicEstimator) Estimate(text string) int {
	n := len([]rune(text))
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

// DefaultEstimator is the estimator used unless a caller swaps it.
var DefaultEstimator TokenEstimator = heuristicEstimator{}

// estimateJSON estimates the token cost of v's JSON encoding.
func estimateJSON(est TokenEstimator, v any) int {
	b, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return est.Estimate(string(b))
}
