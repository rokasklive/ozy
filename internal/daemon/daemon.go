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
	"time"

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
	cfg                *config.Loaded
	store              catalog.Store
	broker             broker.Broker
	sidecarClient      *sidecar.Client
	sidecarAdapter     *sidecar.SemanticAdapter
	semanticAvailable  bool
	semanticReason     string // specific cause when semantic is degraded
	sidecarProvisioned bool   // provisioning attempted once this lifetime
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
		broker: wireBroker(store, resolved, search.New(store, nil)),
	}
}

// wireBroker builds the shared broker, wrapping it with the result cache when the
// resolved config enables caching. A disabled cache is the unwrapped broker, so
// behavior is identical to not caching at all.
func wireBroker(store catalog.Store, resolved *config.Config, engine *search.Engine) broker.Broker {
	b := broker.NewLive(store, resolved, downstream.New(), engine)
	if resolved != nil && resolved.Cache.Enabled {
		b = broker.NewCaching(b, store, resolved.Cache)
	}
	return b
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
			reason := d.semanticReason
			if reason == "" {
				reason = "embedding sidecar unavailable"
			}
			fmt.Fprintf(status, "notice: semantic search requested but unavailable (%s); using lexical baseline — run `ozy doctor` to diagnose, then `ozy index` to rebuild\n", reason)
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

// provisionSidecar starts and verifies the embedding sidecar. It is idempotent:
// provisioning is attempted at most once per daemon lifetime, so the daemon
// startup path and a standalone Index() call share one attempt. Verification is
// a two-step probe — a fast liveness Health then a generous readiness warm-up —
// so a short liveness deadline never aborts an in-progress cold model download.
func (d *Daemon) provisionSidecar(ctx context.Context, status io.Writer) {
	if d.sidecarProvisioned {
		return
	}
	if d.cfg == nil || d.cfg.Resolved == nil {
		return
	}
	resolved := d.cfg.Resolved
	if !resolved.Search.Semantic.Enabled {
		return
	}
	d.sidecarProvisioned = true
	if status != nil {
		fmt.Fprintln(status, "semantic search enabled — provisioning embedding sidecar")
	}
	emb := resolved.Embedding
	r, err := sidecar.Provision(ctx, sidecar.ProvisionOptions{
		Backend: emb.VectorBackend,
		Model:   emb.Model,
	})
	if err != nil {
		d.degradeSemantic(status, fmt.Sprintf("sidecar provisioning failed: %v", err))
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
		d.degradeSemantic(status, fmt.Sprintf("sidecar start failed: %v", err))
		return
	}
	// Liveness: confirm the process answers. A short deadline is fine — this
	// does NOT load the model.
	lctx, lcancel := context.WithTimeout(ctx, sidecarLivenessTimeout)
	hr := sc.Health(lctx)
	lcancel()
	if !hr.OK {
		d.degradeSemantic(status, fmt.Sprintf("sidecar health check failed: %v", hr.Err))
		_ = sc.Close()
		return
	}
	// Readiness warm-up: load the model (may trigger a cold download) and run a
	// probe query, under its OWN generous deadline so the liveness timeout above
	// never aborts a download. "Available" means this succeeds.
	wctx, wcancel := context.WithTimeout(ctx, sidecar.DefaultProvisionTimeout)
	rr := sc.Ready(wctx)
	wcancel()
	if !rr.OK {
		d.degradeSemantic(status, fmt.Sprintf("embedding model warm-up failed: %v", rr.Err))
		_ = sc.Close()
		return
	}
	adapter := sidecar.NewSemanticAdapter(sc)
	d.sidecarClient = sc
	d.sidecarAdapter = adapter
	d.semanticAvailable = true
	d.semanticReason = ""
	d.reWireBroker()
	if status != nil {
		fmt.Fprintf(status, "sidecar ready: backend=%s model=%s dim=%d vectors=%d\n",
			rr.Backend, rr.Model, rr.Dim, rr.VectorCount)
	}
}

// degradeSemantic records the specific reason semantic search is unavailable
// and surfaces it, so the degraded notice names a cause and a next step rather
// than a bare "lexical-only".
func (d *Daemon) degradeSemantic(status io.Writer, reason string) {
	d.semanticAvailable = false
	d.semanticReason = reason
	if status != nil {
		fmt.Fprintf(status, "notice: %s — running lexical-only\n", reason)
	}
}

const sidecarLivenessTimeout = 10 * time.Second

func (d *Daemon) reWireBroker() {
	var resolved *config.Config
	if d.cfg != nil {
		resolved = d.cfg.Resolved
	}
	d.broker = wireBroker(d.store, resolved, search.New(d.store, d.sidecarAdapter))
}

// Shutdown stops the embedding sidecar if one is running. It is safe to call
// when no sidecar was provisioned. The standalone `ozy index` path uses it to
// avoid leaking the subprocess after a one-shot index.
func (d *Daemon) Shutdown() { d.shutdownSidecar() }

func (d *Daemon) shutdownSidecar() {
	if d.sidecarClient == nil {
		return
	}
	_ = d.sidecarClient.Close()
	d.sidecarClient = nil
	d.sidecarAdapter = nil
}

// Index provisions the embedding sidecar (idempotently) and runs the indexer,
// attaching the embedding sink when semantic search is available. It is the one
// index path shared by daemon startup and the standalone `ozy index` command,
// so both index identically — catalog plus vectors when semantic is enabled.
func (d *Daemon) Index(ctx context.Context, status io.Writer) *index.Summary {
	d.provisionSidecar(ctx, status)
	var resolved *config.Config
	if d.cfg != nil {
		resolved = d.cfg.Resolved
	}
	idx := index.New(d.store, nil)
	if d.semanticAvailable && d.sidecarClient != nil {
		idx = index.New(d.store, nil, index.WithSink(newSink(d.sidecarClient)))
	}
	return idx.Run(ctx, resolved)
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

	summary := d.Index(ctx, status)

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

func (s *sink) Upsert(ctx context.Context, items []index.EmbedItem) (int, error) {
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
	res, err := s.c.Upsert(ctx, sideItems)
	if err != nil {
		return 0, err
	}
	return res.Upserted, nil
}

func (s *sink) Delete(ctx context.Context, refs []string) error {
	_, err := s.c.Delete(ctx, refs)
	return err
}

// VectorCount reports how many vectors the sidecar currently stores, so the
// index summary can show the queryable vector count and the loud-fail guard can
// detect an indexed-but-not-embedded run.
func (s *sink) VectorCount(ctx context.Context) (int, error) {
	st, err := s.c.Stats(ctx)
	if err != nil {
		return 0, err
	}
	return st.VectorCount, nil
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
