package daemon

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rokask/ozy/internal/config"
)

func TestNew_WiresBrokerAndStore(t *testing.T) {
	t.Parallel()
	d := New(&config.Loaded{Resolved: &config.Config{Version: 1}})
	if d.Broker() == nil {
		t.Error("Broker() is nil")
	}
	if d.Store() == nil {
		t.Error("Store() is nil")
	}
}

func TestRun_ReportsReadyAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	d := New(&config.Loaded{Resolved: &config.Config{Version: 1}})

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
	d := New(cfg)
	if d.SemanticDegraded() {
		t.Error("SemanticDegraded() = true with semantic disabled, want false")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already done; Run should return promptly
	if err := d.Run(ctx, nil); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}
