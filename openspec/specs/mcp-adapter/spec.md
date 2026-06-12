# mcp-adapter

## Purpose

Define Ozy's agent-facing MCP surface (`SPEC.md` §4.3, §9): registering exactly
the three stable tools (`findTool`, `describeTool`, `callTool`) and no downstream
tools, performing live downstream tool discovery on `findTool`, returning
placeholder responses for deferred operations, and delegating to the shared
broker so MCP and CLI stay equivalent.

## Requirements

### Requirement: Agent-facing tool registration

The MCP adapter SHALL expose exactly the stable agent-facing tools defined in `SPEC.md` §4.3 and §9 — `findTool`, `describeTool`, and `callTool` — and SHALL NOT expose downstream tools directly, preserving the small-surface and capability-brokerage principles.

#### Scenario: Adapter advertises the three stable tools

- **WHEN** an MCP client connects to `ozy mcp` and lists tools
- **THEN** it sees exactly `findTool`, `describeTool`, and `callTool` with their input schemas, and no downstream tools

#### Scenario: Adapter starts over the MCP transport

- **WHEN** a user runs `ozy mcp`
- **THEN** the adapter serves the MCP protocol over its transport and is connectable by a standard MCP client

### Requirement: Instructional placeholder responses conform to contracts

Agent-facing tools whose live behavior remains out of scope SHALL return placeholder responses whose shape already conforms to the corresponding `SPEC.md` §9 contract. `describeTool` SHALL keep returning schema/recommended-call shaped placeholder or catalog-backed responses, and `callTool` SHALL keep returning `ok`/error/`agentInstruction` shaped failures. `findTool` is no longer a placeholder: it SHALL return live downstream discovery results as specified by the live discovery requirement.

#### Scenario: findTool returns a live discovery result

- **WHEN** a client calls `findTool` after live discovery is implemented
- **THEN** the response reflects live downstream discovery from configured MCP servers instead of an unconditional skeleton `catalog_empty` placeholder

#### Scenario: callTool returns a contract-shaped failure

- **WHEN** a client calls `callTool` before invocation is implemented
- **THEN** the response is a structured failure with `ok: false`, an `error.type`, and an `agentInstruction` that states whether to retry, choose an alternative, or report the failure, per §9.3

### Requirement: MCP adapter shares the broker seam

The MCP adapter SHALL route every tool invocation through the shared broker interface used by the CLI, so MCP and CLI paths produce semantically equivalent results (`SPEC.md` §4.9, §14.1 adapter parity).

#### Scenario: Adapter delegates to the shared broker

- **WHEN** an agent calls `findTool`, `describeTool`, or `callTool` through the adapter
- **THEN** the adapter delegates to the shared broker interface rather than a separate MCP-only implementation

### Requirement: MCP findTool performs live downstream discovery

The MCP adapter SHALL provide a working `findTool` path that discovers tools from enabled downstream MCP servers configured in Ozy's resolved configuration when an MCP client calls `findTool`.

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
