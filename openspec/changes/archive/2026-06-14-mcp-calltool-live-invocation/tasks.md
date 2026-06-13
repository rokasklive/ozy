## 1. Seams and Test Coverage

- [x] 1.1 Extend the `downstream.Session` interface with `CallTool(ctx, *mcpsdk.CallToolParams) (*mcpsdk.CallToolResult, error)` and confirm the MCP SDK client session satisfies it.
- [x] 1.2 Extend the broker's `Connector` interface with a single-server connect method and confirm `downstream.Connector.Connect` satisfies it.
- [x] 1.3 Add broker tests for a successful `callTool`: a valid `toolRef` connects to only the target server, calls `tools/call`, and returns `ok: true` with a normalized result and `resultSummary`, without any catalog contents.
- [x] 1.4 Add broker tests for failure paths: malformed `toolRef` (`TOOL_NOT_FOUND`), unknown/disabled server (`CONFIG_ERROR`), unreachable server (`DOWNSTREAM_SERVER_OFFLINE`), and downstream tool error (`DOWNSTREAM_CALL_FAILED`), each asserting an explicit `agentInstruction` and secret redaction.
- [x] 1.5 Add a broker test proving a result over `budgets.callTool.maxResultBytes` returns a bounded, truncation-aware response.

## 2. Live Invocation Implementation

- [x] 2.1 Back `downstream.Session.CallTool` with the MCP SDK client session's `tools/call`.
- [x] 2.2 Replace `live.CallTool`'s delegation to the skeleton with live invocation: split the `toolRef` on the first `.`, resolve the server from `cfg.MCP`, and reject malformed/unknown/disabled refs with the structured failures from §1.4.
- [x] 2.3 Connect to the single resolved server under a per-call timeout derived from the server's configured timeout, call `tools/call` with the supplied arguments, and close the session afterward.
- [x] 2.4 Normalize the downstream result into the §9.3 `CallResult` success shape (structured content when present, else text content) with a short `resultSummary`; map a downstream error result and connection/transport errors onto §9.3 structured failures with redaction already applied.
- [x] 2.5 Apply the `budgets.callTool.maxResultBytes` bound when set, returning a truncation-aware response; otherwise return the normalized result as-is.
- [x] 2.6 Keep `describeTool` and `List` delegating to the skeleton (unchanged scope).

## 3. Adapter and CLI Parity

- [x] 3.1 Confirm `internal/mcp` `handleCall` and `ozy call` need no surface change and that both render the new `CallResult` / §9.3 failure correctly through the shared broker.
- [x] 3.2 Add an MCP adapter test proving `callTool` over the adapter invokes a fixture downstream tool and returns its normalized result, and that the advertised tool list is still exactly `findTool`, `describeTool`, `callTool`.
- [x] 3.3 Add a CLI test proving `ozy call <toolRef> --json '{...}'` produces the semantically equivalent result to the MCP path.

## 4. Acceptance and Documentation

- [x] 4.1 Add an integration-style test with a fixture downstream MCP server proving the end-to-end story: `findTool` → select a `toolRef` → `callTool` succeeds without running `ozy index`.
- [x] 4.2 Update README/docs to show the `findTool` → `callTool` flow through `ozy mcp`, noting that `describeTool`'s live resolution remains out of scope and the agent uses the `findTool` candidate schema.
- [x] 4.3 Run `go test ./...`, `openspec validate mcp-calltool-live-invocation`, and `graphify update .`.
