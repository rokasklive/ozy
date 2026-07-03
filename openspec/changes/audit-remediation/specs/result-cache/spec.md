## MODIFIED Requirements

### Requirement: Broker result caching on the shared seam

Ozy SHALL provide an optional result cache implemented as a transparent decorator over the shared `Broker` interface, so that both the CLI and the MCP adapter benefit from caching without either path importing cache logic. When enabled, the cache SHALL serve a previously computed result for `findTool`, `describeTool`, and read-only `callTool` requests whose inputs and content token match a live, non-expired entry, and SHALL otherwise delegate to the underlying broker and store the successful result. A cached result SHALL be equivalent to the result the underlying broker would have produced for the same request at index time, except that a cached `callTool` result additionally carries the cache-hit stamp defined below.

#### Scenario: Repeated findTool query is served from cache

- **WHEN** caching is enabled and `findTool` is called twice with the same query within the TTL and without an intervening re-index
- **THEN** the second call returns the cached result without re-running search or contacting the embedding sidecar
- **AND** the returned result is equal to the first call's result

#### Scenario: describeTool result is served from cache

- **WHEN** caching is enabled and `describeTool` is called twice for the same `toolRef` whose catalog entry is unchanged
- **THEN** the second call returns the cached describe result without re-reading the underlying catalog entry

#### Scenario: Cache decorator preserves the broker contract

- **WHEN** an agent calls `findTool`, `describeTool`, or `callTool` through the cache decorator
- **THEN** the response shape is identical to calling the underlying broker directly (a cached `callTool` result differing only by its cache-hit stamp), and the decorator exposes the same `Broker` interface

## ADDED Requirements

### Requirement: Cache hits are visibly stamped

A `callTool` result served from the cache SHALL carry the age of the cached entry (seconds since it was produced), and the agent-facing response SHALL state in-band that the result is cached and how old it is. `readOnlyHint` asserts absence of side effects, not temporal validity — the environment can change between calls — so an agent must be able to tell a cached observation from a live one. The stamp SHALL be applied to a copy; the stored cache entry SHALL never be mutated.

#### Scenario: A cached read is labeled with its age

- **WHEN** caching is enabled and an agent's read-only `callTool` is served from the cache
- **THEN** the response states that the result is cached and reports its age, in the response content the agent sees

#### Scenario: A live invocation carries no cache stamp

- **WHEN** a `callTool` is invoked live against the downstream server
- **THEN** the response carries no cache stamp

#### Scenario: The stored entry is never mutated by stamping

- **WHEN** the same cached `callTool` entry is served twice at different times
- **THEN** each response reports its own age at serve time, and the underlying stored entry remains unchanged
