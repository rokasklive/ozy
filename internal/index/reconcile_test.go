package index

import (
	"context"
	"errors"
	"testing"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

func seedTool(t *testing.T, store catalog.Store, serverID, name string) {
	t.Helper()
	err := store.PutTool(context.Background(), catalog.Tool{
		ToolRef:            serverID + "." + name,
		ServerID:           serverID,
		DownstreamToolName: name,
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
		Freshness:          catalog.FreshnessFresh,
		LastIndexedAt:      time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func cfgWith(serverIDs ...string) *config.Config {
	mcp := make(map[string]config.ServerConfig, len(serverIDs))
	for _, id := range serverIDs {
		mcp[id] = config.ServerConfig{Type: "local", Command: []string{"fake"}, Enabled: true}
	}
	return &config.Config{MCP: mcp}
}

// deletionSink is available and records deleted refs.
type deletionSink struct {
	deleted [][]string
}

func (d *deletionSink) Available() bool { return true }
func (d *deletionSink) Upsert(_ context.Context, items []EmbedItem) (int, error) {
	return len(items), nil
}
func (d *deletionSink) Delete(_ context.Context, refs []string) error {
	d.deleted = append(d.deleted, refs)
	return nil
}
func (d *deletionSink) VectorCount(context.Context) (int, error) { return 1, nil }
func (d *deletionSink) Persist(context.Context) error            { return nil }

func TestReconcile_VanishedToolIsDeletedFromCatalogAndSink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	seedTool(t, store, "srv", "kept")
	seedTool(t, store, "srv", "vanished")

	sink := &deletionSink{}
	indexer := New(store, fakeConnector{results: []downstream.Result{{
		ServerID: "srv",
		Session:  fakeSession{tools: []*mcpsdk.Tool{{Name: "kept"}}},
	}}}, WithSink(sink))

	summary := indexer.Run(ctx, cfgWith("srv"))
	if !summary.OK {
		t.Fatalf("summary not OK: %+v", summary)
	}
	if _, ok, _ := store.GetTool(ctx, "srv.vanished"); ok {
		t.Fatal("srv.vanished should be deleted from the catalog")
	}
	if _, ok, _ := store.GetTool(ctx, "srv.kept"); !ok {
		t.Fatal("srv.kept should survive")
	}
	if len(sink.deleted) != 1 || len(sink.deleted[0]) != 1 || sink.deleted[0][0] != "srv.vanished" {
		t.Fatalf("sink deletions = %+v, want [[srv.vanished]]", sink.deleted)
	}
}

func TestReconcile_RemovedServerToolsAreDeleted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	seedTool(t, store, "gone", "tool")
	seedTool(t, store, "srv", "kept")

	indexer := New(store, fakeConnector{results: []downstream.Result{{
		ServerID: "srv",
		Session:  fakeSession{tools: []*mcpsdk.Tool{{Name: "kept"}}},
	}}})

	indexer.Run(ctx, cfgWith("srv")) // "gone" is absent from config
	if _, ok, _ := store.GetTool(ctx, "gone.tool"); ok {
		t.Fatal("tools of a config-removed server should be deleted")
	}
	if _, ok, _ := store.GetTool(ctx, "srv.kept"); !ok {
		t.Fatal("srv.kept should survive")
	}
}

func TestReconcile_UnreachableServerDegradesButKeepsTools(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	seedTool(t, store, "flaky", "tool")

	indexer := New(store, fakeConnector{results: []downstream.Result{
		{ServerID: "flaky", Error: &contract.Error{Type: contract.ErrTypeDownstreamServerOffline, ServerID: "flaky", Message: "down"}},
		{ServerID: "srv", Session: fakeSession{tools: []*mcpsdk.Tool{{Name: "kept"}}}},
	}})

	indexer.Run(ctx, cfgWith("flaky", "srv"))
	tool, ok, _ := store.GetTool(ctx, "flaky.tool")
	if !ok {
		t.Fatal("a flake must never delete cataloged tools")
	}
	if tool.Freshness != catalog.FreshnessStale || tool.CallableNow || tool.ServerStatus != catalog.ServerOffline {
		t.Fatalf("flaky tool should be degraded (stale, not callable, offline), got %+v", tool)
	}
}

func TestReconcile_FailedListingDeletesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := catalog.NewMemory()
	seedTool(t, store, "srv", "tool")

	indexer := New(store, fakeConnector{results: []downstream.Result{{
		ServerID: "srv",
		Session:  fakeSession{err: errors.New("list exploded")},
	}}})

	indexer.Run(ctx, cfgWith("srv"))
	tool, ok, _ := store.GetTool(ctx, "srv.tool")
	if !ok {
		t.Fatal("a failed tools/list must not delete cataloged tools")
	}
	if tool.Freshness != catalog.FreshnessStale || tool.CallableNow {
		t.Fatalf("tool of a server with a failed listing should be degraded, got %+v", tool)
	}
}
