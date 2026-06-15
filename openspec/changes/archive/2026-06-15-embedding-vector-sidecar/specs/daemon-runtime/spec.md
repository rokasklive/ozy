## MODIFIED Requirements

### Requirement: Graceful degradation of optional subsystems

The daemon SHALL operate without semantic search and without the optional Python embedding sidecar, falling back to the lexical baseline, per `SPEC.md` §4.10 and §10.1. When semantic search is enabled but the sidecar cannot be provisioned, started, or stays unhealthy, the daemon SHALL still become ready and serve lexical search, surfacing the degraded mode rather than failing.

#### Scenario: Runs with semantic search disabled

- **WHEN** the daemon starts with semantic search disabled or unavailable
- **THEN** it starts successfully and relies on the lexical/baseline path without requiring the optional sidecar

#### Scenario: Runs lexical-only when the sidecar is enabled but unavailable

- **WHEN** the daemon starts with semantic search enabled but the embedding sidecar cannot be provisioned or started
- **THEN** it still becomes ready, serves `findTool` from the lexical baseline, and surfaces that semantic search is unavailable

## ADDED Requirements

### Requirement: Embedding sidecar supervision

When semantic search is enabled, the daemon SHALL provision (when necessary), launch, health-check, and shut down the embedding sidecar as part of its lifecycle, and SHALL drive embedding upserts during startup indexing and explicit `ozy index`. Sidecar provisioning and supervision SHALL NOT block the daemon from reporting readiness; any failure SHALL degrade to lexical-only.

#### Scenario: Sidecar is started and health-checked when semantic is enabled

- **WHEN** the daemon starts with semantic search enabled and a usable sidecar environment
- **THEN** it launches the sidecar, confirms readiness with a health check, and wires it as the semantic provider for `findTool`

#### Scenario: Sidecar is shut down with the daemon

- **WHEN** the daemon receives an interrupt or termination signal while the sidecar is running
- **THEN** it shuts the sidecar down cleanly as part of its own shutdown rather than leaving an orphaned process

#### Scenario: Sidecar provisioning failure does not block readiness

- **WHEN** the daemon enables semantic search but provisioning or launching the sidecar fails
- **THEN** the daemon still reports ready, serves the existing catalog with lexical search, and surfaces the sidecar failure
