package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/contract"
)

func TestOutput_JSONIsSingleDocument(t *testing.T) {
	t.Parallel()
	res := &contract.FindResult{Query: "q", Decision: contract.DecisionCatalogEmpty}

	var buf bytes.Buffer
	if err := Output(&buf, contract.FormatJSON, res); err != nil {
		t.Fatalf("Output() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not a single valid JSON document: %v", err)
	}
	if decoded["decision"] != contract.DecisionCatalogEmpty {
		t.Errorf("decision = %v, want %q", decoded["decision"], contract.DecisionCatalogEmpty)
	}
}

func TestOutput_HumanUsesRender(t *testing.T) {
	t.Parallel()
	res := &contract.FindResult{Query: "q", Decision: contract.DecisionCatalogEmpty, AgentInstruction: "do x"}

	var buf bytes.Buffer
	if err := Output(&buf, contract.FormatHuman, res); err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if !strings.Contains(buf.String(), "decision: "+contract.DecisionCatalogEmpty) {
		t.Errorf("human output = %q, want it to contain the decision", buf.String())
	}
}

func TestNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"", contract.FormatHuman, true},
		{"json", contract.FormatJSON, true},
		{"concise", contract.FormatConcise, true},
		{"yaml", contract.FormatHuman, false},
	}
	for _, tt := range tests {
		got, ok := Normalize(tt.in)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("Normalize(%q) = (%q, %t), want (%q, %t)", tt.in, got, ok, tt.want, tt.wantOK)
		}
	}
}
