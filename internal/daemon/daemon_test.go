package daemon

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rokask/ozy/internal/catalog"
	"github.com/rokask/ozy/internal/config"
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
