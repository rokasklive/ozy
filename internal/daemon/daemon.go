// Package daemon hosts Ozy's runtime: it owns configuration, the catalog store,
// and the in-process broker that every adapter shares (SPEC.md §6.1). The
// skeleton runs the broker in-process; the Broker interface is the seam a later
// change can use to split the daemon into a client/server transport.
package daemon

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/rokasklive/ozy/internal/broker"
	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/downstream"
	"github.com/rokasklive/ozy/internal/index"
	"github.com/rokasklive/ozy/internal/search"
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
	var resolved *config.Config
	if cfg != nil {
		resolved = cfg.Resolved
	}
	return &Daemon{
		cfg:    cfg,
		store:  store,
		broker: broker.NewLive(store, resolved, downstream.New(), search.New(store, nil)),
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
// daemon never requires semantic search or the embedding worker to start. Before
// readiness, it runs a conditional index when the catalog is stale relative to
// the configuration file's modification time.
func (d *Daemon) Run(ctx context.Context, status io.Writer) error {
	d.runStartupIndex(ctx, status)
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

func (d *Daemon) runStartupIndex(ctx context.Context, status io.Writer) {
	lastIdx, ok, err := d.store.LastIndexedAt(ctx)
	if err != nil {
		if status != nil {
			fmt.Fprintf(status, "notice: could not read last-indexed time: %v\n", err)
		}
		return
	}

	stale := !ok
	if !stale && d.cfg != nil && d.cfg.Path != "" {
		fi, statErr := os.Stat(d.cfg.Path)
		if statErr == nil {
			if fi.ModTime().After(lastIdx) {
				stale = true
				if status != nil {
					fmt.Fprintln(status, "catalog is stale — reindexing on startup")
				}
			}
		} else if !ok {
			// os.Stat failed: index only if never indexed (avoid thrashing).
			stale = true
		}
	} else if stale && status != nil {
		fmt.Fprintln(status, "catalog has never been indexed — running initial index")
	}

	if !stale {
		return
	}

	var resolved *config.Config
	if d.cfg != nil {
		resolved = d.cfg.Resolved
	}
	idx := index.New(d.store, nil)
	summary := idx.Run(ctx, resolved)

	if status != nil {
		fmt.Fprintf(status, "index complete: %d servers reached, %d tools indexed, %d errors",
			summary.ServersReached, summary.ToolsIndexed, len(summary.Errors))
		if !summary.OK {
			fmt.Fprintln(status, " (partial/failed)")
		} else {
			fmt.Fprintln(status)
		}
		if summary.AgentInstruction != "" {
			fmt.Fprintln(status, summary.AgentInstruction)
		}
	}
}
