package eval

import (
	"context"
	"testing"
)

func TestRunErgonomicsEmbedded(t *testing.T) {
	corpus := mustLoad(t)
	report, err := RunErgonomics(context.Background(), corpus)
	if err != nil {
		t.Fatalf("RunErgonomics error = %v", err)
	}
	m := report.Overall
	if m.N != len(corpus.Ergonomics) {
		t.Errorf("N = %d, want %d", m.N, len(corpus.Ergonomics))
	}
	for _, c := range []struct {
		name string
		val  float64
	}{
		{"decisionRate", m.DecisionRate},
		{"instructionRate", m.InstructionRate},
		{"errorDispositionRate", m.ErrorDispositionRate},
		{"withinBudgetRate", m.WithinBudgetRate},
		{"parityRate", m.ParityRate},
	} {
		if c.val != 1.0 {
			t.Errorf("%s = %.3f, want 1.0 over the committed corpus", c.name, c.val)
		}
	}
	if len(report.NonInstructional) != 0 {
		t.Errorf("non-instructional flags = %v, want none", report.NonInstructional)
	}
	if len(report.ParityMismatches) != 0 {
		t.Errorf("CLI↔MCP parity mismatches = %v, want none", report.ParityMismatches)
	}
	if len(report.BudgetExceeded) != 0 {
		t.Errorf("budget-exceeded = %v, want none", report.BudgetExceeded)
	}
}

func TestIsQueryRestating(t *testing.T) {
	const query = "find the deployment runbook"
	tests := []struct {
		name  string
		instr string
		want  bool
	}{
		{"bare restatement", "find the deployment runbook", true},
		{"actionable instruction", "Use describeTool on the selected tool before calling it.", false},
		{"restatement with action", "find the deployment runbook — run describeTool next", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isQueryRestating(tt.instr, query); got != tt.want {
				t.Errorf("isQueryRestating(%q) = %v, want %v", tt.instr, got, tt.want)
			}
		})
	}
}
