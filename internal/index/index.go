// Package index discovers tools from connected downstream servers and persists
// normalized catalog entries.
package index

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/rokasklive/ozy/internal/catalog"
	"github.com/rokasklive/ozy/internal/config"
	"github.com/rokasklive/ozy/internal/contract"
	"github.com/rokasklive/ozy/internal/downstream"
)

// Connector is the downstream dependency used by the indexer.
type Connector interface {
	ConnectAll(ctx context.Context, cfg *config.Config) []downstream.Result
}

// EmbedItem is one tool to embed. ToolRef identifies the catalog row;
// ContentHash lets the sink skip re-embedding unchanged tools. Text is the
// concatenated indexed-field text per SPEC.md §10.2. ServerID and Tags are
// the facet source for filtered semantic queries.
type EmbedItem struct {
	ToolRef     string
	Text        string
	ContentHash string
	ServerID    string
	Tags        []string
}

// EmbeddingSink is the optional, opt-in seam the indexer uses to push
// embeddings to the sidecar after the catalog is persisted. Available() == false
// disables the sink (lexical-only mode). Upsert returns how many tools were
// actually embedded this run; VectorCount reports the total queryable vectors
// after the run so the summary can be honest about vector storage. Deletions
// are catalog-driven: the indexer passes the toolRefs it reconciled out of the
// catalog, so vector storage tracks the catalog without a sink-side list op.
type EmbeddingSink interface {
	Available() bool
	Upsert(ctx context.Context, items []EmbedItem) (int, error)
	Delete(ctx context.Context, toolRefs []string) error
	VectorCount(ctx context.Context) (int, error)
	Persist(ctx context.Context) error
}

// noopSink is the default sink. It is always unavailable.
type noopSink struct{}

func (noopSink) Available() bool                                  { return false }
func (noopSink) Upsert(context.Context, []EmbedItem) (int, error) { return 0, nil }
func (noopSink) Delete(context.Context, []string) error           { return nil }
func (noopSink) VectorCount(context.Context) (int, error)         { return 0, nil }
func (noopSink) Persist(context.Context) error                    { return nil }

// Summary is the structured `ozy index` result.
type Summary struct {
	OK               bool             `json:"ok"`
	ServersReached   int              `json:"serversReached"`
	ServersSkipped   int              `json:"serversSkipped"`
	ServersFailed    int              `json:"serversFailed"`
	ToolsIndexed     int              `json:"toolsIndexed"`
	EmbeddedCount    int              `json:"embeddedCount,omitempty"`
	VectorCount      int              `json:"vectorCount,omitempty"`
	Errors           []contract.Error `json:"errors,omitempty"`
	AgentInstruction string           `json:"agentInstruction,omitempty"`
}

// Render produces the human/concise form of an index summary.
func (s *Summary) Render(format string) string {
	if format == contract.FormatConcise {
		out := fmt.Sprintf("index servers=%d tools=%d errors=%d", s.ServersReached, s.ToolsIndexed, len(s.Errors))
		if s.EmbeddedCount > 0 || s.VectorCount > 0 {
			out += fmt.Sprintf(" embedded=%d vectors=%d", s.EmbeddedCount, s.VectorCount)
		}
		return out
	}
	var b strings.Builder
	fmt.Fprintf(&b, "indexed %d tools from %d reachable servers", s.ToolsIndexed, s.ServersReached)
	if s.EmbeddedCount > 0 || s.VectorCount > 0 {
		fmt.Fprintf(&b, "\nembedded %d tools; %d vectors queryable", s.EmbeddedCount, s.VectorCount)
	}
	if s.ServersSkipped > 0 {
		fmt.Fprintf(&b, "\nskipped servers: %d", s.ServersSkipped)
	}
	if s.ServersFailed > 0 {
		fmt.Fprintf(&b, "\nfailed servers: %d", s.ServersFailed)
		for _, e := range s.Errors {
			fmt.Fprintf(&b, "\n  - %s: %s", e.ServerID, e.Message)
		}
	}
	if s.AgentInstruction != "" {
		fmt.Fprintf(&b, "\n→ %s", s.AgentInstruction)
	}
	return b.String()
}

