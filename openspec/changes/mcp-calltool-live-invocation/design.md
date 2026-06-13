## Context

Ozy's agent-facing surface is the §9 triad: `findTool`, `describeTool`,
`callTool`. The pieces needed for a first working invocation path already exist:

- `ozy mcp` loads config, builds the daemon, and serves the MCP adapter; both the
  adapter (`internal/mcp`) and the CLI (`ozy call`) route `callTool` through
  `broker.CallTool`, so behavior is defined in one place.
- `findTool` already performs **live** discovery: on each call it connects to
  enabled downstream servers, runs `tools/list`, and returns
  `choose_from_candidates` with every tool's `toolRef` (`<serverId>.<name>`),
  `serverId`, downstream name, `title`, `description`, and `inputSchema`.
- The downstream connector (`internal/downstream`) connects to local (stdio) and
  remote (HTTP) servers with per-server isolation, timeouts, structured errors,
  and secret redaction. It exposes `ConnectAll` and a single-server `Connect`.
  Its `Session` interface currently only exposes `ListTools` + `Close`.

The gap: `callTool` is still skeleton-backed. For a catalog-known tool it returns
`NOT_IMPLEMENTED`; for an un-indexed tool it returns `TOOL_NOT_FOUND`. After a
live `findTool` (which does not persist to the catalog), an agent therefore
cannot invoke the tool it just discovered. This change closes that gap.

## Goals / Non-Goals

**Goals:**

- `callTool` resolves a `toolRef` to its downstream server and invokes the tool
  via MCP `tools/call`, returning a `SPEC.md` §9.3 success or structured failure.
- Resolution is **live** (config-driven), so invocation works without `ozy index`
  and is consistent with how `findTool` already behaves.
- Connect to only the **one** server named in the `toolRef`, not every server.
- Reuse existing per-server isolation, timeout, redaction, and error mapping.
- Keep MCP and CLI in parity by implementing this once behind `broker.CallTool`.
- Preserve the small MCP surface: still exactly three advertised tools.

**Non-Goals:**

- Live (catalog-free) `describeTool`. The agent uses the schema/description that
  `findTool` already returns in each candidate.
- Argument validation against the downstream JSON Schema. Argument shape is
  passed through; the downstream server remains the validation authority for now.
- Catalog persistence, ranking, caching of sessions, or a daemon/client split.
- Stale-schema rejection (`TOOL_SCHEMA_CHANGED`) and auth flows beyond the
  existing redacted `AUTH_UNAVAILABLE` mapping.

## Decisions

### Implement invocation in the live broker, not the skeleton

`live.CallTool` currently delegates to `skeleton.CallTool` (which returns
`NOT_IMPLEMENTED`). It will instead perform real invocation, while
`describeTool` and `List` keep delegating to the skeleton. This mirrors the
existing pattern where `live.FindTool` is real and the rest is delegated, and
keeps the change localized to one method.

_Alternative considered:_ implement in the skeleton. Rejected — the skeleton is
deliberately the catalog-only, no-downstream-I/O placeholder; invocation needs a
config + connector, which the live broker already holds.

### Resolve the toolRef live, connect to one server

`toolRef` is `<serverId>.<downstreamToolName>`. The broker splits on the **first**
`.` to get `serverId` and the (possibly dotted) downstream tool name, then looks
up `cfg.MCP[serverId]`:

- unknown or disabled server → structured failure (`CONFIG_ERROR` for
  unknown/disabled config; the agent is told to run `findTool` again);
- malformed `toolRef` (no `.`) → `TOOL_NOT_FOUND` with instruction to discover
  first.

It then connects to that single server via the connector's `Connect` (not
`ConnectAll`), under a per-call timeout derived from the server's configured
timeout, and calls `tools/call`.

_Alternative considered:_ resolve against the catalog. Rejected — it would force
`ozy index` and contradict the live `findTool` story.

_Alternative considered:_ `ConnectAll` then pick the matching result. Rejected —
it pays the cost and failure surface of every configured server for a
single-tool call.

### Extend two seams: `Session.CallTool` and the broker `Connector`

- `downstream.Session` gains
  `CallTool(ctx, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error)`,
  backed by the MCP SDK client session (symmetric to the existing `ListTools`).
  Keeping the SDK behind the `Session` interface preserves the rule that the
  broker never imports the MCP SDK transport directly and tests can inject fakes.
- The broker's `Connector` interface gains a single-server connect method so the
  live broker can reach exactly the target server. `downstream.Connector` already
  satisfies it via its exported `Connect`.

### Map the downstream result onto the §9.3 envelope

On a successful `tools/call`:

- `result.IsError == true` → a structured `DOWNSTREAM_CALL_FAILED` failure
  carrying the downstream message (redacted), `retryable` per the error nature,
  and an instruction to inspect arguments / report rather than blind-retry.
- otherwise → `CallResult{ok:true, toolRef, result, resultSummary}` where
  `result` carries the normalized downstream content (structured content when
  present, else the text content) and `resultSummary` is a short human line.

Connection/transport failures reuse the connector's existing structured errors
(`DOWNSTREAM_SERVER_OFFLINE`, `AUTH_UNAVAILABLE`, `CONFIG_ERROR`) with redaction
already applied, so secrets in headers/env never leak.

### Respect the existing result budget, conservatively

`config.BudgetsConfig.CallTool.MaxResultBytes` already exists. When set (> 0) and
the normalized result exceeds it, the broker returns a `RESULT_TRUNCATED`-aware
response (truncated/summarized payload with an instruction to narrow the call)
rather than streaming an unbounded blob. When unset, the normalized result is
returned as-is. This keeps the first cut correct without inventing new config.

### No adapter or CLI surface change

`internal/mcp.handleCall` and `ozy call` already call `broker.CallTool` and
render `CallResult` / the §9.3 error envelope. They need no change; parity is
automatic because both share the broker seam.

## Risks / Trade-offs

- **Live connect per call adds latency** → reuse the per-server timeout and
  connect to only the one target server; session caching is a deliberate later
  optimization that won't change the MCP surface.
- **No argument validation before dispatch** → downstream server validates and
  Ozy maps its error into `ARGUMENT_VALIDATION_FAILED` / `DOWNSTREAM_CALL_FAILED`
  with a repair instruction; full schema validation is a follow-up.
- **Downstream results can be large or leak detail** → bound with the existing
  `maxResultBytes` budget and run all downstream messages through the connector's
  redaction before surfacing.
- **`toolRef` split ambiguity if a serverId contains `.`** → server ids are config
  keys without dots in practice; splitting on the first `.` is well-defined and an
  unknown server id simply yields a structured `findTool`-again instruction.
- **Retry amplification** → the response states explicitly whether the agent
  should retry; Ozy performs no hidden internal retries on `tools/call`.

## Migration Plan

Additive and behind the existing `callTool` contract shape: previous callers
received a `NOT_IMPLEMENTED` / `TOOL_NOT_FOUND` failure; they now receive a real
result or a more specific structured failure. No config migration. Rollback is
reverting `live.CallTool` to delegate to the skeleton.

## Open Questions

- Should the per-call timeout be the discovery timeout or a separate
  `callTool`-specific budget? Default to the server's configured timeout for now.
- Is `CONFIG_ERROR` vs `TOOL_NOT_FOUND` the right type for a disabled-but-known
  server? Current choice: `CONFIG_ERROR`, since the fix is configuration, not
  rediscovery.
