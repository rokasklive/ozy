// Package daemon hosts Ozy's runtime: it owns configuration, the catalog store,
// and the in-process broker that every adapter shares (SPEC.md §6.1). The
// runtime is hosted by the serving process (the MCP adapter) rather than a
// standalone command: Start provisions the sidecar and conditionally indexes,
// then returns once ready. The Broker seam is swappable under a lock so a
// background provisioning pass can upgrade lexical search to hybrid semantic in
// place, visible to an adapter that reads Broker() per request.
package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
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
	cfg   *config.Loaded
	store catalog.Store
	log   *slog.Logger

	// broker is read per request through Broker() and swapped by reWireBroker
	// when the sidecar becomes ready, so a background provisioning pass upgrades
	// a serving adapter from lexical to hybrid in place. mu guards the swap.
	mu     sync.RWMutex
	broker broker.Broker

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
// Semantic search is wired during Start() when the sidecar can be provisioned;
// New returns the lexical-only baseline that works without Python.
func New(cfg *config.Loaded) (*Daemon, error) {
	store, err := catalog.NewFile(catalog.DefaultPath())
	if err != nil {
		return nil, fmt.Errorf("open catalog store: %w", err)
	}
	return NewWithStore(cfg, store), nil
}

// NewWithStore constructs a daemon with an injected store. It keeps tests and
// focused in-memory callers from touching the user's durable catalog. The
// logger defaults to a discard logger; the serving process installs a real one
// via SetLogger.
func NewWithStore(cfg *config.Loaded, store catalog.Store) *Daemon {
	var resolved *config.Config
	if cfg != nil {
		resolved = cfg.Resolved
	}
	d := &Daemon{
		cfg:   cfg,
		store: store,
		log:   slog.New(slog.DiscardHandler),
	}
	d.broker = wireBroker(store, resolved, search.New(store, nil))
	return d
}

