package sidecar

import (
	"context"
	"log"
	"sync/atomic"

	"github.com/rokasklive/ozy/internal/search"
)

// SemanticAdapter wraps a Client and satisfies the search.Semantic
// interface, so the search engine can fuse the sidecar's semantic
// ranking with the lexical baseline (SPEC.md §10.3). It is the
// only public type a caller normally needs once the sidecar is
// running.
//
// Available is a sticky flag: it starts true and is flipped to false
// on the first Query error. The engine's lexical-only fallback is
// triggered when Available reports false, so we never want a single
// transient query failure to permanently disable semantic search
// mid-session. To reset, construct a new adapter (and a new Client).
type SemanticAdapter struct {
	client    *Client
	available atomic.Bool
	logger    Logger
}

// NewSemanticAdapter wraps an already-constructed Client. The sticky
// available flag starts true; it is flipped to false on the first
// Query error so the engine degrades to lexical-only for the
// remainder of the session. The Client's own Available() is also
// consulted on every call, so the adapter reports unavailable when
// the client has not been Health-checked or has errored.
func NewSemanticAdapter(c *Client) *SemanticAdapter {
	a := &SemanticAdapter{
		client: c,
		logger: noopLogger{},
	}
	a.available.Store(true)
	return a
}

// NewSemanticAdapterWithLogger is NewSemanticAdapter plus a Logger
// for adapter-level diagnostics. nil drops the messages.
func NewSemanticAdapterWithLogger(c *Client, logger Logger) *SemanticAdapter {
	a := NewSemanticAdapter(c)
	if logger != nil {
		a.logger = logger
	}
	return a
}

// Available reports whether the underlying Client reports itself
// as usable. It is a sticky flag flipped to false on the first
// Query error and is the signal the search engine uses to fall
// back to the lexical baseline.
func (a *SemanticAdapter) Available() bool {
	return a.available.Load() && a.client.Available()
}

// Query embeds the query string, runs the sidecar's nearest-neighbor
// search, and returns the ranked hits. The v1 sidecar only honours
// ServerID; ToolRefs is dropped with a debug log so a future
// revision can pick it up. Any error flips Available to false and
// returns (nil, nil) so the engine can degrade to lexical-only
// without surfacing a transport error to the user (SPEC.md §4.10).
func (a *SemanticAdapter) Query(ctx context.Context, query string, k int, filter search.Filter) ([]search.SemanticHit, error) {
	if !a.Available() {
		return nil, nil
	}
	if len(filter.ToolRefs) > 0 {
		a.logger.Log("sidecar.semantic: filter.ToolRefs dropped (not supported by v1 sidecar)")
	}
	res, err := a.client.Query(ctx, query, k, SearchFilter{ServerID: filter.ServerID})
	if err != nil {
		a.available.Store(false)
		a.logger.Log("sidecar.semantic: query failed: " + err.Error())
		return nil, nil
	}
	hits := make([]search.SemanticHit, len(res.Hits))
	for i, h := range res.Hits {
		hits[i] = search.SemanticHit{ToolRef: h.ToolRef, Score: h.Score}
	}
	return hits, nil
}

// Client returns the wrapped Client. Useful for callers that want
// to drive Upsert/Delete/Stats through the same process from
// outside the engine (e.g. embed-on-index).
func (a *SemanticAdapter) Client() *Client { return a.client }

// Compile-time assertion that SemanticAdapter satisfies
// search.Semantic. If the interface ever drifts, the build fails
// here rather than at the wiring site in the daemon.
var _ search.Semantic = (*SemanticAdapter)(nil)

// defaultStdLogger is a tiny shim used by callers that want
// adapter diagnostics routed through the standard library log
// package without depending on slog.
var defaultStdLogger = log.New(log.Writer(), "sidecar: ", log.LstdFlags)

// StdLogger adapts the std-lib log.Logger to the Logger interface
// so callers can pass it to NewSemanticAdapterWithLogger or as
// Options.Logger.
type StdLogger struct {
	L *log.Logger
}

// Log writes one line to the standard library logger.
func (s StdLogger) Log(line string) {
	if s.L == nil {
		defaultStdLogger.Print(line)
		return
	}
	s.L.Print(line)
}
