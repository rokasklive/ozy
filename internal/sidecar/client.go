package sidecar

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures a new Client. A zero Options uses sensible
// defaults: no logger (stderr is silently drained), no Driver (a real
// subprocess is spawned), and no DataDir (the sidecar uses the current
// working directory). NewClient validates the combination and
// returns a structured error on misconfiguration.
type Options struct {
	// DataDir is passed to the sidecar as --data-dir.
	DataDir string

	// Backend names the vector index implementation — "turbovec" or
	// "faiss". Defaults to "turbovec".
	Backend string

	// Model is the FastEmbed model id. Defaults to the documented
	// FastEmbed CPU-friendly model when empty.
	Model string

	// Logger receives the sidecar's stderr output, one line per
	// message. nil drops the messages.
	Logger Logger

	// Driver replaces the real subprocess for tests. nil means "use
	// the real Python sidecar".
	Driver driver

	// ProcessOptions supplies the interpreter, source directory, and
	// extra environment for the real subprocess. Ignored when Driver
	// is set.
	ProcessOptions ProcessOptions
}

// HealthResult is the structured outcome of a single Health call. OK
// reflects the sidecar's response; Available mirrors OK and is the
// value callers should propagate to consumers.
type HealthResult struct {
	OK          bool
	Available   bool
	Model       string
	Dim         int
	Backend     string
	VectorCount int
	Err         error
}

// UpsertItem is one document to embed and store.
type UpsertItem struct {
	ToolRef     string
	Text        string
	ContentHash string
	ServerID    string
	Tags        []string
}

// UpsertResult is the structured outcome of an Upsert call.
type UpsertResult struct {
	Upserted int
	Skipped  int
	Errors   []string
}

// DeleteResult is the structured outcome of a Delete call.
type DeleteResult struct {
	Deleted int
}

// QueryHit is one ranked nearest-neighbor returned by a Query call.
type QueryHit struct {
	ToolRef string
	Score   float64
}

// QueryResult is the structured outcome of a Query call.
type QueryResult struct {
	Hits []QueryHit
}

// SearchFilter narrows a semantic query to a subset of the indexed
// tools. An empty SearchFilter (IsEmpty reports true) means "no
// filter" and is serialized as filter:null on the wire. The v1
// sidecar only honours ServerID; ToolRefs is left for a future
// revision and is intentionally not part of this type.
type SearchFilter struct {
	ServerID string
}

// IsEmpty reports whether the filter would be a no-op if sent to
// the sidecar. A nil-or-empty Filter is serialized as filter:null.
func (f SearchFilter) IsEmpty() bool {
	return f.ServerID == ""
}

// StatsResult is the structured outcome of a Stats call.
type StatsResult struct {
	Backend     string
	Model       string
	Dim         int
	VectorCount int
	ToolCount   int
}

// Client is the Go side of the JSONL protocol. A Client owns exactly
// one sidecar process and is safe for concurrent use; the package
// serializes requests so the protocol is request/response with one
// in-flight call at a time, which keeps the implementation simple and
// matches the documented Python sidecar contract.
//
// Operations after a failed Health, after a runtime error, or after
// Close return an "unavailable" signal without doing I/O. The
// semantic.go adapter maps this to "degrade to lexical-only".
type Client struct {
	driver driver
	stdin  io.WriteCloser
	stdout io.ReadCloser
	logger Logger

	pendingMu sync.Mutex
	pending   map[string]chan jsonlResponse

	inFlight sync.Mutex

	nextID atomic.Uint64

	closed  atomic.Bool
	healthy atomic.Bool

	readDone chan struct{}
	readErr  chan error
}

// NewClient constructs a Client. It starts the sidecar subprocess (or
// the supplied fake Driver) and returns a ready-to-use Client. The
// caller MUST call Health before issuing other operations; NewClient
// itself does not perform a health probe so it can be used in tests
// that want to control health-check timing.
func NewClient(opts Options) (*Client, error) {
	if opts.Logger == nil {
		opts.Logger = noopLogger{}
	}
	if opts.Backend == "" {
		opts.Backend = "turbovec"
	}
	if opts.DataDir == "" {
		opts.DataDir = "."
	}

	c := &Client{
		logger:   opts.Logger,
		pending:  make(map[string]chan jsonlResponse),
		readDone: make(chan struct{}),
		readErr:  make(chan error, 1),
	}

	if opts.Driver != nil {
		c.driver = opts.Driver
	} else {
		if opts.ProcessOptions.PythonPath == "" {
			return nil, errors.New("sidecar: Options.ProcessOptions.PythonPath is required when Driver is nil")
		}
		c.driver = newRealDriver(opts.ProcessOptions)
	}

	stdin, stdout, stderr, _, err := c.driver.Start(context.Background())
	if err != nil {
		return nil, err
	}
	c.stdin = stdin
	c.stdout = stdout

	go drainStderr(stderr, c.logger)
	go c.readLoop(stdout)

	return c, nil
}