// Indexer coordinates connect -> tools/list -> normalize -> persist.
type Indexer struct {
	store     catalog.Store
	connector Connector
	sink      EmbeddingSink
	now       func() time.Time
}

// Option customizes an Indexer.
type Option func(*Indexer)

// WithClock injects time for deterministic tests.
func WithClock(now func() time.Time) Option {
	return func(i *Indexer) {
		if now != nil {
			i.now = now
		}
	}
}

// WithSink attaches an EmbeddingSink for the index run. nil or
// EmbeddingSink.Available() == false disables the embedding path; the
// indexer stays lexical-only and still persists the catalog.
func WithSink(sink EmbeddingSink) Option {
	return func(i *Indexer) {
		if sink != nil {
			i.sink = sink
		}
	}
}

// New constructs an Indexer.
func New(store catalog.Store, connector Connector, opts ...Option) *Indexer {
	if connector == nil {
		connector = downstream.New()
	}
	i := &Indexer{
		store:     store,
		connector: connector,
		sink:      noopSink{},
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(i)
	}
	return i
}

// Run connects to configured downstream servers, discovers tools, persists
// normalized catalog entries, and (when an embedding sink is attached and
// available) pushes the embedded text to the sink. After the catalog is
// persisted, deletes are reconciled and the index is asked to persist.
func (i *Indexer) Run(ctx context.Context, cfg *config.Config) *Summary {
	summary := &Summary{OK: true}
	var embedded []EmbedItem
	// Reconciliation inputs: what this run actually learned per server. Only a
	// successful tools/list is authority to delete; a failed or skipped server
	// merely degrades its cataloged tools.
	listed := make(map[string]map[string]bool)
	degraded := make(map[string]catalog.ServerStatus)
	for _, result := range i.connector.ConnectAll(ctx, cfg) {
		if result.Skipped {
			summary.ServersSkipped++
			degraded[result.ServerID] = catalog.ServerUnknown
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerUnknown)
			continue
		}
		if result.Error != nil {
			summary.ServersFailed++
			summary.Errors = append(summary.Errors, *result.Error)
			degraded[result.ServerID] = catalog.ServerOffline
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerOffline)
			continue
		}
		if result.Session == nil {
			err := contract.Error{
				Type:             contract.ErrTypeDownstreamServerOffline,
				ServerID:         result.ServerID,
				Retryable:        true,
				Message:          "downstream connector returned no session",
				AgentInstruction: "Retry after checking the server connection.",
			}
			summary.ServersFailed++
			summary.Errors = append(summary.Errors, err)
			degraded[result.ServerID] = catalog.ServerOffline
			i.recordServer(ctx, summary, result.ServerID, catalog.ServerOffline)
			continue
		}

		server := config.ServerConfig{}
		if cfg != nil {
			server = cfg.MCP[result.ServerID]
		}
		summary.ServersReached++
		i.recordServer(ctx, summary, result.ServerID, catalog.ServerOnline)
		indexed, refs, ok := i.indexSession(ctx, summary, result.ServerID, server, result.Session)
		embedded = append(embedded, indexed...)
		if ok {
			listed[result.ServerID] = refs
		} else {
			// Connected but tools/list failed: no deletion authority; degrade.
			degraded[result.ServerID] = catalog.ServerOffline
		}
		_ = result.Session.Close()
	}
	deleted := i.reconcile(ctx, summary, cfg, listed, degraded)
	switch {
	case summary.ServersReached == 0:
		summary.OK = false
		summary.AgentInstruction = "No configured downstream server was reachable. Review the per-server errors, run `ozy doctor`, then retry `ozy index` after repairing configuration or connectivity."
	case summary.ToolsIndexed == 0 && len(summary.Errors) > 0:
		summary.OK = false
		summary.AgentInstruction = "No downstream tools were indexed. Review the per-server errors, run `ozy doctor`, then retry `ozy index` after repairing configuration or connectivity."
	case len(summary.Errors) > 0:
		summary.AgentInstruction = "Some servers failed, but reachable servers were indexed. Use `ozy list` or `ozy describe` for indexed tools and repair failed servers separately."
	}
	if summary.ServersReached > 0 {
		if err := i.store.SetLastIndexedAt(ctx, i.now()); err != nil {
			summary.OK = false
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeConfigError,
				Retryable:        true,
				Message:          fmt.Sprintf("failed to persist last-indexed time: %v", err),
				AgentInstruction: "Check catalog storage permissions, then retry indexing.",
			})
		}
	}
	if i.sink != nil && i.sink.Available() {
		i.flushEmbeddings(ctx, summary, embedded, deleted)
		// Loud-fail guard: semantic is enabled and the sidecar is available, yet
		// the catalog holds more tools than the vector store has queryable
		// vectors. That is the silent "indexed-but-not-embedded" (or partially
		// embedded) failure; report it rather than claiming success on an
		// incomplete vector store. Firing on undercount, not only on exactly
		// zero, catches the stale-partial-embed case.
		if summary.ToolsIndexed > 0 && summary.VectorCount < summary.ToolsIndexed {
			summary.OK = false
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeSemanticSearchUnavailable,
				Retryable:        true,
				Message:          fmt.Sprintf("semantic search is enabled and the sidecar is available, but only %d of %d indexed tools are queryable as vectors", summary.VectorCount, summary.ToolsIndexed),
				AgentInstruction: "Run `ozy doctor` to check the embedding sidecar, then re-run `ozy index`. Lexical search still serves from the catalog.",
			})
			summary.AgentInstruction = "Semantic search is enabled but some indexed tools are not embedded. Run `ozy doctor`, then re-run `ozy index`."
		}
	}
	return summary
}

