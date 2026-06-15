## Context

Every agent-facing operation flows through one in-process seam, the `Broker`
interface (`internal/broker/broker.go`): `FindTool`, `DescribeTool`, `CallTool`,
`List`. The CLI and the MCP adapter both depend only on this interface, so a
single decorator placed here is seen by both paths and cannot drift.

The live broker (`internal/broker/live.go`) does real work on each call:
`FindTool` runs the search engine (and the embedding sidecar when semantic search
is on), `DescribeTool` reads the catalog, `CallTool` opens a downstream session
and performs `tools/call`. In an eval run the same query/lookup/read-only call
recurs, paying that cost repeatedly. The benchmark scoreboard counts that waste.

Downstream tools already advertise intent: the MCP SDK exposes
`Tool.Annotations.ReadOnlyHint`. Discovery (`internal/index/normalizeTool`) sees
it but currently drops it.

## Goals / Non-Goals

**Goals:**
- Remove redundant `findTool`/`describeTool`/read-only `callTool` work via a
  transparent cache on the broker seam.
- Make caching of a side-effecting tool impossible (default-deny write exclusion).
- One config switch in `ozy.jsonc`, default on; zero new dependencies.

**Non-Goals:**
- Cross-process or persisted cache. The cache is in-memory, per daemon process.
- Caching `List` (cheap, volatile) or caching error/failure outcomes.
- Negative caching (caching "not found" / no-match decisions for `callTool`).
- A real eviction policy (LRU/LFU). A size cap with simple eviction is enough.

## Decisions

### Decision: A decorator over `Broker`, not logic inside `live` or the adapter

A new `cachingBroker` in `internal/broker/cache.go` wraps any `Broker` and
implements `Broker`. Wiring (`internal/daemon`) wraps the live broker only when
`cfg.Cache.Enabled`. `live.go` and `adapter.go` are untouched, both paths benefit,
and disabling the feature is "don't wrap" — pure pass-through with zero overhead.

*Alternatives:* cache fields inside `live` (couples policy to implementation,
harder to disable); cache in the adapter (CLI wouldn't benefit, violates the
single-seam principle).

### Decision: Write-tool exclusion via captured `readOnlyHint`, default-deny

`normalizeTool` records `tool.Annotations != nil && tool.Annotations.ReadOnlyHint`
into a new `catalog.Tool.ReadOnly` field. `cachingBroker.CallTool` looks the tool
up (`store.GetTool`) and caches **only** when `ReadOnly` is true. Absent, false,
or unknown → invoke live, never store. Caching a mutating call would silently skip
a side effect — a data-loss class bug — so positive evidence is required.

*Alternatives:* tool-name heuristics (e.g. exclude `create*`/`delete*`) — brittle
and unsafe; a config allow/deny list — manual and drifts from reality. The MCP
annotation is the authoritative, server-declared signal.

### Decision: Content-hash key folded with a generation/schema token

Key = `sha256(op || inputs || token)`:
- `findTool`: inputs = query; token = catalog last-indexed generation
  (`store.LastIndexedAt`). Re-index advances the generation → automatic miss.
- `describeTool` / `callTool`: token = the target tool's `SchemaHash`
  (already computed at index time). A changed schema → automatic miss.
- `callTool` inputs additionally include the canonical JSON of arguments.
  `json.Marshal` orders map keys deterministically, so equal argument maps hash
  equally and distinct ones don't collide.

TTL bounds staleness regardless of these tokens. This gives content-addressed
invalidation without any explicit cache-busting calls from the index path.

### Decision: In-process map with lazy TTL + capped size

A `sync.Mutex`-guarded `map[string]entry{value any; expires time.Time}`. Expiry is
checked on read (lazy). On insert past `MaxEntries`, evict expired entries first
and, if still full, drop one arbitrary entry.
`// ponytail: arbitrary eviction at cap; swap for LRU only if hit-rate data shows it matters.`

### Decision: `cache` config section, default on

```jsonc
"cache": { "enabled": true, "ttlSeconds": 300, "maxEntries": 1024 }
```
`CacheConfig` follows the existing `SemanticSearch` pattern: a raw JSON struct with
`*bool Enabled` so an omitted `enabled` defaults to true while explicit `false`
disables. Defaults applied in `applyDefaults`: ttl 300s, maxEntries 1024.
Default-on matches the project's posture (semantic search is on by default) and the
benchmark goal; `enabled: false` is the documented escape hatch.

## Risks / Trade-offs

- **Stale read-only result within TTL** → Read-only by definition has no side
  effect, and schema/generation tokens plus a short default TTL bound drift. Tune
  `ttlSeconds` down (or disable) for fast-changing read tools.
- **Many servers omit `readOnlyHint`** → those `callTool`s simply aren't cached
  (correctness preserved, benefit forgone). `findTool`/`describeTool` — the cited
  high-frequency wins — still hit fully.
- **A tool removed/disabled after caching** → `callTool` always re-resolves via
  `GetTool` (miss → live path returns the proper error); `findTool`/`describeTool`
  bounded by generation/schemaHash + TTL.
- **Unbounded memory** → `MaxEntries` cap; values are small result structs.
- **Existing file-catalog entries predate `ReadOnly`** → deserialize as `false`
  (safe default-deny); those tools become call-cacheable after the next `ozy index`.

## Migration Plan

Additive and default-on. No data migration: `catalog.Tool.ReadOnly` is a new field
whose zero value (`false`) is the safe default; a re-index populates it. Rollback is
`cache.enabled: false` in config, or reverting the one-line daemon wrap.

## Open Questions

- Default TTL of 300s is a starting point; the eval run can tune it. Not blocking.
- Gate only on `readOnlyHint`, not `idempotentHint` — an idempotent *write* is still
  a write, so `idempotentHint` is intentionally ignored for caching.
