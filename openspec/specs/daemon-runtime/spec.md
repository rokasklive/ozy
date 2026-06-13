# daemon-runtime

## Purpose

Define Ozy's runtime core (`SPEC.md` §6.1): the daemon lifecycle, the single
in-process broker seam shared by every adapter, the catalog store interface, and
graceful degradation when optional subsystems (semantic search, the embedding
worker) are absent.

## Requirements

### Requirement: Daemon lifecycle

Ozy SHALL provide a daemon process that starts, loads configuration, initializes the broker seam and catalog store, and shuts down cleanly on interrupt, per `SPEC.md` §6.1.

#### Scenario: Daemon starts and reports readiness

- **WHEN** a user runs `ozy daemon` with valid configuration
- **THEN** the daemon initializes configuration, broker, and catalog store, and reports a ready state

#### Scenario: Daemon shuts down cleanly

- **WHEN** the daemon receives an interrupt or termination signal
- **THEN** it stops accepting work and releases resources without leaving the process hung

#### Scenario: Daemon refuses to start on invalid configuration

- **WHEN** the daemon starts with configuration that fails validation
- **THEN** it reports the structured `CONFIG_ERROR` and exits non-zero instead of starting in a broken state

### Requirement: Shared in-process broker seam

Ozy SHALL define a single broker interface exposing the `findTool`, `describeTool`, and `callTool` operations, and both the CLI and MCP adapter SHALL depend on this interface so adapter behavior stays semantically equivalent (`SPEC.md` §4.9).

#### Scenario: Single broker interface backs all adapters

- **WHEN** the CLI and the MCP adapter perform a broker operation
- **THEN** both call the same broker interface implementation rather than duplicating logic

### Requirement: Catalog store interface placeholder

Ozy SHALL define a catalog store interface owning servers, tools, schemas, freshness, and runtime status as described in `SPEC.md` §6.1 and §8, with a working in-memory or local placeholder implementation, and SHALL operate when the catalog is empty.

#### Scenario: Catalog store seam is present

- **WHEN** the daemon initializes
- **THEN** it constructs a catalog store implementing the catalog interface

#### Scenario: Empty catalog is handled gracefully

- **WHEN** a broker operation runs against an empty catalog
- **THEN** the broker returns an instructional empty-state result (e.g. the `catalog_empty` decision) rather than an error or crash, per `SPEC.md` §9.1

### Requirement: Graceful degradation of optional subsystems

The daemon SHALL operate without semantic search and without the optional Python embedding worker, falling back to baseline behavior, per `SPEC.md` §4.10 and §10.1.

#### Scenario: Runs with semantic search disabled

- **WHEN** the daemon starts with semantic search disabled or unavailable
- **THEN** it starts successfully and relies on the lexical/baseline path without requiring the optional worker

### Requirement: Conditional indexing on startup

On startup the daemon SHALL determine whether the catalog is stale relative to the configuration file and, when it is, run an index pass to populate the catalog before reporting readiness. The catalog is stale when no prior successful index exists, or when the last successful index time predates the modification time of the loaded configuration file (`ozy.jsonc`). When the catalog is not stale, the daemon SHALL skip startup indexing and serve the existing catalog (`SPEC.md` §12).

#### Scenario: A stale catalog is reindexed on startup

- **WHEN** the daemon starts and the loaded configuration file's modification time is newer than the catalog's last successful index time (or no prior index exists)
- **THEN** the daemon runs an index pass to refresh the catalog before reporting ready

#### Scenario: A fresh catalog skips startup indexing

- **WHEN** the daemon starts and the catalog's last successful index time is at or after the configuration file's modification time
- **THEN** the daemon does not re-index on startup and serves the existing catalog

### Requirement: Startup indexing degrades gracefully

Startup indexing SHALL NOT prevent the daemon from becoming ready. When startup indexing fails entirely or no configured server is reachable, the daemon SHALL still report readiness, serve whatever catalog already exists, and surface the indexing outcome rather than crashing or hanging (`SPEC.md` §4.11, §10.1).

#### Scenario: Startup indexing failure does not block readiness

- **WHEN** the daemon attempts startup indexing and no configured downstream server is reachable
- **THEN** the daemon still reports ready, continues serving the previously persisted catalog, and surfaces that indexing did not refresh the catalog

#### Scenario: Partial startup indexing still serves reachable results

- **WHEN** startup indexing reaches some servers and fails others
- **THEN** the daemon becomes ready with the reachable servers' tools indexed and the per-server failures surfaced, rather than aborting startup