// flushEmbeddings batches an upsert to the sink for this run's tools, pushes
// the catalog reconciliation's deletions so vector storage tracks the catalog,
// and asks the sink to persist its index. Errors degrade the catalog run:
// the catalog is still updated; only the embedding pipeline is skipped.
func (i *Indexer) flushEmbeddings(ctx context.Context, summary *Summary, embedded []EmbedItem, deleted []string) {
	upserted, err := i.sink.Upsert(ctx, embedded)
	if err != nil {
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeSemanticSearchUnavailable,
			Retryable:        true,
			Message:          fmt.Sprintf("embedding sink upsert failed: %v", err),
			AgentInstruction: "Retry `ozy index` later; lexical search still serves from the catalog.",
		})
		return
	}
	summary.EmbeddedCount = upserted
	if len(deleted) > 0 {
		if err := i.sink.Delete(ctx, deleted); err != nil {
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeSemanticSearchUnavailable,
				Retryable:        true,
				Message:          fmt.Sprintf("embedding sink delete failed: %v", err),
				AgentInstruction: "Retry `ozy index` later; lexical search still serves from the catalog.",
			})
			return
		}
	}
	// Record the queryable vector count after this run's upserts and deletes, so
	// the summary is honest and the loud-fail guard can detect an empty store.
	if vc, vcErr := i.sink.VectorCount(ctx); vcErr == nil {
		summary.VectorCount = vc
	}
	if err := i.sink.Persist(ctx); err != nil {
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeSemanticSearchUnavailable,
			Retryable:        true,
			Message:          fmt.Sprintf("embedding sink persist failed: %v", err),
			AgentInstruction: "Retry `ozy index` later; the embedding index is reloaded from the catalog on next start.",
		})
	}
}