// Health issues a health probe and returns the structured result. A
// successful Health marks the Client healthy; all later operations
// behave as "available" until a runtime failure or Close. A failed
// Health marks the Client unhealthy; subsequent operations return
// errors without performing I/O.
func (c *Client) Health(ctx context.Context) HealthResult {
	resp, err := c.call(ctx, opHealth, &jsonlRequest{})
	if err != nil {
		c.healthy.Store(false)
		return HealthResult{Available: false, Err: err}
	}
	if !resp.OK {
		c.healthy.Store(false)
		return HealthResult{
			Available: false,
			Err:       fmt.Errorf("sidecar: health not ok: %s", resp.Error),
		}
	}
	c.healthy.Store(true)
	return HealthResult{
		OK:          true,
		Available:   true,
		Model:       resp.Model,
		Dim:         resp.Dim,
		Backend:     resp.Backend,
		VectorCount: resp.VectorCount,
	}
}

// Upsert embeds and stores one or more documents. An empty input is
// a no-op. The returned Errors slice mirrors the sidecar's
// per-document error list; a non-nil error means the request itself
// failed (transport, timeout, malformed response).
func (c *Client) Upsert(ctx context.Context, items []UpsertItem) (UpsertResult, error) {
	if len(items) == 0 {
		return UpsertResult{}, nil
	}
	wire := make([]jsonlUpsertItem, len(items))
	for i, it := range items {
		wire[i] = jsonlUpsertItem{
			ToolRef:     it.ToolRef,
			Text:        it.Text,
			ContentHash: it.ContentHash,
			ServerID:    it.ServerID,
			Tags:        it.Tags,
		}
	}
	resp, err := c.call(ctx, opUpsert, &jsonlRequest{Items: wire})
	if err != nil {
		return UpsertResult{}, err
	}
	return UpsertResult{
		Upserted: resp.Upserted,
		Skipped:  resp.Skipped,
		Errors:   resp.Errors,
	}, nil
}

// Delete removes one or more tools from the sidecar's store. An empty
// input is a no-op.
func (c *Client) Delete(ctx context.Context, toolRefs []string) (DeleteResult, error) {
	if len(toolRefs) == 0 {
		return DeleteResult{}, nil
	}
	resp, err := c.call(ctx, opDelete, &jsonlRequest{ToolRefs: toolRefs})
	if err != nil {
		return DeleteResult{}, err
	}
	return DeleteResult{Deleted: resp.Deleted}, nil
}

// Query embeds the query string, searches the vector index, and
// returns up to k nearest neighbors. A nil filter (the zero
// searchFilter) means "no filter" and is sent as filter:null on the
// wire.
func (c *Client) Query(ctx context.Context, text string, k int, filter SearchFilter) (QueryResult, error) {
	wire := &jsonlRequest{Text: text, K: k}
	if !filter.IsEmpty() {
		wire.Filter = &jsonlFilter{ServerID: filter.ServerID}
	}
	resp, err := c.call(ctx, opQuery, wire)
	if err != nil {
		return QueryResult{}, err
	}
	hits := make([]QueryHit, len(resp.Hits))
	for i, h := range resp.Hits {
		hits[i] = QueryHit{ToolRef: h.ToolRef, Score: h.Score}
	}
	return QueryResult{Hits: hits}, nil
}

// Stats returns the sidecar's bookkeeping — backend, model, dimension,
// and the count of vectors and toolRefs it currently stores.
func (c *Client) Stats(ctx context.Context) (StatsResult, error) {
	resp, err := c.call(ctx, opStats, &jsonlRequest{})
	if err != nil {
		return StatsResult{}, err
	}
	return StatsResult{
		Backend:     resp.Backend,
		Model:       resp.Model,
		Dim:         resp.Dim,
		VectorCount: resp.VectorCount,
		ToolCount:   resp.ToolCount,
	}, nil
}

// Available reports whether the Client is currently usable. It is
// true only after a successful Health and stays true until a runtime
// error or Close. Callers should treat it as advisory and gate
// non-critical work on it; the methods themselves do not panic.
func (c *Client) Available() bool {
	return c.healthy.Load() && !c.closed.Load()
}

// Close terminates the sidecar. It is idempotent and safe to call
// from multiple goroutines. After Close, all subsequent operations
// return an "unavailable" error without performing I/O.
func (c *Client) Close() error {
	var err error
	if c.closed.CompareAndSwap(false, true) {
		if c.driver != nil {
			err = c.driver.Close()
		}
		// Close stdin so the reader goroutine sees EOF if it hasn't
		// already. (realDriver already closed stdin; the fake driver
		// closes its pipe writer in Close.)
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		// Close stdout so the reader goroutine unblocks.
		if c.stdout != nil {
			_ = c.stdout.Close()
		}
		// Wait briefly for the read loop to finish so we don't race
		// with its final markUnhealthy call.
		select {
		case <-c.readDone:
		case <-time.After(time.Second):
		}
	}
	return err
}

