package daemon

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
)

func TestNew_WiresBrokerAndStore(t *testing.T) {
	t.Parallel()
	d := NewWithStore(&config.Loaded{Resolved: &config.Config{Version: 1}}, catalog.NewMemory())
	if d.Broker() == nil {
		t.Error("Broker() is nil")
	}
	if d.Store() == nil {
		t.Error("Store() is nil")
	}
}

func TestNew_UsesPersistentCatalogStore(t *testing.T) {
	ctx := context.Background()
	t.Setenv("OZY_CATALOG", t.TempDir()+"/catalog.json")
	cfg := &config.Loaded{Resolved: &config.Config{}}

	d, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := d.Store().PutTool(ctx, catalog.Tool{
		ToolRef:   "atlassian.search",
		ServerID:  "atlassian",
		Freshness: catalog.FreshnessFresh,
	}); err != nil {
		t.Fatalf("PutTool() error = %v", err)
	}

	restarted, err := New(cfg)
	if err != nil {
		t.Fatalf("New(restart) error = %v", err)
	}
	if _, ok, err := restarted.Store().GetTool(ctx, "atlassian.search"); err != nil || !ok {
		t.Fatalf("restarted store GetTool() = ok %t, err %v; want persisted tool", ok, err)
	}
}

func TestRun_ReportsReadyAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	d := NewWithStore(&config.Loaded{Resolved: &config.Config{Version: 1}}, catalog.NewMemory())

	ctx, cancel := context.WithCancel(context.Background())
	var status bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, &status) }()

	// Give Run a moment to write the ready line, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after cancel")
	}
	if !strings.Contains(status.String(), "ready") {
		t.Errorf("status = %q, want it to report readiness", status.String())
	}
}

func TestRun_StartsWithSemanticDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = false
	d := NewWithStore(cfg, catalog.NewMemory())
	if d.SemanticDegraded() {
		t.Error("SemanticDegraded() = true with semantic disabled, want false")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done; Run should return promptly
	if err := d.Run(ctx, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

func TestRun_StaleCatalogTriggersReindex(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	ctx := context.Background()

	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.SetLastIndexedAt(ctx, oldTime); err != nil {
		t.Fatalf("SetLastIndexedAt() error = %v", err)
	}

	// Config file with mtime newer than oldTime.
	cfgPath := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := &config.Loaded{Path: cfgPath, Resolved: &config.Config{Version: 1}}

	d := NewWithStore(cfg, store)
	ctx2, cancel := context.WithCancel(ctx)
	var status bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx2, &status) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	out := status.String()
	if !strings.Contains(out, "reindexing") && !strings.Contains(out, "index complete") {
		t.Errorf("status does not mention reindexing: %q", out)
	}
	if !strings.Contains(out, "ready") {
		t.Error("daemon should report ready even after indexing")
	}
}

func TestRun_FreshCatalogSkipsIndexing(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	ctx := context.Background()

	cfgPath := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := &config.Loaded{Path: cfgPath, Resolved: &config.Config{Version: 1}}

	// Set last indexed to now+1s so config file is definitely older.
	if err := store.SetLastIndexedAt(ctx, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetLastIndexedAt() error = %v", err)
	}

	d := NewWithStore(cfg, store)
	ctx2, cancel := context.WithCancel(ctx)
	var status bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx2, &status) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	out := status.String()
	if strings.Contains(out, "reindexing") || strings.Contains(out, "index complete") {
		t.Errorf("fresh catalog should not reindex: %q", out)
	}
	if !strings.Contains(out, "ready") {
		t.Error("daemon should report ready")
	}
}

func TestRun_IndexingFailureStillReportsReady(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	// No MCP servers configured — startup indexing will find no reachable servers.
	d := NewWithStore(cfg, store)

	ctx, cancel := context.WithCancel(context.Background())
	var status bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, &status) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	out := status.String()
	if !strings.Contains(out, "ready") {
		t.Errorf("daemon should report ready even on indexing failure: %q", out)
	}
}