// reconcile trues the catalog up against what this run learned. Tools whose
// server listed successfully but no longer serves them, and tools whose server
// is absent from the configuration, are deleted (returned for the embedding
// sink). Tools whose configured server was unreachable or disabled are kept but
// degraded — stale, not callable, with the observed server status — so a flake
// never erases the catalog while the responses stop asserting fresh/callable.
func (i *Indexer) reconcile(ctx context.Context, summary *Summary, cfg *config.Config, listed map[string]map[string]bool, degraded map[string]catalog.ServerStatus) []string {
	tools, err := i.store.Tools(ctx)
	if err != nil {
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        true,
			Message:          fmt.Sprintf("failed to read catalog for reconciliation: %v", err),
			AgentInstruction: "Check catalog storage permissions, then retry indexing.",
		})
		return nil
	}

	var toDelete []string
	for _, t := range tools {
		// A successful listing this run is the strongest authority — it decides
		// keep-or-delete for its server's tools before any config check, so a
		// run can never delete what it just discovered.
		if refs, ok := listed[t.ServerID]; ok {
			if !refs[t.ToolRef] {
				toDelete = append(toDelete, t.ToolRef)
			}
			continue
		}
		if _, inConfig := configServer(cfg, t.ServerID); !inConfig {
			toDelete = append(toDelete, t.ToolRef)
			continue
		}
		if status, ok := degraded[t.ServerID]; ok {
			if t.Freshness == catalog.FreshnessStale && !t.CallableNow && t.ServerStatus == status {
				continue // already degraded; skip a pointless persist
			}
			t.Freshness = catalog.FreshnessStale
			t.CallableNow = false
			t.ServerStatus = status
			if err := i.store.PutTool(ctx, t); err != nil {
				summary.OK = false
				summary.Errors = append(summary.Errors, contract.Error{
					Type:             contract.ErrTypeConfigError,
					ServerID:         t.ServerID,
					Retryable:        true,
					Message:          fmt.Sprintf("failed to degrade tool %q: %v", t.ToolRef, err),
					AgentInstruction: "Check catalog storage permissions, then retry indexing.",
				})
			}
		}
	}

	if len(toDelete) == 0 {
		return nil
	}
	sort.Strings(toDelete)
	if err := i.store.DeleteTools(ctx, toDelete); err != nil {
		summary.OK = false
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeConfigError,
			Retryable:        true,
			Message:          fmt.Sprintf("failed to delete %d vanished tools: %v", len(toDelete), err),
			AgentInstruction: "Check catalog storage permissions, then retry indexing.",
		})
		return nil
	}
	return toDelete
}

// configServer looks up a server in the resolved config, tolerating nil.
func configServer(cfg *config.Config, serverID string) (config.ServerConfig, bool) {
	if cfg == nil {
		return config.ServerConfig{}, false
	}
	s, ok := cfg.MCP[serverID]
	return s, ok
}

func (i *Indexer) recordServer(ctx context.Context, summary *Summary, serverID string, status catalog.ServerStatus) {
	if err := i.store.PutServer(ctx, catalog.Server{ID: serverID, Status: status}); err != nil {
		summary.OK = false
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeConfigError,
			ServerID:         serverID,
			Retryable:        true,
			Message:          fmt.Sprintf("failed to persist server status: %v", err),
			AgentInstruction: "Check catalog storage permissions, then retry indexing.",
		})
	}
}