// call is the single point of contact with the wire. It enforces
// the one-in-flight rule, generates a unique request ID, writes the
// request, and waits for the matching response. Any failure marks
// the Client unhealthy and short-circuits future calls.
func (c *Client) call(ctx context.Context, op string, req *jsonlRequest) (*jsonlResponse, error) {
	if c.closed.Load() {
		return nil, errUnavailable
	}

	// Enforce one-in-flight. The lock is released when the call
	// returns (deferred). A new caller arriving after a failure
	// will still see closed=true (or healthy=false once we
	// markUnhealthy) and bail.
	c.inFlight.Lock()
	defer c.inFlight.Unlock()

	if c.closed.Load() {
		return nil, errUnavailable
	}
	if !c.healthy.Load() && op != opHealth {
		// Allow Health to run after a failure so the caller can
		// re-probe. Other operations are blocked once we've been
		// marked unhealthy.
		return nil, errUnavailable
	}

	id := fmt.Sprintf("r%d", c.nextID.Add(1))
	req.ID = id
	req.Op = op

	ch := make(chan jsonlResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	defer func() {
		// Ensure the pending entry is removed so the reader does
		// not see it after we've returned.
		c.pendingMu.Lock()
		if cur, ok := c.pending[id]; ok && cur == ch {
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	}()

	if err := c.writeRequest(req); err != nil {
		c.markUnhealthy(fmt.Errorf("sidecar: write %s: %w", op, err))
		return nil, err
	}

	select {
	case resp := <-ch:
		if !resp.OK {
			return &resp, fmt.Errorf("sidecar: %s: %s", op, resp.Error)
		}
		return &resp, nil
	case <-ctx.Done():
		c.markUnhealthy(fmt.Errorf("sidecar: %s: %w", op, ctx.Err()))
		return nil, ctx.Err()
	case <-c.readDone:
		// The reader exited (sidecar crash, malformed response, or
		// explicit close). Signal unavailability so callers don't
		// block forever.
		c.markUnhealthy(errors.New("sidecar: reader exited"))
		return nil, errUnavailable
	}
}

// writeRequest encodes the request to a single JSON line and writes
// it to stdin, flushing the underlying writer. A newline is appended
// because the Python sidecar uses line-buffered stdin.
func (c *Client) writeRequest(req *jsonlRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if _, err := io.WriteString(c.stdin, string(data)); err != nil {
		return err
	}
	if f, ok := c.stdin.(interface{ Sync() error }); ok {
		_ = f.Sync()
	}
	return nil
}

// readLoop is the single goroutine that consumes the sidecar's
// stdout. It dispatches each JSONL response to the per-request
// channel keyed by id, then removes the entry from the pending map.
// On EOF or any error it marks the Client unhealthy and returns.
func (c *Client) readLoop(stdout io.ReadCloser) {
	defer close(c.readDone)
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var resp jsonlResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			c.markUnhealthy(fmt.Errorf("sidecar: malformed response: %w", err))
			return
		}
		if resp.ID == "" {
			c.markUnhealthy(errors.New("sidecar: response missing id"))
			return
		}
		ch := c.takePending(resp.ID)
		if ch == nil {
			// Late response for a request that already timed out
			// or completed. Drop it; the request is no longer
			// listening.
			continue
		}
		ch <- resp
	}
	if err := scanner.Err(); err != nil {
		select {
		case c.readErr <- err:
		default:
		}
		c.markUnhealthy(fmt.Errorf("sidecar: stdout read: %w", err))
		return
	}
	// EOF means the sidecar exited (cleanly or otherwise). Mark
	// unhealthy so subsequent callers see Unavailable.
	c.markUnhealthy(errors.New("sidecar: stdout closed"))
}

// takePending atomically removes the entry for id and returns its
// channel. The reader uses this to deliver a response exactly once;
// the request handler also uses it when the context expires so
// late responses are dropped on the floor.
func (c *Client) takePending(id string) chan jsonlResponse {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	ch, ok := c.pending[id]
	if ok {
		delete(c.pending, id)
	}
	return ch
}

// markUnhealthy flips the healthy flag and, if the Client has not
// been closed, kills the underlying driver. It is safe to call from
// multiple goroutines.
func (c *Client) markUnhealthy(err error) {
	if c.healthy.CompareAndSwap(true, false) {
		// First time we go unhealthy; emit a diagnostic if the
		// caller wired a logger.
		if c.logger != nil && err != nil {
			c.logger.Log("sidecar: " + err.Error())
		}
	}
	if c.closed.Load() {
		return
	}
	// Kill the driver so the read loop unblocks. We use Close
	// directly because it is idempotent.
	if c.driver != nil {
		_ = c.driver.Close()
	}
}

// noopLogger is the default Logger used when the caller did not
// supply one.
type noopLogger struct{}

func (noopLogger) Log(string) {}

// errUnavailable is the package-private sentinel for "the sidecar
// is not usable right now". All public methods translate it into a
// descriptive error before returning.
var errUnavailable = errors.New("sidecar: unavailable")
