## REMOVED Requirements

### Requirement: MCP findTool performs live downstream discovery

**Reason**: This requirement describes superseded behavior. `findTool` has ranked the persistent catalog since the hybrid-search change; live discovery happens at index time. Keeping the live-discovery contract in force made the spec (and the README derived from it) assert behavior the runtime does not have — the audit's "lying interface" root at spec level.
**Migration**: Catalog-backed `findTool` behavior is specified in `tool-search`; live discovery at index time in `tool-discovery`; re-index cache invalidation is preserved by "MCP findTool serves catalog-backed decisions" below and by `result-cache`.

## ADDED Requirements

### Requirement: MCP findTool serves catalog-backed decisions

When an MCP client calls `findTool`, the adapter SHALL return the shared broker's ranked decision over the persistent catalog (per `tool-search`), without connecting to downstream servers during the call. When result caching is enabled, a `findTool` response MAY be served from the broker-level result cache within its TTL; the cache SHALL be invalidated when the catalog is re-indexed so a cached result never masks a newly indexed or removed tool.

#### Scenario: findTool answers from the catalog without live connections

- **WHEN** an MCP client calls `findTool` while downstream servers are slow or unreachable
- **THEN** the adapter returns the catalog-backed ranked decision without attempting downstream connections for that call

#### Scenario: Cached findTool is invalidated by re-index

- **WHEN** result caching is enabled, a `findTool` query is served from cache, and the catalog is then re-indexed
- **THEN** the next identical `findTool` call is recomputed against the freshly indexed catalog rather than served from the stale entry

### Requirement: findTool failures return the error envelope

When the broker's `FindTool` returns an error, the adapter SHALL return the §9.3 `{ok: false, error}` envelope with the MCP result marked as an error — exactly as `describeTool` and `callTool` already do. The adapter SHALL NOT emit a null or empty body with a success flag.

#### Scenario: A broker failure is a labeled error, not a null success

- **WHEN** an MCP client calls `findTool` and the broker fails (for example the catalog store cannot be read)
- **THEN** the response content is a §9.3 error envelope with a structured `error.type` and `agentInstruction`, the MCP `isError` flag is set, and the content is never the literal `null`

### Requirement: Actionable guidance is delivered in-band

Any notice the agent must act on — truncation recovery, cache-hit staleness, degraded-mode warnings attached to a call result — SHALL be delivered inside the MCP `content` (as a short, clearly marked trailing text block separate from the payload block), because major MCP clients do not surface `_meta` to the model. `_meta` MAY continue to mirror `toolRef`, `resultSummary`, and machine-readable metadata for clients that read it, but SHALL NOT be the only carrier of any actionable instruction. The payload content block SHALL remain byte-identical to the normalized result, unpolluted by the notice.

#### Scenario: Truncation recovery arrives in content

- **WHEN** a `callTool` result is truncated under `budgets.callTool.maxResultBytes`
- **THEN** the response content includes a short trailing notice naming the truncation and how to recover (narrow the call or raise the budget), in addition to any `_meta` mirror

#### Scenario: The payload block stays pristine

- **WHEN** a notice accompanies a `callTool` result
- **THEN** the notice is a separate trailing text block and the payload text block is byte-identical to the normalized downstream result

### Requirement: MCP initialize advertises usage instructions

The `ozy mcp` server SHALL set the MCP server `instructions` field at initialize: brief guidance on when to reach for `findTool` (capabilities beyond the agent's built-ins, before broad shell exploration) and a bounded summary of the configured downstream servers, honoring `surface.capabilityBreadcrumb`. Instructions SHALL stay short (always-loaded context is paid on every turn).

#### Scenario: Initialize carries when-to-use guidance

- **WHEN** an MCP client completes the initialize handshake with `ozy mcp`
- **THEN** the server's `instructions` field contains when-to-use guidance and the downstream server summary, and is present in the handshake result

#### Scenario: Breadcrumb setting is honored

- **WHEN** `surface.capabilityBreadcrumb` is disabled in configuration
- **THEN** the initialize instructions omit the downstream server list while retaining the brief when-to-use guidance

## MODIFIED Requirements

### Requirement: Agent-facing tool registration

The MCP adapter SHALL expose exactly the stable agent-facing tools defined in `SPEC.md` §4.3 and §9 — `findTool`, `describeTool`, and `callTool` — and SHALL NOT expose downstream tools directly, preserving the small-surface and capability-brokerage principles. Each tool's description SHALL promise only what its responses actually deliver: `describeTool`'s description SHALL match its payload, and the fields it advertises (such as a recommended call shape) SHALL be populated in its responses.

#### Scenario: Adapter advertises the three stable tools

- **WHEN** an MCP client connects to `ozy mcp` and lists tools
- **THEN** it sees exactly `findTool`, `describeTool`, and `callTool` with their input schemas, and no downstream tools

#### Scenario: Adapter starts over the MCP transport

- **WHEN** a user runs `ozy mcp`
- **THEN** the adapter serves the MCP protocol over its transport and is connectable by a standard MCP client

#### Scenario: describeTool delivers what its description promises

- **WHEN** an agent calls `describeTool` for a cataloged tool
- **THEN** the response includes the exact input schema and a populated `recommendedCall` (a `callTool` shape with an argument skeleton from the schema's required fields), and the tool description does not advertise fields the response omits
