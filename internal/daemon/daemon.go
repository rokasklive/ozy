// Package daemon hosts Ozy's runtime: it owns configuration, the catalog store,
// and the in-process broker that every adapter shares (SPEC.md §6.1). The
// skeleton runs the broker in-process; the Broker interface is the seam a later
// change can use to split the daemon into a client/server transport.
package daemon

import (
	"context"
	"fmt"
	"io"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
)

// Daemon holds the runtime wiring shared by the CLI and MCP adapter.
type Daemon struct {
	cfg    *config.Loaded
	store  catalog.Store
	broker broker.Broker
}

// New constructs the runtime from already-loaded configuration. It initializes
// the persistent catalog store and the broker. Configuration validation happens
// in config.Load, so an invalid config never reaches this point.
func New(cfg *config.Loaded) (*Daemon, error) {
	store, err := catalog.NewFile(catalog.DefaultPath())
	if err != nil {
		return nil, fmt.Errorf("open catalog store: %w", err)
	}
	return NewWithStore(cfg, store), nil
}

// NewWithStore constructs a daemon with an injected store. It keeps tests and
// focused in-memory callers from touching the user's durable catalog.
func NewWithStore(cfg *config.Loaded, store catalog.Store) *Daemon {
	return &Daemon{
		cfg:    cfg,
		store:  store,
		broker: broker.NewSkeleton(store),
	}
}

// Broker returns the shared broker used by all adapters.
func (d *Daemon) Broker() broker.Broker { return d.broker }

// Store returns the catalog store.
func (d *Daemon) Store() catalog.Store { return d.store }

// Config returns the loaded configuration.
func (d *Daemon) Config() *config.Loaded { return d.cfg }

// SemanticDegraded reports whether semantic search was requested but is treated
// as unavailable in this build, so callers can surface the graceful fallback to
// lexical search (SPEC.md §4.10, §10.1).
func (d *Daemon) SemanticDegraded() bool {
	return d.cfg != nil && d.cfg.Resolved != nil && d.cfg.Resolved.Search.Semantic.Enabled
}

// Run reports readiness, then blocks until ctx is cancelled (for example by an
// interrupt signal), at which point it returns nil after a clean shutdown. The
// daemon never requires semantic search or the embedding worker to start.
func (d *Daemon) Run(ctx context.Context, status io.Writer) error {
	if status != nil {
		fmt.Fprintln(status, "ozy daemon ready")
		if d.SemanticDegraded() {
			fmt.Fprintln(status, "notice: semantic search requested but not available in this build; using lexical baseline")
		}
	}
	<-ctx.Done()
	if status != nil {
		fmt.Fprintln(status, "ozy daemon stopped")
	}
	return nil
}
