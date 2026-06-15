## Why

The benchmark scoreboard tracks total-tokens-to-success, latency, and redundant
work per run. In a typical agent loop the same `findTool` query, `describeTool`
lookup, or read-only `callTool` invocation is issued more than once (retries,
re-discovery, multi-step plans). Each repeat re-runs search (and the embedding
sidecar), re-reads the catalog, or re-invokes a downstream server — paying the
full cost again for a result that has not changed. A small result cache on the
shared broker seam removes that redundant work and improves exactly the numbers
the scoreboard measures, while staying safe by never caching a tool that can
mutate downstream state.

## What Changes

- Add a result cache as a transparent decorator over the shared `Broker` seam, so
  both the CLI and the MCP adapter benefit without either importing cache logic.
- Cache `findTool` and `describeTool` results unconditionally (pure catalog/search
  reads) with a TTL.
- Cache `callTool` results **only** for tools with a positive `readOnlyHint`
  annotation; write tools and tools of unknown intent are never cached
  (default-deny write-tool exclusion).
- Key entries by a content hash of the request (operation + inputs) folded with a
  content/generation token (per-tool `schemaHash`, catalog last-indexed
  generation) so re-indexing or a changed tool schema invalidates stale entries.
- Never cache error or failure envelopes — only successful results.
- Add a `cache` section to `ozy.jsonc` to toggle the cache on/off and tune TTL and
  size. Default **on** (consistent with the project's good-defaults posture).
- Capture each downstream tool's `readOnlyHint` during discovery so the cache can
  apply write-tool exclusion.

## Capabilities

### New Capabilities
- `result-cache`: a TTL + content-hash result cache on the broker seam, with
  default-deny write-tool exclusion and a configuration toggle.

### Modified Capabilities
- `configuration`: the loader recognizes a new top-level `cache` section (enabled,
  TTL, max entries) with documented defaults.
- `mcp-adapter`: the `callTool` and `findTool` live-invocation/live-discovery
  requirements are refined to permit serving a cached, content-equivalent result
  within TTL when caching is enabled — while write/unknown tools always invoke
  live.

## Impact

- `internal/broker`: new `cache.go` decorator implementing `Broker`; no change to
  `live.go` behavior.
- `internal/config`: new `CacheConfig` struct + defaults + validation.
- `internal/catalog`: `Tool` gains a `ReadOnly` field.
- `internal/index`: `normalizeTool` records `readOnlyHint` from downstream
  annotations.
- `internal/daemon`: wraps the live broker with the cache when enabled.
- No new third-party dependencies (stdlib `crypto/sha256`, `sync`, `time`).
