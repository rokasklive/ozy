## MODIFIED Requirements

### Requirement: MCP findTool performs live downstream discovery

The MCP adapter SHALL provide a working `findTool` path that discovers tools from enabled downstream MCP servers configured in Ozy's resolved configuration when an MCP client calls `findTool`. When result caching is enabled, a `findTool` response MAY be served from the broker-level result cache within its TTL; the cache SHALL be invalidated when the catalog is re-indexed so a cached result never masks a newly indexed or removed tool.

#### Scenario: Harness sees findTool on Ozy MCP

- **WHEN** a user configures `ozy mcp` as an MCP server in opencode or another MCP client and the client lists Ozy's tools
- **THEN** the client sees `findTool` as an available Ozy tool

#### Scenario: findTool returns all live-discovered tools

- **WHEN** an MCP client calls `findTool` and at least one configured downstream MCP server returns tools from `tools/list`
- **THEN** the response includes every tool discovered from every successful enabled downstream server, including each tool's Ozy `toolRef`, source `serverId`, downstream tool name, title or description when provided, and input schema when provided
- **AND** the response does not require `ozy index` to have been run first

#### Scenario: findTool keeps downstream tools as data

- **WHEN** Ozy discovers downstream tools for a `findTool` call
- **THEN** Ozy returns those tools in the `findTool` result payload
- **AND** Ozy does not register those downstream tools as top-level tools on the Ozy MCP server

#### Scenario: findTool reports empty live discovery

- **WHEN** an MCP client calls `findTool` and all enabled downstream servers are reachable but return zero tools
- **THEN** the response uses an explicit empty result decision and instructs the agent to check downstream server configuration or capabilities rather than inventing a tool

#### Scenario: findTool reports partial downstream failures

- **WHEN** an MCP client calls `findTool` and at least one configured downstream server returns tools while another configured downstream server fails
- **THEN** the response includes tools from the successful servers
- **AND** the response includes structured diagnostics for the failed servers with secrets redacted
- **AND** the agent instruction states that the tool list is partial

#### Scenario: findTool reports total downstream failure

- **WHEN** an MCP client calls `findTool` and no enabled downstream server returns tools because all connection or `tools/list` attempts failed
- **THEN** the response is a structured failure or failure decision with per-server diagnostics and repair-oriented agent instruction
- **AND** the response does not report `catalog_empty`

#### Scenario: Cached findTool is invalidated by re-index

- **WHEN** result caching is enabled, a `findTool` query is served from cache, and the catalog is then re-indexed
- **THEN** the next identical `findTool` call is recomputed against the freshly indexed catalog rather than served from the stale entry

### Requirement: MCP callTool performs live brokered invocation

The MCP adapter SHALL provide a working `callTool` path that, when an MCP client calls `callTool`, resolves the `toolRef` against Ozy's resolved configuration, invokes the downstream tool via MCP `tools/call`, and returns the normalized §9.3 result — without requiring `ozy index` to have been run first. The adapter SHALL route the invocation through the shared broker interface and SHALL NOT expose downstream tools as top-level MCP tools. When result caching is enabled, a tool whose `readOnlyHint` is positively `true` MAY have its result served from the broker-level result cache within its TTL instead of repeating the downstream `tools/call`; a tool whose read-only intent is `false`, absent, or otherwise unknown SHALL always be invoked live.

#### Scenario: Harness invokes a discovered tool through Ozy

- **WHEN** a user configures `ozy mcp` as an MCP server, calls `findTool`, selects a returned `toolRef`, and then calls `callTool` with that `toolRef` and arguments
- **THEN** Ozy invokes the corresponding downstream tool and returns its normalized result without requiring `ozy index`

#### Scenario: callTool stays behind the broker seam

- **WHEN** an agent calls `callTool` through the adapter
- **THEN** the adapter delegates to the shared broker `CallTool` rather than a separate MCP-only implementation, so the CLI `ozy call` and MCP paths produce semantically equivalent results

#### Scenario: callTool does not enlarge the MCP surface

- **WHEN** a client lists Ozy's tools while live invocation is available
- **THEN** it still sees exactly `findTool`, `describeTool`, and `callTool`, and no downstream tools are registered as top-level MCP tools

#### Scenario: Write tool always invokes live despite caching

- **WHEN** result caching is enabled and an agent calls `callTool` twice for a tool whose `readOnlyHint` is not positively `true`
- **THEN** each call performs a live downstream `tools/call` and no result is served from the cache
