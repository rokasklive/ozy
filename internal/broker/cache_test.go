package broker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

// countingBroker is a stub Broker that counts delegated calls so tests can prove
// cache hits avoid the underlying work.
type countingBroker struct {
	find, describe, call, list int
	callErr                    error
}

func (b *countingBroker) FindTool(_ context.Context, q string) (*contract.FindResult, error) {
	b.find++
	return &contract.FindResult{Query: q}, nil
}

func (b *countingBroker) DescribeTool(_ context.Context, _ string) (*contract.DescribeResult, error) {
	b.describe++
	return &contract.DescribeResult{}, nil
}

func (b *countingBroker) CallTool(_ context.Context, ref string, _ map[string]any) (*contract.CallResult, error) {
	b.call++
	if b.callErr != nil {
		return nil, b.callErr
	}
	return &contract.CallResult{OK: true, ToolRef: ref}, nil
}

func (b *countingBroker) List(context.Context) (*contract.ListResult, error) {
	b.list++
	return &contract.ListResult{}, nil
}

func newTestCache(t *testing.T) (*countingBroker, *catalog.Memory, Broker) {
	t.Helper()
	ctx := context.Background()
	store := catalog.NewMemory()
	mustPut(t, store, catalog.Tool{ToolRef: "srv.read", SchemaHash: "h1", ReadOnly: true})
	mustPut(t, store, catalog.Tool{ToolRef: "srv.write", SchemaHash: "h2", ReadOnly: false})
	if err := store.SetLastIndexedAt(ctx, time.Unix(1000, 0)); err != nil {
		t.Fatalf("SetLastIndexedAt: %v", err)
	}
	inner := &countingBroker{}
	cfg := config.CacheConfig{Enabled: true, TTLSeconds: 60, MaxEntries: 100}
	return inner, store, NewCaching(inner, store, cfg)
}

func mustPut(t *testing.T, store *catalog.Memory, tool catalog.Tool) {
	t.Helper()
	if err := store.PutTool(context.Background(), tool); err != nil {
		t.Fatalf("PutTool: %v", err)
	}
}

func TestCachingBroker_FindAndDescribeCached(t *testing.T) {
	inner, _, c := newTestCache(t)
	ctx := context.Background()

	_, _ = c.FindTool(ctx, "q")
	_, _ = c.FindTool(ctx, "q")
	if inner.find != 1 {
		t.Errorf("find delegated %d times, want 1 (second is a cache hit)", inner.find)
	}

	_, _ = c.DescribeTool(ctx, "srv.read")
	_, _ = c.DescribeTool(ctx, "srv.read")
	if inner.describe != 1 {
		t.Errorf("describe delegated %d times, want 1", inner.describe)
	}
}

func TestCachingBroker_ReadOnlyCallCachedDistinctArgs(t *testing.T) {
	inner, _, c := newTestCache(t)
	ctx := context.Background()

	_, _ = c.CallTool(ctx, "srv.read", map[string]any{"a": 1})
	_, _ = c.CallTool(ctx, "srv.read", map[string]any{"a": 1})
	if inner.call != 1 {
		t.Fatalf("read-only call delegated %d times, want 1", inner.call)
	}

	_, _ = c.CallTool(ctx, "srv.read", map[string]any{"a": 2})
	if inner.call != 2 {
		t.Errorf("distinct args delegated %d times, want 2", inner.call)
	}
}

func TestCachingBroker_WriteToolNeverCached(t *testing.T) {
	inner, _, c := newTestCache(t)
	ctx := context.Background()

	_, _ = c.CallTool(ctx, "srv.write", map[string]any{"x": 1})
	_, _ = c.CallTool(ctx, "srv.write", map[string]any{"x": 1})
	if inner.call != 2 {
		t.Errorf("write tool delegated %d times, want 2 (never cached)", inner.call)
	}
}

func TestCachingBroker_UnknownToolInvokedLive(t *testing.T) {
	inner, _, c := newTestCache(t)
	ctx := context.Background()

	_, _ = c.CallTool(ctx, "srv.missing", nil)
	_, _ = c.CallTool(ctx, "srv.missing", nil)
	if inner.call != 2 {
		t.Errorf("unknown tool delegated %d times, want 2 (no positive read-only evidence)", inner.call)
	}
}

func TestCachingBroker_FailuresNotCached(t *testing.T) {
	inner, _, c := newTestCache(t)
	inner.callErr = errors.New("boom")
	ctx := context.Background()

	_, _ = c.CallTool(ctx, "srv.read", nil)
	_, _ = c.CallTool(ctx, "srv.read", nil)
	if inner.call != 2 {
		t.Errorf("failing call delegated %d times, want 2 (failures not cached)", inner.call)
	}
}

func TestCachingBroker_ReindexInvalidatesFind(t *testing.T) {
	inner, store, c := newTestCache(t)
	ctx := context.Background()

	_, _ = c.FindTool(ctx, "q")
	if err := store.SetLastIndexedAt(ctx, time.Unix(2000, 0)); err != nil {
		t.Fatalf("SetLastIndexedAt: %v", err)
	}
	_, _ = c.FindTool(ctx, "q")
	if inner.find != 2 {
		t.Errorf("find delegated %d times, want 2 (re-index advances generation)", inner.find)
	}
}

func TestCachingBroker_TTLExpiry(t *testing.T) {
	store := catalog.NewMemory()
	if err := store.SetLastIndexedAt(context.Background(), time.Unix(1000, 0)); err != nil {
		t.Fatalf("SetLastIndexedAt: %v", err)
	}
	inner := &countingBroker{}
	// Negative TTL means every stored entry is already expired on read.
	c := &cachingBroker{inner: inner, store: store, ttl: -time.Second, max: 100, entries: map[string]cacheEntry{}}
	ctx := context.Background()

	_, _ = c.FindTool(ctx, "q")
	_, _ = c.FindTool(ctx, "q")
	if inner.find != 2 {
		t.Errorf("find delegated %d times, want 2 (expired entries are misses)", inner.find)
	}
}

func TestCachingBroker_ListPassthrough(t *testing.T) {
	inner, _, c := newTestCache(t)
	ctx := context.Background()
	_, _ = c.List(ctx)
	_, _ = c.List(ctx)
	if inner.list != 2 {
		t.Errorf("list delegated %d times, want 2 (List is not cached)", inner.list)
	}
}
