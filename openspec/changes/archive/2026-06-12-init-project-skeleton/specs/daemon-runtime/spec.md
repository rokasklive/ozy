## ADDED Requirements

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
