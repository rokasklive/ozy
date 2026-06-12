## 1. Contract and Test Coverage

- [x] 1.1 Add contract coverage for a `findTool` result that can carry multiple live-discovered downstream tools with `toolRef`, `serverId`, downstream name, description/title, and input schema.
- [x] 1.2 Add broker tests proving non-empty live discovery returns all downstream tools with `decision: choose_from_candidates` and does not require catalog contents.
- [x] 1.3 Add broker tests for zero live tools, partial server failure, and total downstream failure, including structured diagnostics and redaction.
- [x] 1.4 Add MCP adapter tests proving `findTool` is advertised and calling it over the MCP adapter returns the live-discovered tool list.

## 2. Live Discovery Implementation

- [x] 2.1 Add a live-discovery dependency to the broker implementation while keeping the MCP adapter routed through the broker interface.
- [x] 2.2 Reuse the existing downstream connector to connect to enabled configured MCP servers and call `tools/list` during `FindTool`.
- [x] 2.3 Normalize each downstream tool into a stable Ozy candidate entry using `<serverId>.<downstreamToolName>` as `toolRef`.
- [x] 2.4 Preserve existing placeholder behavior for `describeTool` and `callTool`.

## 3. Runtime Wiring

- [x] 3.1 Update daemon construction so the shared broker receives the resolved Ozy config and downstream connector needed for live discovery.
- [x] 3.2 Ensure `ozy mcp` loads config once at startup and exposes a broker-backed `findTool` to MCP clients without requiring `ozy index`.
- [x] 3.3 Ensure downstream sessions opened during `findTool` are closed after discovery.

## 4. Acceptance and Documentation

- [x] 4.1 Add an integration-style CLI/MCP test with a fixture downstream MCP server proving an agent client can list Ozy tools and call `findTool`.
- [x] 4.2 Update README or relevant docs with the minimal opencode-compatible configuration flow for running `ozy mcp`.
- [x] 4.3 Run `go test ./...`, `openspec validate mcp-findtool-live-discovery`, and `graphify update .`.
