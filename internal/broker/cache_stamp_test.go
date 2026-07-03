package broker

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

// countingCallBroker returns a fixed CallResult and counts invocations.
type countingCallBroker struct {
	calls atomic.Int64
}

func (b *countingCallBroker) FindTool(context.Context, string) (*contract.FindResult, error) {
	return &contract.FindResult{Decision: contract.DecisionNoGoodMatch}, nil
}

func (b *countingCallBroker) DescribeTool(context.Context, string) (*contract.DescribeResult, error) {
	return &contract.DescribeResult{}, nil
}

func (b *countingCallBroker) CallTool(context.Context, string, map[string]any) (*contract.CallResult, error) {
	b.calls.Add(1)
	return &contract.CallResult{OK: true, ToolRef: "srv.read", Result: "live payload"}, nil
}

func (b *countingCallBroker) List(context.Context) (*contract.ListResult, error) {
	return &contract.ListResult{}, nil
}

func stampStore(t *testing.T) catalog.Store {
	t.Helper()
	store := catalog.NewMemory()
	if err := store.PutTool(context.Background(), catalog.Tool{
		ToolRef: "srv.read", ServerID: "srv", DownstreamToolName: "read",
		ReadOnly: true, SchemaHash: "h1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetLastIndexedAt(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCache_HitCarriesAgeStampAndStoredEntryStaysClean(t *testing.T) {
	t.Parallel()
	inner := &countingCallBroker{}
	b := NewCaching(inner, stampStore(t), config.CacheConfig{Enabled: true, TTLSeconds: 300, MaxEntries: 8})
	ctx := context.Background()

	first, err := b.CallTool(ctx, "srv.read", map[string]any{"path": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if first.CachedAgeSeconds != nil {
		t.Fatalf("live invocation must carry no cache stamp, got %d", *first.CachedAgeSeconds)
	}

	second, err := b.CallTool(ctx, "srv.read", map[string]any{"path": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if second.CachedAgeSeconds == nil {
		t.Fatal("cache hit must carry CachedAgeSeconds")
	}
	if inner.calls.Load() != 1 {
		t.Fatalf("inner broker invoked %d times, want 1 (second call cached)", inner.calls.Load())
	}
	// The stamp goes on a copy: the first result and the stored entry are
	// untouched, and a later hit stamps its own age again.
	if first.CachedAgeSeconds != nil {
		t.Fatal("stamping a hit must not mutate previously returned results")
	}
	third, err := b.CallTool(ctx, "srv.read", map[string]any{"path": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if third.CachedAgeSeconds == nil {
		t.Fatal("every hit carries its own stamp")
	}
	if third == second {
		t.Fatal("hits must be distinct shallow copies, not the shared stored pointer")
	}
}
