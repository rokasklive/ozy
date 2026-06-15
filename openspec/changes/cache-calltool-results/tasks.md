## 1. Capture read-only intent at discovery

- [x] 1.1 Add a `ReadOnly bool` field to `catalog.Tool` (`internal/catalog/catalog.go`); confirm the file and memory stores round-trip it.
- [x] 1.2 In `normalizeTool` (`internal/index/index.go`) set `ReadOnly` from `tool.Annotations != nil && tool.Annotations.ReadOnlyHint`.
- [x] 1.3 Test: a discovered tool with `readOnlyHint: true` yields `Tool.ReadOnly == true`; absent or `false` yields `false`.

## 2. Cache configuration

- [x] 2.1 Add `CacheConfig` plus a `cacheConfigJSON` raw form with `*bool Enabled` (omitted → true) to `internal/config/config.go`, and a `Cache CacheConfig` field on `Config`.
- [x] 2.2 Apply defaults in `applyDefaults` (enabled true, `ttlSeconds` 300, `maxEntries` 1024) and include `Cache` in `cloneConfig`.
- [x] 2.3 Validate in `validate()`: negative `ttlSeconds` or `maxEntries` returns a structured `CONFIG_ERROR`.
- [x] 2.4 Test: omitted `cache` section → enabled with defaults; `enabled: false` → disabled; explicit `ttlSeconds`/`maxEntries` preserved.

## 3. Caching broker decorator

- [x] 3.1 Create `internal/broker/cache.go`: `cachingBroker` wrapping a `Broker`, a `catalog.Store`, and the resolved cache settings; add `NewCaching(inner Broker, store catalog.Store, cfg config.CacheConfig) Broker`.
- [x] 3.2 Implement a `sync.Mutex`-guarded TTL map (entry = value + expiry) with lazy expiry on read and a `MaxEntries` cap (evict expired first, then drop one arbitrary entry). Mark the eviction ceiling with a `ponytail:` comment.
- [x] 3.3 Implement the key builder: `sha256(op || inputs || token)` — `findTool` token = catalog generation (`store.LastIndexedAt`), `describeTool`/`callTool` token = the target tool's `SchemaHash`, and `callTool` inputs include the canonical `json.Marshal` of arguments.
- [x] 3.4 `FindTool` and `DescribeTool`: serve from cache on a live hit, otherwise delegate and store only successful results.
- [x] 3.5 `CallTool`: resolve the tool via `store.GetTool`; cache only when `ReadOnly` is true; delegate and store on a successful read-only call; never store failures; pass `List` straight through.
- [x] 3.6 Tests: hit/miss, TTL expiry, generation and schemaHash invalidation, distinct-args keying, write tool never cached, unknown/absent-annotation tool invoked live, failures not cached.

## 4. Wire into the daemon

- [x] 4.1 In `internal/daemon/daemon.go`, wrap `broker.NewLive(...)` with `broker.NewCaching(...)` (same `store`) when `cfg.Cache.Enabled`; leave it unwrapped otherwise.
- [x] 4.2 Confirm CLI and MCP both route through the cache and that `enabled: false` yields behavior identical to today (pure pass-through).

## 5. Config surface and docs

- [x] 5.1 Add a `cache` block to the init scaffold (`internal/config/scaffold.go`) and any bundled example fixture / JSON schema.
- [x] 5.2 Document the `cache` section (`enabled`/`ttlSeconds`/`maxEntries`, default-on, read-only write-tool exclusion) in the README / SPEC configuration docs.

## 6. Verify

- [x] 6.1 `go test ./...` green and `go vet ./...` clean.
- [x] 6.2 Run `graphify update .` to refresh the knowledge graph.
