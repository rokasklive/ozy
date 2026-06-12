# tool-discovery

## Purpose

Define how Ozy discovers tools from connected downstream servers via
`tools/list`, normalizes them into stable `toolRef`s, populates the
catalog, and serves discovered tools through `list` and `describe`.

## Requirements

### Requirement: Discover tools via tools/list

Ozy SHALL discover the tools of each connected downstream server by calling MCP `tools/list`, capturing each tool's name, title, description, and input schema.

#### Scenario: Tools are retrieved from a connected server

- **WHEN** Ozy has an initialized session to a downstream server that exposes tools
- **THEN** it retrieves the server's tool list including each tool's name, description, and input schema

### Requirement: Stable toolRef normalization

Ozy SHALL normalize each discovered tool into a stable reference of the form `<serverId>.<downstreamToolName>` (SPEC.md §8), recording `serverId`, `downstreamToolName`, title, description, and input schema.

#### Scenario: A discovered tool gets a stable toolRef

- **WHEN** server `atlassian` exposes a tool named `confluence_search`
- **THEN** Ozy catalogs it under `toolRef` `atlassian.confluence_search` with its server id, downstream name, and schema

### Requirement: `ozy index` populates the catalog

The `ozy index` command SHALL connect to configured servers, discover their tools, write the normalized tools to the catalog, and report a summary of servers indexed and tools discovered (including per-server failures).

#### Scenario: Indexing reports a summary

- **WHEN** a user runs `ozy index` with at least one reachable configured server
- **THEN** the catalog is populated with the discovered tools and the command reports how many servers were reached and how many tools were indexed, plus any per-server errors

#### Scenario: Indexing with no reachable servers is instructional

- **WHEN** `ozy index` runs but no configured server is reachable
- **THEN** it reports the per-server failures with repair guidance rather than silently succeeding

### Requirement: Discovered tools carry freshness and runtime status

Each cataloged tool SHALL record `lastIndexedAt`, a `schemaHash`, a freshness marker, and the server's runtime status at index time (SPEC.md §8, §8.1).

#### Scenario: A freshly indexed tool is marked fresh

- **WHEN** a tool is discovered from a reachable server during `ozy index`
- **THEN** its catalog entry records `lastIndexedAt`, a schema hash, freshness `fresh`, and the server status

### Requirement: List and describe reflect discovered tools

After indexing, `ozy list` SHALL list the discovered tools and `describeTool` / `ozy describe` SHALL return the exact input schema and status for a known `toolRef`, drawing from the catalog.

#### Scenario: describe returns a discovered tool's schema

- **WHEN** a tool has been indexed and a user runs `ozy describe <toolRef>` for it
- **THEN** Ozy returns that tool's exact input schema and status instead of `TOOL_NOT_FOUND`

#### Scenario: list shows indexed tools

- **WHEN** tools have been indexed and a user runs `ozy list`
- **THEN** the discovered tools appear with their server id and freshness
