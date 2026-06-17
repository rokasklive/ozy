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

func TestRun_SemanticEnabledButNoSidecar_StillReadyDegraded(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, store)
	if !d.SemanticDegraded() {
		t.Error("SemanticDegraded() = false, want true when semantic enabled but no sidecar provisioned")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var status bytes.Buffer
	if err := d.Run(ctx, &status); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	out := status.String()
	if !strings.Contains(out, "ready") {
		t.Errorf("daemon should report ready even with no sidecar: %q", out)
	}
	if !strings.Contains(out, "lexical baseline") {
		t.Errorf("daemon should surface lexical-only degradation: %q", out)
	}
}

func TestRun_SidecarShutDownWithDaemon(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, store)

	ctx, cancel := context.WithCancel(context.Background())
	var status bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, &status) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not return after cancel; shutdown may have hung")
	}
	out := status.String()
	if !strings.Contains(out, "stopped") {
		t.Errorf("daemon should report stopped: %q", out)
	}
	if d.SemanticDegraded() {
		t.Log("semantic degraded after shutdown — expected (sidecar never provisioned)")
	}
}

func TestIndex_SemanticDisabled_RunsLexicalWithoutProvisioning(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = false
	d := NewWithStore(cfg, catalog.NewMemory())

	summary := d.Index(context.Background(), nil)
	if summary == nil {
		t.Fatal("Index() returned nil summary")
	}
	if d.sidecarProvisioned {
		t.Error("Index() provisioned a sidecar with semantic disabled, want lexical-only")
	}
	// No sink wired, so nothing is embedded. (Sink wiring on the available path
	// is covered by internal/index sink tests and the end-to-end check.)
	if summary.EmbeddedCount != 0 || summary.VectorCount != 0 {
		t.Errorf("embedded=%d vectors=%d, want 0/0 with semantic disabled", summary.EmbeddedCount, summary.VectorCount)
	}
}

func TestIndex_SemanticEnabledButProvisioningFails_Degrades(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, catalog.NewMemory())

	// A cancelled context makes provisioning/health fail fast, so Index falls
	// back to the lexical path and reports degraded — without hanging on a real
	// model download.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary := d.Index(ctx, nil)
	if summary == nil {
		t.Fatal("Index() returned nil summary")
	}
	if !d.SemanticDegraded() {
		t.Error("SemanticDegraded() = false, want true when provisioning fails")
	}
	if d.semanticReason == "" {
		t.Error("semanticReason is empty, want a specific degraded cause")
	}
}

func TestSemanticDegraded_FalseWhenDisabled(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = false
	d := NewWithStore(cfg, catalog.NewMemory())
	if d.SemanticDegraded() {
		t.Error("SemanticDegraded() = true when semantic disabled")
	}
}

func TestSemanticDegraded_TrueWhenEnabledButUnavailable(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, catalog.NewMemory())
	if !d.SemanticDegraded() {
		t.Error("SemanticDegraded() = false when semantic enabled but no sidecar")
	}
}
