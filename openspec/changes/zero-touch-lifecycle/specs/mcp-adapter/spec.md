## ADDED Requirements

### Requirement: MCP serving self-provisions the runtime

When `ozy mcp` starts, the adapter SHALL run the runtime startup sequence — provisioning the embedding sidecar when semantic search is enabled and conditionally indexing a stale catalog — so that semantic search is available through the MCP surface without the user running any separate command such as `ozy daemon` or `ozy index`. The adapter SHALL NOT require a prior manual index or a separate daemon process to serve semantic results.

#### Scenario: Serving a never-indexed catalog provisions and indexes on start

- **WHEN** a user configures `ozy mcp` in an agent harness and starts it with semantic search enabled and a catalog that has never been indexed
- **THEN** the adapter provisions the embedding sidecar, runs an index pass that populates the catalog and embeds the indexed tools, and then serves semantic-ranked `findTool` results
- **AND** the user runs no other ozy command to make this happen

#### Scenario: Serving a fresh catalog skips reindexing

- **WHEN** `ozy mcp` starts and the catalog's last successful index time is at or after the configuration file's modification time
- **THEN** the adapter does not re-index on startup and serves the existing catalog and vectors

#### Scenario: Semantic provider is wired when the sidecar is healthy

- **WHEN** `ozy mcp` starts with semantic search enabled and the sidecar provisions and passes its readiness check
- **THEN** the adapter wires the semantic provider into the broker so `findTool` ranks with hybrid lexical+semantic results rather than the lexical baseline

### Requirement: Embedding sidecar lifetime is bound to the MCP connection

The adapter SHALL keep the provisioned embedding sidecar running for the lifetime of the MCP stdio connection and SHALL shut it down when that connection ends — client disconnect (EOF) or an interrupt/termination signal — leaving no orphaned sidecar process. The user SHALL NOT have to start or stop the sidecar.

#### Scenario: Sidecar is shut down when the agent disconnects

- **WHEN** the MCP client closes the connection or the `ozy mcp` process receives an interrupt or termination signal while the sidecar is running
- **THEN** the adapter shuts the sidecar down as part of its own shutdown, leaving no orphaned process

#### Scenario: Sidecar stays warm across calls within one session

- **WHEN** an agent issues multiple `findTool` calls over a single MCP connection
- **THEN** the adapter reuses the already-provisioned warm sidecar rather than re-provisioning per call

### Requirement: MCP readiness does not block the protocol handshake

Provisioning the sidecar and indexing — including a first-run cold embedding-model download — SHALL NOT prevent the adapter from completing the MCP initialize handshake and answering tool calls. Until the semantic provider is ready, the adapter SHALL serve the lexical baseline and surface a warming/degraded state; when readiness completes it SHALL upgrade subsequent `findTool` results to hybrid semantic ranking.

#### Scenario: Cold first-run start stays connectable

- **WHEN** an agent starts `ozy mcp` for the first time and provisioning must download the embedding model
- **THEN** the MCP initialize handshake completes promptly and `findTool` answers from the lexical baseline while the model downloads
- **AND** once the sidecar becomes ready, later `findTool` calls in the same session return hybrid semantic results

#### Scenario: Degraded semantic is surfaced, not failed

- **WHEN** semantic search is enabled but the sidecar cannot be provisioned or stays unhealthy
- **THEN** the adapter still serves `findTool` from the lexical baseline and surfaces that semantic search is unavailable rather than failing the connection
