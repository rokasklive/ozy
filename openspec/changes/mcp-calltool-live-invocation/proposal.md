## Why

`ozy mcp` now exposes a working `findTool` that lists tools live from configured
downstream MCP servers, but `callTool` still returns a `NOT_IMPLEMENTED`
placeholder. An agent can discover a tool and read its schema, yet cannot
actually use it through Ozy, so the broker's core promise — invoke a downstream
tool without exposing every downstream tool — is unproven end to end.

## What Changes

- Make `callTool` perform live brokered invocation: resolve the `toolRef` to a
  configured downstream server, connect, call downstream MCP `tools/call`, and
  return a `SPEC.md` §9.3-shaped success or structured failure.
- Keep the scope narrow and controlled — this introduces exactly one new
  capability (invocation). `findTool` (live discovery) is unchanged.
- Resolve `toolRef`s on the **live** path, consistent with live `findTool`, so an
  agent can call a tool it just discovered **without** running `ozy index` first.
- Normalize the downstream result and surface obvious next-action guidance when
  available; preserve downstream error detail with secrets redacted.
- Return structured, instructional `§9.3` failures for malformed `toolRef`s,
  unknown/disabled servers, unreachable servers, and downstream `tools/call`
  errors, each stating whether the agent should retry, choose another tool, ask
  the user, or report the failure.
- `describeTool`'s live (catalog-free) resolution is explicitly **out of scope**:
  the agent obtains a tool's definition/description and input schema from the
  `findTool` candidate payload, which already carries `title`, `description`, and
  `inputSchema`.

## Capabilities

### New Capabilities
- `tool-invocation`: Ozy invokes a selected downstream MCP tool via `tools/call`
  through the broker seam — resolving the `toolRef`, connecting to the one target
  server, passing validated arguments, normalizing the result, and emitting
  §9.3 success/failure envelopes with redaction and per-call timeouts.

### Modified Capabilities
- `mcp-adapter`: `callTool` changes from a `NOT_IMPLEMENTED` placeholder into a
  broker-routed live invocation path; the adapter still advertises only
  `findTool`, `describeTool`, and `callTool` and never registers downstream tools
  as top-level MCP tools.

## Impact

- Affected code:
  - `internal/broker`: implement live `CallTool` (currently delegated to the
    skeleton's `NOT_IMPLEMENTED`); extend the broker's `Connector` seam to connect
    to a single target server.
  - `internal/downstream`: add `CallTool` to the `Session` interface and back it
    with the MCP SDK client session's `tools/call`; reuse existing per-server
    isolation, timeout, and redaction.
  - `internal/mcp` and `internal/cli`: no surface changes — both already route
    `callTool` / `ozy call` through `broker.CallTool`; behavior changes underneath.
  - `internal/contract`: reuse the existing `CallResult` and §9.3 `Error` set; no
    new error type beyond retiring `NOT_IMPLEMENTED` for `callTool`.
- Affected behavior:
  - Agents can complete `findTool` → pick a tool → `callTool` end to end against
    configured downstream servers without `ozy index`.
  - `ozy call <toolRef> --json '{...}'` performs the same live invocation as the
    MCP path (adapter parity).
- Dependencies:
  - No new external dependency. Uses the already-vendored MCP Go SDK.
