package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFile_EmptyStoreQueriesAreClean(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := NewFile(filepath.Join(t.TempDir(), "catalog.json"))
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	tools, err := store.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("Tools() len = %d, want 0", len(tools))
	}

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats != (Stats{}) {
		t.Errorf("Stats() = %+v, want zero counts", stats)
	}
}

func TestFile_WritesAndReloadsCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.json")
	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	indexedAt := time.Date(2026, 6, 12, 10, 30, 0, 0, time.UTC)
	if err := store.PutServer(ctx, Server{ID: "atlassian", Status: ServerOnline}); err != nil {
		t.Fatalf("PutServer() error = %v", err)
	}
	if err := store.PutTool(ctx, Tool{
		ToolRef:            "atlassian.confluence_search",
		ServerID:           "atlassian",
		DownstreamToolName: "confluence_search",
		Title:              "Confluence Search",
		Description:        "Search Confluence",
		InputSchema:        map[string]any{"type": "object"},
		ServerStatus:       ServerOnline,
		CallableNow:        true,
		LastIndexedAt:      indexedAt,
		SchemaHash:         "abc123",
		Freshness:          FreshnessFresh,
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	reloaded, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile(reload) error = %v", err)
	}
	tool, ok, err := reloaded.GetTool(ctx, "atlassian.confluence_search")
	if err != nil {
		t.Fatalf("GetTool() error = %v", err)
	}
	if !ok {
		t.Fatal("GetTool() ok = false, want true")
	}
	if tool.SchemaHash != "abc123" || !tool.LastIndexedAt.Equal(indexedAt) {
		t.Errorf("reloaded tool = %+v, want persisted metadata", tool)
	}
}

func TestFile_OverwriteKeepsValidJSON(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.json")
	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}

	for _, ref := range []string{"server.one", "server.two"} {
		if err := store.PutTool(ctx, Tool{ToolRef: ref, ServerID: "server", Freshness: FreshnessFresh}); err != nil {
			t.Fatalf("PutTool(%s) error = %v", ref, err)
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("catalog file is not valid JSON after overwrite: %v\n%s", err, data)
	}
	reloaded, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile(reload) error = %v", err)
	}
	tools, err := reloaded.Tools(ctx)
	if err != nil {
		t.Fatalf("Tools() error = %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("Tools() len = %d, want 2", len(tools))
	}
}

func TestFile_PersistedCatalogContainsNoConfigSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "catalog.json")
	store, err := NewFile(path)
	if err != nil {
		t.Fatalf("NewFile() error = %v", err)
	}
	if err := store.PutServer(ctx, Server{ID: "atlassian", Status: ServerOnline}); err != nil {
		t.Fatalf("PutServer() error = %v", err)
	}
	if err := store.PutTool(ctx, Tool{
		ToolRef:            "atlassian.confluence_search",
		ServerID:           "atlassian",
		DownstreamToolName: "confluence_search",
		InputSchema:        map[string]any{"type": "object"},
		Freshness:          FreshnessFresh,
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}
	if strings.Contains(string(data), "supersecretvalue") || strings.Contains(string(data), "Authorization") {
		t.Fatalf("catalog persisted config secret material:\n%s", data)
	}
}
