## MODIFIED Requirements

### Requirement: Instructional placeholder responses conform to contracts

Agent-facing tools whose live behavior remains out of scope SHALL return placeholder responses whose shape already conforms to the corresponding `SPEC.md` §9 contract. `describeTool` SHALL keep returning schema/recommended-call shaped placeholder or catalog-backed responses. `findTool` is no longer a placeholder: it SHALL return live downstream discovery results as specified by the live discovery requirement. `callTool` is no longer a placeholder: it SHALL perform live brokered invocation as specified by the live invocation requirement, returning a §9.3 success envelope on success and a §9.3 structured failure only on an actual resolution, connection, or downstream error.

#### Scenario: findTool returns a live discovery result

- **WHEN** a client calls `findTool` after live discovery is implemented
- **THEN** the response reflects live downstream discovery from configured MCP servers instead of an unconditional skeleton `catalog_empty` placeholder

#### Scenario: callTool returns a live result or a contract-shaped failure

- **WHEN** a client calls `callTool` with a `toolRef` and arguments
- **THEN** the response is either a §9.3 success with `ok: true` and a normalized result, or a §9.3 structured failure with `ok: false`, an `error.type`, and an `agentInstruction`, and it is never an unconditional `NOT_IMPLEMENTED` placeholder

#### Scenario: describeTool keeps its catalog-backed placeholder behavior

- **WHEN** a client calls `describeTool` for a `toolRef` that is not in the catalog
- **THEN** the response is a structured `TOOL_NOT_FOUND` failure directing the agent to discover first, because catalog-free `describeTool` resolution is out of scope for this change

## ADDED Requirements

### Requirement: MCP callTool performs live brokered invocation

The MCP adapter SHALL provide a working `callTool` path that, when an MCP client calls `callTool`, resolves the `toolRef` against Ozy's resolved configuration, invokes the downstream tool via MCP `tools/call`, and returns the normalized §9.3 result — without requiring `ozy index` to have been run first. The adapter SHALL route the invocation through the shared broker interface and SHALL NOT expose downstream tools as top-level MCP tools.

#### Scenario: Harness invokes a discovered tool through Ozy

- **WHEN** a user configures `ozy mcp` as an MCP server, calls `findTool`, selects a returned `toolRef`, and then calls `callTool` with that `toolRef` and arguments
- **THEN** Ozy invokes the corresponding downstream tool and returns its normalized result without requiring `ozy index`

#### Scenario: callTool stays behind the broker seam

- **WHEN** an agent calls `callTool` through the adapter
- **THEN** the adapter delegates to the shared broker `CallTool` rather than a separate MCP-only implementation, so the CLI `ozy call` and MCP paths produce semantically equivalent results

#### Scenario: callTool does not enlarge the MCP surface

- **WHEN** a client lists Ozy's tools while live invocation is available
- **THEN** it still sees exactly `findTool`, `describeTool`, and `callTool`, and no downstream tools are registered as top-level MCP tools
