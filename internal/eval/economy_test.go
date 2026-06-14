package eval

import (
	"context"
	"testing"
)

func TestRunTokenEconomy(t *testing.T) {
	corpus := mustLoad(t)
	m, err := RunTokenEconomy(context.Background(), corpus, nil)
	if err != nil {
		t.Fatalf("RunTokenEconomy error = %v", err)
	}
	if m.OzyStartupTokens >= m.DirectStartupTokens {
		t.Errorf("Ozy startup (%d) should be far below direct-MCP startup (%d)", m.OzyStartupTokens, m.DirectStartupTokens)
	}
	if m.StartupReductionRatio <= 0.5 {
		t.Errorf("startup reduction = %.3f, want > 0.5 (the §13 headline)", m.StartupReductionRatio)
	}
	if m.BrokerCalls != 3 {
		t.Errorf("brokerCalls = %d, want 3 (find→describe→call)", m.BrokerCalls)
	}
	if m.LargestPayloadTokens <= 0 {
		t.Error("largest payload should be measured")
	}
	if m.Estimator == "" {
		t.Error("token economy must record which estimator produced the numbers")
	}
}

// fixedEstimator is a swap-in estimator: one token per rune. It proves the
// estimator is a seam — only the implementation changes, and the recorded name
// follows it (SPEC.md §13).
type fixedEstimator struct{}

func (fixedEstimator) Name() string          { return "one-per-rune (test)" }
func (fixedEstimator) Estimate(s string) int { return len([]rune(s)) }

func TestTokenEstimatorIsSwappable(t *testing.T) {
	corpus := mustLoad(t)
	def, err := RunTokenEconomy(context.Background(), corpus, nil)
	if err != nil {
		t.Fatalf("default RunTokenEconomy error = %v", err)
	}
	swapped, err := RunTokenEconomy(context.Background(), corpus, fixedEstimator{})
	if err != nil {
		t.Fatalf("swapped RunTokenEconomy error = %v", err)
	}
	if swapped.Estimator != "one-per-rune (test)" {
		t.Errorf("estimator name = %q, want the swapped estimator", swapped.Estimator)
	}
	// The per-rune estimator counts ~4x the chars/4 heuristic, so the absolute
	// numbers move while the structural story (Ozy ≪ direct) is preserved.
	if swapped.DirectStartupTokens <= def.DirectStartupTokens {
		t.Errorf("swapped direct startup (%d) should exceed the chars/4 default (%d)", swapped.DirectStartupTokens, def.DirectStartupTokens)
	}
	if swapped.OzyStartupTokens >= swapped.DirectStartupTokens {
		t.Error("startup story (Ozy ≪ direct) must be robust to estimator choice")
	}
}
