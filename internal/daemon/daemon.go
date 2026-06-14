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
	"github.com/rokasklive/ozy/internal/sidecar"
)

// Daemon holds the runtime wiring shared by the CLI and MCP adapter.
type Daemon struct {
	cfg               *config.Loaded
	store             catalog.Store
	broker            broker.Broker
	sidecarClient     *sidecar.Client
	sidecarAdapter    *sidecar.SemanticAdapter
	semanticAvailable bool
}

// New constructs the runtime from already-loaded configuration. It initializes
// the persistent catalog store and the broker. Configuration validation happens
// in config.Load, so an invalid config never reaches this point.
//
// Semantic search is wired during Run() when the sidecar can be provisioned;
// New returns the lexical-only baseline that works without Python.
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

// SemanticDegraded reports whether semantic search is configured but the
// sidecar is not actually serving (provisioning failed, health check failed,
// or the sidecar crashed), so callers can surface the graceful fallback to
// lexical search (SPEC.md §4.10, §10.1). Returns false when semantic is
// disabled or when it is enabled AND the sidecar is healthy.
func (d *Daemon) SemanticDegraded() bool {
	if d.cfg == nil || d.cfg.Resolved == nil {
		return false
	}
	if !d.cfg.Resolved.Search.Semantic.Enabled {
		return false
	}
	return !d.semanticAvailable
}

// Run reports readiness, then blocks until ctx is cancelled (for example by an
// interrupt signal), at which point it returns nil after a clean shutdown. The
// daemon never requires semantic search or the embedding worker to start. Before
// readiness, it provisions and health-checks the sidecar when semantic is
// enabled, then runs a conditional index when the catalog is stale.
func (d *Daemon) Run(ctx context.Context, status io.Writer) error {
	d.provisionSidecar(ctx, status)
	d.runStartupIndex(ctx, status)
	if status != nil {
		fmt.Fprintln(status, "ozy daemon ready")
		if d.SemanticDegraded() {
			fmt.Fprintln(status, "notice: semantic search requested but unavailable; using lexical baseline")
		}
	}
	<-ctx.Done()
	if status != nil {
		fmt.Fprintln(status, "ozy daemon stopping")
	}
	d.shutdownSidecar()
	if status != nil {
		fmt.Fprintln(status, "ozy daemon stopped")
	}
	return nil
}

func (d *Daemon) provisionSidecar(ctx context.Context, status io.Writer) {
	if d.cfg == nil || d.cfg.Resolved == nil {
		return
	}
	resolved := d.cfg.Resolved
	if !resolved.Search.Semantic.Enabled {
		return
	}
	if status != nil {
		fmt.Fprintln(status, "semantic search enabled — provisioning embedding sidecar")
	}
	emb := resolved.Embedding
	r, err := sidecar.Provision(ctx, sidecar.ProvisionOptions{
		Backend: emb.VectorBackend,
		Model:   emb.Model,
	})
	if err != nil {
		d.semanticAvailable = false
		if status != nil {
			fmt.Fprintf(status, "notice: sidecar provisioning failed: %v — running lexical-only\n", err)
		}
		return
	}
	sidecarOpts := sidecar.Options{
		DataDir: r.VenvDir,
		Backend: emb.VectorBackend,
		Model:   emb.Model,
		ProcessOptions: sidecar.ProcessOptions{
			PythonPath: r.PythonPath,
			SourceDir:  r.SourceDir,
			DataDir:    r.VenvDir,
			Backend:    emb.VectorBackend,
			Model:      emb.Model,
		},
	}
	sc, err := sidecar.NewClient(sidecarOpts)
	if err != nil {
		d.semanticAvailable = false
		if status != nil {
			fmt.Fprintf(status, "notice: sidecar start failed: %v — running lexical-only\n", err)
		}
		return
	}
	hctx, hcancel := context.WithTimeout(ctx, sidecarSidecarHealthTimeout)
	defer hcancel()
	hr := sc.Health(hctx)
	if !hr.OK {
		d.semanticAvailable = false
		if status != nil {
			fmt.Fprintf(status, "notice: sidecar health check failed: %v — running lexical-only\n", hr.Err)
		}
		_ = sc.Close()
		return
	}
	adapter := sidecar.NewSemanticAdapter(sc)
	d.sidecarClient = sc
	d.sidecarAdapter = adapter
	d.semanticAvailable = true
	d.reWireBroker()
	if status != nil {
		fmt.Fprintf(status, "sidecar ready: backend=%s model=%s dim=%d vectors=%d\n",
			hr.Backend, hr.Model, hr.Dim, hr.VectorCount)
	}
}

const sidecarSidecarHealthTimeout = 10 * 1e9 // nanoseconds (10 s)

func (d *Daemon) reWireBroker() {
	var resolved *config.Config
	if d.cfg != nil {
		resolved = d.cfg.Resolved
	}
	d.broker = broker.NewLive(d.store, resolved, downstream.New(),
		search.New(d.store, d.sidecarAdapter))
}

func (d *Daemon) shutdownSidecar() {
	if d.sidecarClient == nil {
		return
	}
	_ = d.sidecarClient.Close()
	d.sidecarClient = nil
	d.sidecarAdapter = nil
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
	if d.semanticAvailable && d.sidecarClient != nil {
		idx = index.New(d.store, nil, index.WithSink(newSink(d.sidecarClient)))
	}
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

// sink adapts a *sidecar.Client to the index.EmbeddingSink interface.
type sink struct {
	c *sidecar.Client
}

func newSink(c *sidecar.Client) *sink { return &sink{c: c} }

func (s *sink) Available() bool { return s.c.Available() }

func (s *sink) Upsert(ctx context.Context, items []index.EmbedItem) error {
	sideItems := make([]sidecar.UpsertItem, len(items))
	for i, item := range items {
		sideItems[i] = sidecar.UpsertItem{
			ToolRef:     item.ToolRef,
			Text:        item.Text,
			ContentHash: item.ContentHash,
			ServerID:    item.ServerID,
			Tags:        item.Tags,
		}
	}
	_, err := s.c.Upsert(ctx, sideItems)
	return err
}

func (s *sink) Delete(ctx context.Context, refs []string) error {
	_, err := s.c.Delete(ctx, refs)
	return err
}

func (s *sink) List(ctx context.Context) ([]string, error) {
	// The sidecar's Stats returns toolCount but not individual toolRefs.
	// The index reconciliation needs a list; the v1 sidecar does not expose
	// a list op (the protocol only has health/upsert/delete/query/stats).
	// Until the sidecar adds a list op, skip reconciliation.
	return nil, nil
}

func (s *sink) Persist(ctx context.Context) error {
	// The sidecar persists its index on every upsert/delete batch. The
	// explicit persist call is triggered by the indexer after reconciliation.
	// The sidecar's write(path) is called internally; we don't need to
	// re-persist.
	return nil
}
