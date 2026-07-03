## MODIFIED Requirements

### Requirement: CLI command surface

The `ozy` CLI SHALL expose the command surface `init`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, and `eval run`. Each command SHALL be registered with help text describing its purpose. The CLI SHALL NOT expose a `daemon` command: the runtime that serving requires is hosted by `ozy mcp`, which self-provisions, so no standalone daemon process exists for the user to run or manage.

#### Scenario: All commands are registered

- **WHEN** a user runs `ozy --help`
- **THEN** all of `init`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, and `eval` appear with usage descriptions
- **AND** no `daemon` command is listed

#### Scenario: Command-specific help

- **WHEN** a user runs `ozy search --help`
- **THEN** the command's expected arguments and flags are described

## REMOVED Requirements

### Requirement: Daemon command

**Reason**: The standalone `ozy daemon` command provisioned the sidecar and held it warm but served no transport; with `ozy mcp` now self-provisioning and binding the sidecar to its own connection lifetime, the command has no remaining job and was a source of operational confusion (the README told users to run it alongside `ozy mcp`).

**Migration**: Remove `ozy daemon` from any startup scripts or harness configuration. Configure `ozy mcp` as the MCP server; it now provisions the embedding sidecar and indexes automatically on start. For a one-off manual catalog/embedding refresh outside a serving session, use `ozy index`.

## ADDED Requirements

### Requirement: Index and doctor are optional, not setup steps

Normal operation through the MCP adapter SHALL require neither `ozy index` nor `ozy doctor`. `ozy index` SHALL remain available as a manual reindex/escape hatch and `ozy doctor` as a diagnostic, but a user who only configures `ozy mcp` SHALL get automatic indexing and embedding without invoking either.

#### Scenario: Semantic works without running index or doctor

- **WHEN** a user configures only `ozy mcp` in their agent harness and never runs `ozy index` or `ozy doctor`
- **THEN** starting the agent yields a populated, embedded catalog and hybrid semantic `findTool` results

#### Scenario: Index remains available as a manual refresh

- **WHEN** a user runs `ozy index`
- **THEN** the catalog is refreshed and embedded as before, unchanged by this capability

### Requirement: CLI semantic search is automatic

Broker-backed CLI commands that rank tools through the search engine — at minimum `ozy search` — SHALL serve hybrid semantic results automatically when semantic search is enabled, by provisioning the embedding sidecar on demand and running a conditional index, without a prior `ozy index`, without a daemon, and without any flag. The command SHALL release the on-demand sidecar when it completes, and SHALL degrade to lexical results (surfacing the degraded mode) when the sidecar cannot be provisioned. Catalog-only commands (`list`, `describe`, `call`) SHALL NOT provision the sidecar.

#### Scenario: Search returns semantic results with no setup

- **WHEN** a user who has only configured `ozy.jsonc` runs `ozy search <query>` with semantic search enabled and no prior `ozy index` or daemon
- **THEN** the command provisions the sidecar on demand, ensures the catalog is indexed and embedded, and returns hybrid semantic-ranked results, then releases the sidecar

#### Scenario: Search degrades to lexical when the sidecar is unavailable

- **WHEN** a user runs `ozy search <query>` with semantic enabled but the sidecar cannot be provisioned
- **THEN** the command returns lexical-ranked results and surfaces that semantic search is unavailable rather than failing

#### Scenario: Catalog-only commands do not provision

- **WHEN** a user runs `ozy list`, `ozy describe`, or `ozy call`
- **THEN** the command serves from the catalog and live connections without provisioning the embedding sidecar

### Requirement: Doctor reports embedding coverage drift

`ozy doctor` SHALL compare the queryable vector count against the catalog tool count and report a warning when semantic search is enabled, the sidecar is available, and the vector count is below the catalog tool count, so a partial or stale embed is visible rather than presented as two independent healthy checks.

#### Scenario: Partial embed is flagged

- **WHEN** `ozy doctor` runs with semantic enabled, the sidecar healthy, and the catalog holding more tools than the embedding store holds vectors
- **THEN** the embedding check reports a warning naming the catalog and vector counts and the remediation (`ozy index`), rather than reporting both counts as independently OK

#### Scenario: Full coverage is healthy

- **WHEN** `ozy doctor` runs and the vector count is at or above the catalog tool count
- **THEN** the embedding check reports OK
