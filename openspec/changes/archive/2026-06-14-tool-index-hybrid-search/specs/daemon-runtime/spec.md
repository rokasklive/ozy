## ADDED Requirements

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