// indexSession lists and persists one server's tools. It returns the embed
// items, the set of toolRefs the server currently serves, and whether the
// listing succeeded — only a successful listing grants deletion authority to
// the reconciler. The discovered set includes tools that failed to normalize
// or persist, because the server does still serve them.
func (i *Indexer) indexSession(ctx context.Context, summary *Summary, serverID string, server config.ServerConfig, session downstream.Session) ([]EmbedItem, map[string]bool, bool) {
	listCtx, cancel := context.WithTimeout(ctx, server.DiscoveryTimeout())
	defer cancel()
	list, err := session.ListTools(listCtx, nil)
	if err != nil {
		summary.ServersFailed++
		summary.Errors = append(summary.Errors, contract.Error{
			Type:             contract.ErrTypeDownstreamCallFailed,
			ServerID:         serverID,
			Retryable:        true,
			Message:          fmt.Sprintf("tools/list failed: %v", scrub(err.Error(), server)),
			AgentInstruction: "Check the downstream server health and retry indexing.",
		})
		return nil, nil, false
	}
	var embedded []EmbedItem
	discovered := make(map[string]bool, len(list.Tools))
	for _, tool := range list.Tools {
		discovered[serverID+"."+tool.Name] = true
		catalogTool, err := normalizeTool(serverID, tool, i.now())
		if err != nil {
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeDownstreamCallFailed,
				ServerID:         serverID,
				Retryable:        false,
				Message:          fmt.Sprintf("tool %q has invalid schema: %v", tool.Name, err),
				AgentInstruction: "Report the invalid downstream tool schema to the server owner.",
			})
			continue
		}
		if err := i.store.PutTool(ctx, catalogTool); err != nil {
			summary.OK = false
			summary.Errors = append(summary.Errors, contract.Error{
				Type:             contract.ErrTypeConfigError,
				ServerID:         serverID,
				Retryable:        true,
				Message:          fmt.Sprintf("failed to persist tool %q: %v", catalogTool.ToolRef, err),
				AgentInstruction: "Check catalog storage permissions, then retry indexing.",
			})
			continue
		}
		summary.ToolsIndexed++
		embedded = append(embedded, buildEmbedItem(catalogTool))
	}
	return embedded, discovered, true
}

// buildEmbedItem renders the §10.2 indexed text for embedding. It is the same
// field set the lexical scorer reads, so the two signals describe the same
// tool. The content hash is the schema hash; the sink can use it to skip
// re-embedding when only metadata changed.
func buildEmbedItem(t catalog.Tool) EmbedItem {
	parts := []string{
		"server: " + t.ServerID,
		"tool: " + t.ToolRef,
		"downstream: " + t.DownstreamToolName,
	}
	if t.Title != "" {
		parts = append(parts, "title: "+t.Title)
	}
	if t.Description != "" {
		parts = append(parts, "description: "+t.Description)
	}
	for _, alias := range t.CapabilityText {
		if alias != "" {
			parts = append(parts, "alias: "+alias)
		}
	}
	return EmbedItem{
		ToolRef:     t.ToolRef,
		Text:        strings.Join(parts, "\n"),
		ContentHash: t.SchemaHash,
		ServerID:    t.ServerID,
		Tags:        nil,
	}
}

func scrub(msg string, server config.ServerConfig) string {
	for _, secret := range secretValues(server) {
		if secret == "" || strings.Contains(secret, "{env:") {
			continue
		}
		msg = strings.ReplaceAll(msg, secret, "****")
	}
	return msg
}

func secretValues(server config.ServerConfig) []string {
	values := make([]string, 0, len(server.Headers)+len(server.Environment))
	for _, v := range server.Headers {
		values = append(values, v)
	}
	for _, v := range server.Environment {
		values = append(values, v)
	}
	return values
}

func normalizeTool(serverID string, tool *mcpsdk.Tool, now time.Time) (catalog.Tool, error) {
	inputSchema, schemaHash, err := normalizeSchema(tool.InputSchema)
	if err != nil {
		return catalog.Tool{}, err
	}
	return catalog.Tool{
		ToolRef:            serverID + "." + tool.Name,
		ServerID:           serverID,
		DownstreamToolName: tool.Name,
		Title:              tool.Title,
		Description:        tool.Description,
		InputSchema:        inputSchema,
		CapabilityText:     []string{tool.Title, tool.Description},
		ServerStatus:       catalog.ServerOnline,
		CallableNow:        true,
		ReadOnly:           tool.Annotations != nil && tool.Annotations.ReadOnlyHint,
		LastIndexedAt:      now,
		SchemaHash:         schemaHash,
		Freshness:          catalog.FreshnessFresh,
	}, nil
}

func normalizeSchema(schema any) (map[string]any, string, error) {
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	data, err := json.Marshal(schema)
	if err != nil {
		return nil, "", err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, "", err
	}
	canonical, err := json.Marshal(out)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(canonical)
	return out, hex.EncodeToString(sum[:]), nil
}
