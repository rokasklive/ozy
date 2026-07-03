package daemon

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
)

// captureLogger returns a logger writing structured text into buf for assertions.
func captureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})), &buf
}

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

func TestStart_ReportsReady(t *testing.T) {
	t.Parallel()
	d := NewWithStore(&config.Loaded{Resolved: &config.Config{Version: 1}}, catalog.NewMemory())
	log, buf := captureLogger()
	d.SetLogger(log)

	// Start is synchronous: it returns once ready, with no blocking on cancel.
	d.Start(context.Background())

	if !strings.Contains(buf.String(), "ready") {
		t.Errorf("log = %q, want it to report readiness", buf.String())
	}
}

func TestStart_SemanticDisabledDoesNotProvision(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = false
	d := NewWithStore(cfg, catalog.NewMemory())
	if d.SemanticDegraded() {
		t.Error("SemanticDegraded() = true with semantic disabled, want false")
	}

	d.Start(context.Background())

	if d.sidecarProvisioned {
		t.Error("Start provisioned a sidecar with semantic disabled, want lexical-only")
	}
}

func TestStart_StaleCatalogTriggersReindex(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	ctx := context.Background()

	oldTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := store.SetLastIndexedAt(ctx, oldTime); err != nil {
		t.Fatalf("SetLastIndexedAt() error = %v", err)
	}

	cfgPath := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := &config.Loaded{Path: cfgPath, Resolved: &config.Config{Version: 1}}

	d := NewWithStore(cfg, store)
	log, buf := captureLogger()
	d.SetLogger(log)
	d.Start(ctx)

	out := buf.String()
	if !strings.Contains(out, "reindex") && !strings.Contains(out, "index complete") {
		t.Errorf("log does not mention reindexing: %q", out)
	}
	if !strings.Contains(out, "ready") {
		t.Error("daemon should report ready even after indexing")
	}
}

func TestStart_FreshCatalogSkipsIndexing(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	ctx := context.Background()

	cfgPath := filepath.Join(t.TempDir(), "ozy.jsonc")
	if err := os.WriteFile(cfgPath, []byte(`{"version":1}`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg := &config.Loaded{Path: cfgPath, Resolved: &config.Config{Version: 1}}

	// Set last indexed to now+1s so the config file is definitely older.
	if err := store.SetLastIndexedAt(ctx, time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetLastIndexedAt() error = %v", err)
	}

	d := NewWithStore(cfg, store)
	log, buf := captureLogger()
	d.SetLogger(log)
	d.Start(ctx)

	out := buf.String()
	if strings.Contains(out, "reindex") || strings.Contains(out, "index complete") {
		t.Errorf("fresh catalog should not reindex: %q", out)
	}
	if !strings.Contains(out, "ready") {
		t.Error("daemon should report ready")
	}
}

func TestStart_IndexingFailureStillReportsReady(t *testing.T) {
	t.Parallel()
	store := catalog.NewMemory()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	// No MCP servers configured — startup indexing finds no reachable servers.
	d := NewWithStore(cfg, store)
	log, buf := captureLogger()
	d.SetLogger(log)

	d.Start(context.Background())

	if !strings.Contains(buf.String(), "ready") {
		t.Errorf("daemon should report ready even on indexing failure: %q", buf.String())
	}
}

func TestStart_SemanticEnabledNoSidecar_Degrades(t *testing.T) {
	// Redirect the venv to an empty temp dir and cancel the context so
	// provisioning fails fast without touching the user's real sidecar.
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, catalog.NewMemory())
	if !d.SemanticDegraded() {
		t.Error("SemanticDegraded() = false, want true when semantic enabled but no sidecar provisioned")
	}
	log, buf := captureLogger()
	d.SetLogger(log)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.Start(ctx)

	if !d.SemanticDegraded() {
		t.Error("SemanticDegraded() = false after failed provisioning, want true")
	}
	if !strings.Contains(buf.String(), "lexical") {
		t.Errorf("daemon should surface lexical-only degradation: %q", buf.String())
	}
}

// TestStart_DegradeLogsCauseAndActionWithoutSecret covers the runtime-logging
// contract: a degradation record names a cause and a next action, and a
// configured secret never appears in any emitted record.
func TestStart_DegradeLogsCauseAndActionWithoutSecret(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	const secret = "super-secret-token-xyz"
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	cfg.Resolved.MCP = map[string]config.ServerConfig{
		"x": {URL: "http://127.0.0.1:1/mcp", Headers: map[string]string{"Authorization": secret}, Enabled: true},
	}
	d := NewWithStore(cfg, catalog.NewMemory())
	log, buf := captureLogger()
	d.SetLogger(log)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.Start(ctx)

	out := buf.String()
	if !strings.Contains(out, "reason=") {
		t.Errorf("degradation log missing a cause: %q", out)
	}
	if !strings.Contains(out, "action=") {
		t.Errorf("degradation log missing an action: %q", out)
	}
	if strings.Contains(out, secret) {
		t.Errorf("log leaked a configured secret value: %q", out)
	}
}

func TestShutdown_SafeWithoutSidecar(t *testing.T) {
	t.Parallel()
	d := NewWithStore(&config.Loaded{Resolved: &config.Config{Version: 1}}, catalog.NewMemory())
	// No sidecar was provisioned; Shutdown must be a safe, idempotent no-op.
	d.Shutdown()
	d.Shutdown()
}

// stubBroker is a do-nothing broker used to assert the swap mechanism.
type stubBroker struct{ id string }

func (stubBroker) FindTool(context.Context, string) (*contract.FindResult, error) { return nil, nil }
func (stubBroker) DescribeTool(context.Context, string) (*contract.DescribeResult, error) {
	return nil, nil
}
func (stubBroker) CallTool(context.Context, string, map[string]any) (*contract.CallResult, error) {
	return nil, nil
}
func (stubBroker) List(context.Context) (*contract.ListResult, error) { return nil, nil }

// TestSetBroker_SwapVisibleAndRaceFree covers the new atomic broker handle: a
// swap is immediately visible to Broker(), and concurrent swap+read is race-free
// (run under -race). This is the seam that lets a background provisioning pass
// upgrade a serving adapter from lexical to hybrid in place.
func TestSetBroker_SwapVisibleAndRaceFree(t *testing.T) {
	t.Parallel()
	d := NewWithStore(&config.Loaded{Resolved: &config.Config{Version: 1}}, catalog.NewMemory())
	sb := stubBroker{id: "swapped"}
	d.setBroker(sb)
	if got := d.Broker(); got != broker.Broker(sb) {
		t.Errorf("Broker() did not reflect the swap: got %#v", got)
	}

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			d.setBroker(stubBroker{id: "x"})
		}
		close(done)
	}()
	for i := 0; i < 1000; i++ {
		_ = d.Broker()
	}
	<-done
}

func TestIndex_SemanticDisabled_RunsLexicalWithoutProvisioning(t *testing.T) {
	t.Parallel()
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = false
	d := NewWithStore(cfg, catalog.NewMemory())

	summary := d.Index(context.Background())
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
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	cfg := &config.Loaded{Resolved: &config.Config{Version: 1}}
	cfg.Resolved.Search.Semantic.Enabled = true
	d := NewWithStore(cfg, catalog.NewMemory())

	// A cancelled context makes provisioning/health fail fast, so Index falls
	// back to the lexical path and reports degraded — without hanging on a real
	// model download.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	summary := d.Index(ctx)
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
