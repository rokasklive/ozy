// Package catalog defines Ozy's persistent capability catalog seam.
//
// The catalog owns servers, tools, schemas, freshness, and runtime status
// (SPEC.md §6.1, §8). This package provides the Store interface plus an
// in-memory placeholder implementation; a durable local store is a later change.
package catalog

import (
	"context"
	"time"
)

// Freshness marks whether a catalog entry reflects the latest successful
// discovery (SPEC.md §8.1). Search and describe may use stale entries when
// clearly marked; invocation must verify live state.
type Freshness string

// Freshness values.
const (
	FreshnessFresh Freshness = "fresh"
	FreshnessStale Freshness = "stale"
)

// ServerStatus is the last-known runtime state of a downstream server.
type ServerStatus string

// Server status values.
const (
	ServerOnline  ServerStatus = "online"
	ServerOffline ServerStatus = "offline"
	ServerUnknown ServerStatus = "unknown"
)

// Server is a configured downstream MCP server known to the catalog.
type Server struct {
	ID     string
	Status ServerStatus
}

// Tool is a cataloged downstream tool with the minimum metadata from SPEC.md §8.
type Tool struct {
	ToolRef            string
	ServerID           string
	DownstreamToolName string
	Title              string
	Description        string
	InputSchema        map[string]any
	CapabilityText     []string
	ServerStatus       ServerStatus
	CallableNow        bool
	// ReadOnly is the downstream tool's readOnlyHint annotation. It gates
	// result caching: only positively read-only tools may be cached so a cache
	// hit can never substitute for a side-effecting invocation.
	ReadOnly      bool
	LastIndexedAt time.Time
	SchemaHash    string
	Freshness     Freshness
}

// Stats is lightweight catalog health used to drive instructional responses.
type Stats struct {
	ConfiguredServers int
	IndexedTools      int
	FreshTools        int
	StaleTools        int
}

// Store is the catalog persistence seam. Implementations must operate correctly
// when empty so the broker can return the catalog_empty decision (SPEC.md §9.1).
type Store interface {
	// PutServer inserts or replaces a server.
	PutServer(ctx context.Context, s Server) error
	// PutTool inserts or replaces a tool keyed by its stable toolRef.
	PutTool(ctx context.Context, t Tool) error
	// DeleteTools removes the tools with the given toolRefs. Unknown refs are
	// ignored, so reconciliation can pass its full computed set idempotently.
	DeleteTools(ctx context.Context, toolRefs []string) error
	// Servers returns all configured/known servers.
	Servers(ctx context.Context) ([]Server, error)
	// Tools returns all indexed tools.
	Tools(ctx context.Context) ([]Tool, error)
	// GetTool resolves one tool by its stable toolRef. The bool is false when the
	// tool is not in the catalog.
	GetTool(ctx context.Context, toolRef string) (Tool, bool, error)
	// Stats returns lightweight catalog health.
	Stats(ctx context.Context) (Stats, error)
	// LastIndexedAt returns the run-level timestamp of the last successful index
	// pass. The bool is true when a prior index has completed; false when the
	// catalog has never been indexed. A zero time with false means "never indexed".
	LastIndexedAt(ctx context.Context) (time.Time, bool, error)
	// SetLastIndexedAt records the timestamp of a completed index run.
	SetLastIndexedAt(ctx context.Context, t time.Time) error
}
