## MODIFIED Requirements

### Requirement: `ozy index` populates the catalog

The `ozy index` command SHALL connect to configured servers, discover their tools, write the normalized tools to the catalog, and report a summary of servers indexed and tools discovered (including per-server failures). An index run SHALL also reconcile the catalog against what the run learned: for each server whose `tools/list` succeeded, cataloged tools that server no longer serves SHALL be deleted; tools whose server is no longer present in the configuration SHALL be deleted; tools whose server is configured but unreachable or disabled SHALL be kept and degraded (marked stale and not callable) rather than deleted. Catalog deletions SHALL be propagated to the embedding sink in the same run so vector storage tracks the catalog.

#### Scenario: Indexing reports a summary

- **WHEN** a user runs `ozy index` with at least one reachable configured server
- **THEN** the catalog is populated with the discovered tools and the command reports how many servers were reached and how many tools were indexed, plus any per-server errors

#### Scenario: Indexing with no reachable servers is instructional

- **WHEN** `ozy index` runs but no configured server is reachable
- **THEN** it reports the per-server failures with repair guidance rather than silently succeeding

#### Scenario: A vanished tool is deleted

- **WHEN** an index run's `tools/list` succeeds for a server and a previously cataloged tool of that server is absent from the listing
- **THEN** the tool's catalog entry is deleted and its embedding is removed from the sink

#### Scenario: A removed server's tools are deleted

- **WHEN** an index run finds cataloged tools whose server is no longer present in the configuration
- **THEN** those catalog entries are deleted, because `callTool` would refuse them with `CONFIG_ERROR` anyway

#### Scenario: An unreachable server's tools are degraded, not deleted

- **WHEN** an index run cannot reach a configured, enabled server
- **THEN** that server's cataloged tools are kept but marked stale, not callable, and their server status offline — a flake never erases the catalog

#### Scenario: A failed listing never triggers deletion

- **WHEN** a server connects but its `tools/list` call fails during an index run
- **THEN** no tool of that server is deleted or degraded to a vanished state on the basis of that failed listing

### Requirement: Discovered tools carry freshness and runtime status

Each cataloged tool SHALL record `lastIndexedAt`, a `schemaHash`, a freshness marker, and the server's runtime status at index time (SPEC.md §8, §8.1). Freshness, callability, and server status SHALL reflect the outcome of the most recent index run for that tool's server — they are reconciled state, never values hardcoded fresh/callable at write time and left to drift.

#### Scenario: A freshly indexed tool is marked fresh

- **WHEN** a tool is discovered from a reachable server during `ozy index`
- **THEN** its catalog entry records `lastIndexedAt`, a schema hash, freshness `fresh`, and the server status

#### Scenario: Status responses reflect reconciled state

- **WHEN** a tool's server was unreachable during the most recent index run and an agent receives that tool in a `findTool` or `describeTool` response
- **THEN** the response reports the tool as stale and not callable with server status offline, rather than the fresh/callable/online snapshot from an earlier run
