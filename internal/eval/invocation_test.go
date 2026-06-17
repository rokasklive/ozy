package eval

import (
	"context"
	"testing"

	"github.com/rokasklive/ozy/evals"
	"github.com/rokasklive/ozy/internal/contract"
)

func TestRunInvocationEmbedded(t *testing.T) {
	corpus, err := Load(evals.Data())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	report, err := RunInvocation(context.Background(), corpus)
	if err != nil {
		t.Fatalf("RunInvocation error = %v", err)
	}
	m := report.Overall
	if m.N != len(corpus.Invocation) {
		t.Errorf("N = %d, want %d", m.N, len(corpus.Invocation))
	}
	for _, c := range []struct {
		name string
		val  float64
	}{
		{"validArgumentRate", m.ValidArgumentRate},
		{"firstCallSuccess", m.FirstCallSuccess},
		{"repairSuccess", m.RepairSuccess},
		{"schemaErrorRate", m.SchemaErrorRate},
		{"offlineHandledRate", m.OfflineHandledRate},
		{"errorClarity", m.ErrorClarity},
	} {
		if c.val != 1.0 {
			t.Errorf("%s = %.3f, want 1.0 over the committed corpus", c.name, c.val)
		}
	}
}

// TestInvocationRepairLoop drives the repair loop end-to-end: invalid arguments
// produce a structured ARGUMENT_VALIDATION_FAILED, and the corrected call then
// succeeds through the real broker.
func TestInvocationRepairLoop(t *testing.T) {
	corpus := mustLoad(t)
	bk, err := newCorpusBroker(corpus, nil)
	if err != nil {
		t.Fatalf("newCorpusBroker error = %v", err)
	}
	tool, ok := corpus.toolByRef("jira.create_issue")
	if !ok {
		t.Fatal("jira.create_issue must be in the corpus")
	}
	ctx := context.Background()

	_, cerr := callWithModeling(ctx, bk, tool, nil, map[string]any{"projectKey": "OPS"})
	if cerr == nil || cerr.Type != contract.ErrTypeArgumentValidationFailed {
		t.Fatalf("invalid call error = %+v, want ARGUMENT_VALIDATION_FAILED", cerr)
	}
	if cerr.Retryable {
		t.Error("an argument-validation error must not be retryable (no amplification)")
	}
	if !errorIsClear(cerr) {
		t.Error("argument-validation error should be structurally clear")
	}

	res, cerr := callWithModeling(ctx, bk, tool, nil, map[string]any{"projectKey": "OPS", "summary": "stale pager"})
	if cerr != nil {
		t.Fatalf("corrected call error = %+v, want success", cerr)
	}
	if res == nil || !res.OK {
		t.Error("corrected call should succeed")
	}
}

// TestInvocationOfflineIsNonAmplifying verifies an offline server surfaces a
// structured DOWNSTREAM_SERVER_OFFLINE with a non-retry-amplifying disposition
// through the real broker.CallTool seam.
func TestInvocationOfflineIsNonAmplifying(t *testing.T) {
	corpus := mustLoad(t)
	bk, err := newCorpusBroker(corpus, map[string]bool{"slack": true})
	if err != nil {
		t.Fatalf("newCorpusBroker error = %v", err)
	}
	tool, _ := corpus.toolByRef("slack.post_message")
	_, cerr := callWithModeling(context.Background(), bk, tool, nil, map[string]any{"channel": "#ops", "text": "hi"})
	if cerr == nil || cerr.Type != contract.ErrTypeDownstreamServerOffline {
		t.Fatalf("offline call error = %+v, want DOWNSTREAM_SERVER_OFFLINE", cerr)
	}
	if !isNonAmplifying(cerr) {
		t.Errorf("offline error must be non-amplifying, got retryable=%t instruction=%q", cerr.Retryable, cerr.AgentInstruction)
	}
}

// TestInvocationSchemaDriftDetected verifies a drifted live schema yields
// TOOL_SCHEMA_CHANGED for arguments still valid against the cataloged schema.
func TestInvocationSchemaDriftDetected(t *testing.T) {
	corpus := mustLoad(t)
	bk, err := newCorpusBroker(corpus, nil)
	if err != nil {
		t.Fatalf("newCorpusBroker error = %v", err)
	}
	tool, _ := corpus.toolByRef("confluence.search_pages")
	live := map[string]any{
		"type":       "object",
		"required":   []any{"cql"},
		"properties": map[string]any{"cql": map[string]any{"type": "string"}},
	}
	_, cerr := callWithModeling(context.Background(), bk, tool, live, map[string]any{"query": "runbook"})
	if cerr == nil || cerr.Type != contract.ErrTypeToolSchemaChanged {
		t.Fatalf("drift call error = %+v, want TOOL_SCHEMA_CHANGED", cerr)
	}
	if cerr.Retryable {
		t.Error("schema-drift error must not be retryable with the stale arguments")
	}
}

func mustLoad(t *testing.T) *Corpus {
	t.Helper()
	corpus, err := Load(evals.Data())
	if err != nil {
		t.Fatalf("Load error = %v", err)
	}
	return corpus
}
