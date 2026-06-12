# mcp-adapter

## Purpose

Define Ozy's agent-facing MCP surface (`SPEC.md` §4.3, §9): registering exactly
the three stable tools (`findTool`, `describeTool`, `callTool`) and no downstream
tools, returning placeholder responses that already conform to the §9 contracts,
and delegating to the shared broker so MCP and CLI stay equivalent.

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

Until broker behavior is implemented, each agent-facing tool SHALL return a placeholder response whose shape already conforms to the corresponding `SPEC.md` §9 contract (decision/instruction fields for `findTool`, schema/recommended-call fields for `describeTool`, `ok`/error/`agentInstruction` fields for `callTool`), so later changes refine behavior without breaking the response contract.

#### Scenario: findTool returns a contract-shaped result

- **WHEN** a client calls `findTool` against the skeleton
- **THEN** the response includes an explicit `decision` and instructional fields (e.g. `nextAction` or `agentInstruction`) matching the §9.1 shape, including the `catalog_empty` decision when no tools are indexed

#### Scenario: callTool returns a contract-shaped failure

- **WHEN** a client calls `callTool` before invocation is implemented
- **THEN** the response is a structured failure with `ok: false`, an `error.type`, and an `agentInstruction` that states whether to retry, choose an alternative, or report the failure, per §9.3

### Requirement: MCP adapter shares the broker seam

The MCP adapter SHALL route every tool invocation through the shared broker interface used by the CLI, so MCP and CLI paths produce semantically equivalent results (`SPEC.md` §4.9, §14.1 adapter parity).

#### Scenario: Adapter delegates to the shared broker

- **WHEN** an agent calls `findTool`, `describeTool`, or `callTool` through the adapter
- **THEN** the adapter delegates to the shared broker interface rather than a separate MCP-only implementation
