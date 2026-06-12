## Why

Users should be able to configure `ozy mcp` as an MCP server in opencode or any
other agent harness and immediately see an Ozy tool they can call. Today the MCP
adapter registers placeholder tools, but `findTool` does not yet resolve tools
from the MCP servers configured in Ozy's config, so the first agent-facing path
does not prove the broker value.

## What Changes

- Make `ozy mcp` expose a working `findTool` tool over the MCP adapter.
- Keep the scope narrow: `describeTool` and `callTool` remain deferred.
- When an agent calls `findTool`, Ozy loads the configured downstream MCP
  servers, connects to enabled servers, calls `tools/list`, and returns the
  complete discovered tool list.
- For this feature, `findTool` intentionally does not rank, filter, or require a
  pre-built index; it returns all live-discovered tools from configured servers.
- Preserve the small Ozy MCP surface: agents see `findTool`, not every downstream
  tool as top-level MCP tools.
- Return structured, instructional errors when config loading, downstream
  connection, or downstream `tools/list` fails.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `mcp-adapter`: `findTool` changes from placeholder/catalog-empty behavior to
  live downstream tool discovery when called through `ozy mcp`.

## Impact

- Affected code:
  - `internal/mcp`: adapter registration, `findTool` handler, MCP response shape.
  - `internal/cli`: `ozy mcp` startup wiring so the adapter has access to loaded
    config and live discovery dependencies.
  - `internal/downstream`: reuse existing connection and `ListTools` behavior.
  - `internal/contract` or `internal/broker`: add or adapt the response model for
    returning all live-discovered tools from `findTool`.
- Affected behavior:
  - Agents can configure Ozy as an MCP server and call `findTool` without running
    `ozy index` first.
  - The first working MCP path depends on the same Ozy config file and downstream
    MCP server definitions used elsewhere.
- Dependencies:
  - No new external dependency is expected.
