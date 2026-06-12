package broker

import (
	"context"
	"errors"
	"testing"

	"github.com/rokask/ozy/internal/catalog"
	"github.com/rokask/ozy/internal/contract"
)

func newBroker(t *testing.T) (Broker, *catalog.Memory) {
	t.Helper()
	store := catalog.NewMemory()
	return NewSkeleton(store), store
}

func TestFindTool_EmptyCatalogReturnsCatalogEmpty(t *testing.T) {
	t.Parallel()
	b, _ := newBroker(t)

	res, err := b.FindTool(context.Background(), "search confluence")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionCatalogEmpty {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionCatalogEmpty)
	}
	if res.AgentInstruction == "" {
		t.Error("catalog_empty result must carry an agentInstruction (SPEC §9.1)")
	}
	if res.CatalogStats == nil || res.CatalogStats.IndexedTools != 0 {
		t.Errorf("CatalogStats = %+v, want IndexedTools 0", res.CatalogStats)
	}
}

func TestFindTool_NonEmptyCatalogReturnsNoGoodMatch(t *testing.T) {
	t.Parallel()
	b, store := newBroker(t)
	store.PutTool(catalog.Tool{ToolRef: "atlassian.search", ServerID: "atlassian", Freshness: catalog.FreshnessFresh})

	res, err := b.FindTool(context.Background(), "anything")
	if err != nil {
		t.Fatalf("FindTool() unexpected error = %v", err)
	}
	if res.Decision != contract.DecisionNoGoodMatch {
		t.Errorf("Decision = %q, want %q", res.Decision, contract.DecisionNoGoodMatch)
	}
	if res.AgentInstruction == "" {
		t.Error("result must remain instructional")
	}
}

func TestDescribeTool_UnknownReturnsToolNotFound(t *testing.T) {
	t.Parallel()
	b, _ := newBroker(t)

	_, err := b.DescribeTool(context.Background(), "atlassian.missing")
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("DescribeTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeToolNotFound {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeToolNotFound)
	}
	if ce.AgentInstruction == "" {
		t.Error("structured error must carry an agentInstruction (SPEC §9.3)")
	}
}

func TestCallTool_KnownToolReturnsNotImplemented(t *testing.T) {
	t.Parallel()
	b, store := newBroker(t)
	store.PutTool(catalog.Tool{ToolRef: "atlassian.search", ServerID: "atlassian"})

	_, err := b.CallTool(context.Background(), "atlassian.search", map[string]any{"q": "x"})
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeNotImplemented {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeNotImplemented)
	}
	if ce.Retryable {
		t.Error("NOT_IMPLEMENTED must not advertise retryable=true (avoids retry amplification, SPEC §9.3)")
	}
	if ce.AgentInstruction == "" {
		t.Error("structured failure must carry an agentInstruction")
	}
}

func TestCallTool_UnknownToolReturnsToolNotFound(t *testing.T) {
	t.Parallel()
	b, _ := newBroker(t)

	_, err := b.CallTool(context.Background(), "atlassian.missing", nil)
	var ce *contract.Error
	if !errors.As(err, &ce) {
		t.Fatalf("CallTool() error = %v, want *contract.Error", err)
	}
	if ce.Type != contract.ErrTypeToolNotFound {
		t.Errorf("error type = %q, want %q", ce.Type, contract.ErrTypeToolNotFound)
	}
}

func TestList_EmptyCatalogIsInstructional(t *testing.T) {
	t.Parallel()
	b, _ := newBroker(t)

	res, err := b.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(res.Tools) != 0 {
		t.Errorf("Tools len = %d, want 0", len(res.Tools))
	}
	if res.AgentInstruction == "" {
		t.Error("empty listing must be instructional")
	}
}
