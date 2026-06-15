## ADDED Requirements

### Requirement: Broker result caching on the shared seam

Ozy SHALL provide an optional result cache implemented as a transparent decorator over the shared `Broker` interface, so that both the CLI and the MCP adapter benefit from caching without either path importing cache logic. When enabled, the cache SHALL serve a previously computed result for `findTool`, `describeTool`, and read-only `callTool` requests whose inputs and content token match a live, non-expired entry, and SHALL otherwise delegate to the underlying broker and store the successful result. A cached result SHALL be equivalent to the result the underlying broker would have produced for the same request at index time.

#### Scenario: Repeated findTool query is served from cache

- **WHEN** caching is enabled and `findTool` is called twice with the same query within the TTL and without an intervening re-index
- **THEN** the second call returns the cached result without re-running search or contacting the embedding sidecar
- **AND** the returned result is equal to the first call's result

#### Scenario: describeTool result is served from cache

- **WHEN** caching is enabled and `describeTool` is called twice for the same `toolRef` whose catalog entry is unchanged
- **THEN** the second call returns the cached describe result without re-reading the underlying catalog entry

#### Scenario: Cache decorator preserves the broker contract

- **WHEN** an agent calls `findTool`, `describeTool`, or `callTool` through the cache decorator
- **THEN** the response shape is identical to calling the underlying broker directly, and the decorator exposes the same `Broker` interface

### Requirement: Read-only write-tool exclusion is default-deny

Ozy SHALL cache `callTool` results only for tools that carry positive read-only evidence — the downstream tool's `readOnlyHint` annotation is `true`. Tools whose annotation is absent, `false`, or otherwise unknown SHALL NOT be cached and SHALL always be invoked live, so a cache hit can never substitute for a side-effecting invocation. The List operation SHALL NOT be cached.

#### Scenario: Read-only tool result is cached

- **WHEN** caching is enabled and a `callTool` invocation targets a tool whose `readOnlyHint` is `true`, with identical arguments, within the TTL
- **THEN** the repeated invocation returns the cached result without performing a second downstream `tools/call`

#### Scenario: Write tool is never cached

- **WHEN** a `callTool` invocation targets a tool whose `readOnlyHint` is `false`
- **THEN** every invocation performs a live downstream `tools/call` and no result is stored in the cache

#### Scenario: Unknown read-only intent defaults to live invocation

- **WHEN** a `callTool` invocation targets a tool that is not in the catalog or whose read-only annotation is absent
- **THEN** the invocation is performed live and its result is not cached

### Requirement: TTL expiry

Each cached entry SHALL carry an expiry derived from the configured TTL. A request matching an expired entry SHALL be treated as a miss: the underlying broker is consulted and the entry is refreshed. The TTL SHALL be configurable.

#### Scenario: Expired entry is recomputed

- **WHEN** a cached result's TTL has elapsed and the same request is made again
- **THEN** the cache treats it as a miss, delegates to the underlying broker, and stores the new result

### Requirement: Content-hash invalidation

The cache key SHALL be a content hash of the request — the operation plus its inputs (`findTool` query; `describeTool` toolRef; `callTool` toolRef and canonically serialized arguments) — folded with a content/generation token: the catalog last-indexed generation for `findTool`, and the target tool's `schemaHash` for `describeTool` and `callTool`. A re-index that advances the generation, or a tool whose schema content hash changes, SHALL therefore miss prior entries rather than serve stale results.

#### Scenario: Re-index invalidates findTool entries

- **WHEN** a `findTool` result is cached and the catalog is subsequently re-indexed, advancing its generation
- **THEN** the same query is treated as a miss and recomputed against the new catalog

#### Scenario: Changed tool schema invalidates its entries

- **WHEN** a tool's `schemaHash` changes after re-indexing
- **THEN** previously cached `describeTool` and `callTool` entries for that tool are no longer matched

#### Scenario: Different arguments are distinct entries

- **WHEN** a read-only tool is called with two different argument sets
- **THEN** each argument set is keyed independently and does not collide with the other

### Requirement: Failures are never cached

The cache SHALL store only successful results. A `describeTool` or `callTool` error, a structured failure envelope, or any non-success outcome SHALL pass through without being stored, so a transient downstream failure cannot be replayed from cache.

#### Scenario: Downstream failure is not cached

- **WHEN** a `callTool` invocation returns a structured failure
- **THEN** the failure is returned to the caller and no entry is stored, so a later identical call is retried live

### Requirement: Cache configuration and toggle

Ozy SHALL expose a top-level `cache` configuration section in `ozy.jsonc` that toggles the result cache and tunes its TTL and maximum entry count. The cache SHALL be enabled by default. When disabled, every request SHALL be a pure pass-through to the underlying broker with no entries stored or served.

#### Scenario: Cache disabled is a pure pass-through

- **WHEN** the `cache` section sets `enabled: false`
- **THEN** every `findTool`, `describeTool`, and `callTool` request delegates directly to the underlying broker and nothing is cached

#### Scenario: Cache defaults to enabled

- **WHEN** configuration omits the `cache` section
- **THEN** the result cache is active with the documented default TTL and maximum entry count

#### Scenario: TTL and size are configurable

- **WHEN** the `cache` section sets a TTL and a maximum entry count
- **THEN** entries expire after the configured TTL and the number of stored entries is bounded by the configured maximum
