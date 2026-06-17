// Package sidecar is the Go client for Ozy's optional Python embedding
// sidecar. The sidecar is a child process that the daemon (or a focused
// caller) spawns and speaks to over its standard input and output using
// newline-delimited JSON. The sidecar's standard error is reserved for
// diagnostic logs and is drained into a Logger the caller supplies.
//
// A Client owns exactly one sidecar process. Each request is sent as one
// JSON object on stdin and correlated to its reply by the "id" field on
// stdout. The package degrades gracefully: when the sidecar is absent,
// fails to provision, crashes, or times out, every operation surfaces a
// structured "unavailable" signal so the search engine can fall back to
// the lexical baseline (SPEC.md §4.10, §10.1, §10.4).
package sidecar

// Operation names sent on the wire. Must match the Python sidecar exactly.
const (
	opHealth = "health"
	opReady  = "warmup"
	opUpsert = "upsert"
	opDelete = "delete"
	opQuery  = "query"
	opStats  = "stats"
)

// jsonlRequest is the wire shape sent to the sidecar. Exactly one
// operation-specific payload is set per request.
//
// The "filter" field is a pointer so the JSON encoder emits "filter":null
// (no filter) versus "filter":{} or "filter":{"serverId":"..."}. The
// Python sidecar treats null as "no filter".
type jsonlRequest struct {
	ID       string            `json:"id"`
	Op       string            `json:"op"`
	Items    []jsonlUpsertItem `json:"items,omitempty"`
	ToolRefs []string          `json:"toolRefs,omitempty"`
	Text     string            `json:"text,omitempty"`
	K        int               `json:"k,omitempty"`
	Filter   *jsonlFilter      `json:"filter"`
}

// jsonlFilter is the on-the-wire shape of a query facet filter. The only
// facet the v1 sidecar honours is serverId; ToolRefs is handled in Go and
// is never sent to the sidecar.
type jsonlFilter struct {
	ServerID string `json:"serverId"`
}

// jsonlUpsertItem is one document to embed and index.
type jsonlUpsertItem struct {
	ToolRef     string   `json:"toolRef"`
	Text        string   `json:"text"`
	ContentHash string   `json:"contentHash"`
	ServerID    string   `json:"serverId"`
	Tags        []string `json:"tags"`
}

// jsonlHit is one ranked nearest-neighbor returned by a query.
type jsonlHit struct {
	ToolRef string  `json:"toolRef"`
	Score   float64 `json:"score"`
}

// jsonlResponse is the wire shape received from the sidecar. The ID field
// is always echoed back; the error field is only set when ok is false.
type jsonlResponse struct {
	ID          string     `json:"id"`
	OK          bool       `json:"ok"`
	Model       string     `json:"model,omitempty"`
	Dim         int        `json:"dim,omitempty"`
	Backend     string     `json:"backend,omitempty"`
	VectorCount int        `json:"vectorCount,omitempty"`
	ToolCount   int        `json:"toolCount,omitempty"`
	Upserted    int        `json:"upserted,omitempty"`
	Skipped     int        `json:"skipped,omitempty"`
	Deleted     int        `json:"deleted,omitempty"`
	Errors      []string   `json:"errors,omitempty"`
	Hits        []jsonlHit `json:"hits,omitempty"`
	Error       string     `json:"error,omitempty"`
	// ErrorKind classifies an error so the Go client can map it to an
	// actionable reason. The readiness probe sets "model_download_incomplete"
	// when the model cannot be fetched even after a cache self-heal.
	ErrorKind string `json:"errorKind,omitempty"`
}