// SetLogger installs the structured logger used for lifecycle and degradation
// events. The serving process points this at the config-dir log file; tests may
// install a capturing handler. A nil logger is ignored.
func (d *Daemon) SetLogger(log *slog.Logger) {
	if log != nil {
		d.log = log
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

// Broker returns the shared broker used by all adapters. It is read under a
// lock so a background sidecar swap is visible and race-free.
func (d *Daemon) Broker() broker.Broker {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.broker
}

// setBroker atomically swaps the shared broker.
func (d *Daemon) setBroker(b broker.Broker) {
	d.mu.Lock()
	d.broker = b
	d.mu.Unlock()
}

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

// semanticState names the semantic search state for a readiness log line.
func (d *Daemon) semanticState() string {
	if d.cfg == nil || d.cfg.Resolved == nil || !d.cfg.Resolved.Search.Semantic.Enabled {
		return "disabled"
	}
	if d.semanticAvailable {
		return "available"
	}
	return "degraded"
}

// Start provisions the embedding sidecar (when semantic search is enabled) and
// runs a conditional index, then returns once the runtime is ready. It does NOT
// block: the serving process owns the lifetime and calls Shutdown on exit. A
// cold model download or provisioning failure degrades to the lexical baseline
// rather than failing Start, so a serving adapter can launch Start in the
// background and keep answering from lexical until semantic becomes ready.
func (d *Daemon) Start(ctx context.Context) {
	d.provisionSidecar(ctx)
	d.runStartupIndex(ctx)
	if d.semanticState() == "degraded" {
		reason := d.semanticReason
		if reason == "" {
			reason = "embedding sidecar unavailable"
		}
		d.log.Warn("runtime ready; semantic search unavailable, serving lexical baseline",
			"reason", reason,
			"action", "run `ozy doctor` to diagnose, then `ozy index` to rebuild")
		return
	}
	d.log.Info("runtime ready", "semantic", d.semanticState())
}

// provisionSidecar starts and verifies the embedding sidecar. It is idempotent:
// provisioning is attempted at most once per daemon lifetime, so the Start path
// and a standalone Index() call share one attempt. Verification is a two-step
// probe — a fast liveness Health then a generous readiness warm-up — so a short
// liveness deadline never aborts an in-progress cold model download.
func (d *Daemon) provisionSidecar(ctx context.Context) {
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
	d.log.Info("provisioning embedding sidecar", "reason", "semantic search enabled")
	emb := resolved.Embedding
	r, err := sidecar.Provision(ctx, sidecar.ProvisionOptions{
		Backend: emb.VectorBackend,
		Model:   emb.Model,
	})
	if err != nil {
		d.degradeSemantic(fmt.Sprintf("sidecar provisioning failed: %v", err))
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
		d.degradeSemantic(fmt.Sprintf("sidecar start failed: %v", err))
		return
	}
	// Liveness: confirm the process answers. A short deadline is fine — this
	// does NOT load the model.
	lctx, lcancel := context.WithTimeout(ctx, sidecarLivenessTimeout)
	hr := sc.Health(lctx)
	lcancel()
	if !hr.OK {
		d.degradeSemantic(fmt.Sprintf("sidecar health check failed: %v", hr.Err))
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
		d.degradeSemantic(fmt.Sprintf("embedding model warm-up failed: %v", rr.Err))
		_ = sc.Close()
		return
	}
	adapter := sidecar.NewSemanticAdapter(sc)
	d.sidecarClient = sc
	d.sidecarAdapter = adapter
	d.semanticAvailable = true
	d.semanticReason = ""
	d.reWireBroker()
	d.log.Info("embedding sidecar ready",
		"backend", rr.Backend, "model", rr.Model, "dim", rr.Dim, "vectors", rr.VectorCount)
}

// degradeSemantic records the specific reason semantic search is unavailable
// and logs it with a remediation action, so the degraded notice names a cause
// and a next step rather than a bare "lexical-only".
func (d *Daemon) degradeSemantic(reason string) {
	d.semanticAvailable = false
	d.semanticReason = reason
	d.log.Warn("semantic search degraded; serving lexical-only",
		"reason", reason,
		"action", "run `ozy doctor` to diagnose, then `ozy index` to rebuild")
}

const sidecarLivenessTimeout = 10 * time.Second

func (d *Daemon) reWireBroker() {
	var resolved *config.Config
	if d.cfg != nil {
		resolved = d.cfg.Resolved
	}
	d.setBroker(wireBroker(d.store, resolved, search.New(d.store, d.sidecarAdapter)))
}

// Shutdown stops the embedding sidecar if one is running. It is safe to call
// when no sidecar was provisioned. The serving adapter and the standalone
// `ozy index` / `ozy search` paths use it to avoid leaking the subprocess.
func (d *Daemon) Shutdown() { d.shutdownSidecar() }

func (d *Daemon) shutdownSidecar() {
	if d.sidecarClient == nil {
		return
	}
	_ = d.sidecarClient.Close()
	d.sidecarClient = nil
	d.sidecarAdapter = nil
	d.log.Info("embedding sidecar shut down")
}

// Index provisions the embedding sidecar (idempotently) and runs the indexer,
// attaching the embedding sink when semantic search is available. It is the one
// index path shared by Start and the standalone `ozy index` command, so both
// index identically — catalog plus vectors when semantic is enabled.
func (d *Daemon) Index(ctx context.Context) *index.Summary {
	d.provisionSidecar(ctx)
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

func (d *Daemon) runStartupIndex(ctx context.Context) {
	lastIdx, ok, err := d.store.LastIndexedAt(ctx)
	if err != nil {
		d.log.Warn("could not read last-indexed time", "error", err)
		return
	}

	stale := !ok
	if !stale && d.cfg != nil && d.cfg.Path != "" {
		fi, statErr := os.Stat(d.cfg.Path)
		if statErr == nil {
			if fi.ModTime().After(lastIdx) {
				stale = true
				d.log.Info("catalog is stale; reindexing on startup")
			}
		} else if !ok {
			stale = true
		}
	} else if stale {
		d.log.Info("catalog has never been indexed; running initial index")
	}

	if !stale {
		return
	}

	summary := d.Index(ctx)
	d.log.Info("startup index complete",
		"serversReached", summary.ServersReached,
		"toolsIndexed", summary.ToolsIndexed,
		"errors", len(summary.Errors),
		"ok", summary.OK)
	if summary.AgentInstruction != "" {
		d.log.Info("index guidance", "action", summary.AgentInstruction)
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

func (s *sink) Persist(ctx context.Context) error {
	// The sidecar persists its index on every upsert/delete batch. The
	// explicit persist call is triggered by the indexer after reconciliation.
	// The sidecar's write(path) is called internally; we don't need to
	// re-persist.
	return nil
}
