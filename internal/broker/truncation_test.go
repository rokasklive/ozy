package broker

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rokasklive/ozy/internal/config"
)

func budgetCfg(maxBytes int) *config.Config {
	return &config.Config{Budgets: config.BudgetsConfig{CallTool: config.CallToolBudget{MaxResultBytes: maxBytes}}}
}

func TestApplyCallBudget_ArrayDropsWholeElementsAndStaysValidJSON(t *testing.T) {
	t.Parallel()
	arr := make([]any, 0, 50)
	for i := 0; i < 50; i++ {
		arr = append(arr, map[string]any{"idx": i, "pad": strings.Repeat("p", 20)})
	}
	bounded, notice := applyCallBudget(budgetCfg(300), arr)

	kept, ok := bounded.([]any)
	if !ok || len(kept) == 0 || len(kept) >= 50 {
		t.Fatalf("bounded = %T len %d, want a shorter []any prefix", bounded, len(kept))
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		t.Fatalf("truncated array must stay marshalable: %v", err)
	}
	if len(encoded) > 300 {
		t.Fatalf("truncated array encodes to %d bytes, want <= 300", len(encoded))
	}
	var roundTrip []any
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("truncated array is not valid JSON: %v", err)
	}
	if !strings.Contains(notice, "of 50 items") || !strings.Contains(notice, "narrow the call") {
		t.Fatalf("notice should report N of M and recovery, got %q", notice)
	}
}

func TestApplyCallBudget_TextCutsAtLineBoundary(t *testing.T) {
	t.Parallel()
	text := strings.Repeat("line of output here\n", 100)
	bounded, notice := applyCallBudget(budgetCfg(256), text)

	cut, ok := bounded.(string)
	if !ok {
		t.Fatalf("bounded = %T, want string", bounded)
	}
	if len(cut) > 256 {
		t.Fatalf("cut = %d bytes, want <= 256", len(cut))
	}
	if strings.HasSuffix(cut, "here") == false && strings.HasSuffix(cut, "\n") {
		t.Fatalf("cut should end at a complete line, got tail %q", cut[len(cut)-24:])
	}
	if !strings.Contains(notice, "narrow the call") {
		t.Fatalf("notice missing recovery guidance: %q", notice)
	}
	if strings.Contains(notice, "not valid JSON") {
		t.Fatalf("plain-text truncation should not warn about JSON validity: %q", notice)
	}
}

func TestApplyCallBudget_NonArrayJSONWarnsUnparseable(t *testing.T) {
	t.Parallel()
	obj := map[string]any{"big": strings.Repeat("word and word ", 100)}
	bounded, notice := applyCallBudget(budgetCfg(128), obj)

	if _, ok := bounded.(string); !ok {
		t.Fatalf("bounded = %T, want string (cut JSON rendering)", bounded)
	}
	if !strings.Contains(notice, "not valid JSON") {
		t.Fatalf("object truncation must warn the payload is unparseable, got %q", notice)
	}
}

func TestApplyCallBudget_WithinBudgetUntouched(t *testing.T) {
	t.Parallel()
	bounded, notice := applyCallBudget(budgetCfg(1024), "small")
	if bounded != "small" || notice != "" {
		t.Fatalf("within-budget result must pass through, got %v / %q", bounded, notice)
	}
}
