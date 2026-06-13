package catalog

import (
	"context"
	"testing"
	"time"
)

func TestMemory_EmptyStoreQueriesAreClean(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()

	servers, err := m.Servers(ctx)
	if err != nil {
		t.Fatalf("Servers() error = %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("Servers() len = %d, want 0", len(servers))
	}

	tools, err := m.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("Tools() len = %d, want 0", len(tools))
	}

	stats, err := m.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.IndexedTools != 0 || stats.ConfiguredServers != 0 {
		t.Errorf("Stats() = %+v, want zero counts", stats)
	}

	if _, ok, err := m.GetTool(ctx, "atlassian.confluence_search"); err != nil || ok {
		t.Errorf("GetTool() on empty store = (ok=%t, err=%v), want (false, nil)", ok, err)
	}
}

func TestMemory_StatsCountFreshness(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()
	if err := m.PutServer(ctx, Server{ID: "atlassian", Status: ServerOnline}); err != nil {
		t.Fatalf("PutServer() error = %v", err)
	}
	if err := m.PutTool(ctx, Tool{ToolRef: "atlassian.a", ServerID: "atlassian", Freshness: FreshnessFresh}); err != nil {
		t.Fatalf("PutTool(a) error = %v", err)
	}
	if err := m.PutTool(ctx, Tool{ToolRef: "atlassian.b", ServerID: "atlassian", Freshness: FreshnessStale}); err != nil {
		t.Fatalf("PutTool(b) error = %v", err)
	}

	stats, err := m.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	want := Stats{ConfiguredServers: 1, IndexedTools: 2, FreshTools: 1, StaleTools: 1}
	if stats != want {
		t.Errorf("Stats() = %+v, want %+v", stats, want)
	}
}

func TestMemory_LastIndexedAt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()

	ts, ok, err := m.LastIndexedAt(ctx)
	if err != nil {
		t.Fatalf("LastIndexedAt() error = %v", err)
	}
	if ok {
		t.Errorf("LastIndexedAt() ok = true for fresh store, want false")
	}
	if !ts.IsZero() {
		t.Errorf("LastIndexedAt() = %v for fresh store, want zero time", ts)
	}

	indexedAt := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	if err := m.SetLastIndexedAt(ctx, indexedAt); err != nil {
		t.Fatalf("SetLastIndexedAt() error = %v", err)
	}

	ts, ok, err = m.LastIndexedAt(ctx)
	if err != nil {
		t.Fatalf("LastIndexedAt() error = %v", err)
	}
	if !ok {
		t.Error("LastIndexedAt() ok = false after SetLastIndexedAt, want true")
	}
	if !ts.Equal(indexedAt) {
		t.Errorf("LastIndexedAt() = %v, want %v", ts, indexedAt)
	}
}
