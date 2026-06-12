## Context

Ozy already has the pieces needed for a narrow first working MCP path:

- `ozy mcp` loads Ozy configuration, constructs the daemon, and serves the MCP
  adapter over stdio.
- The MCP adapter registers the stable Ozy tools and delegates calls through the
  broker interface.
- The downstream connector can connect to configured MCP servers and list their
  tools.
- Indexing/search ranking is not implemented yet, so catalog-backed `findTool`
  currently returns instructional placeholder decisions.

The user story for this change is narrower than full search: a user configures
Ozy as an MCP server in opencode or another harness, starts that harness, sees
`findTool`, calls it, and receives the complete list of tools exposed by the MCP
servers configured in Ozy's config.

## Goals / Non-Goals

**Goals:**

- Make `findTool` callable through `ozy mcp` with real downstream tool data.
- Discover tools live from enabled downstream MCP servers configured in Ozy's
  config file.
- Return all discovered tools without ranking or filtering.
- Keep the adapter's small Ozy-facing surface: downstream tools are returned as
  data from `findTool`, not registered as top-level MCP tools.
- Preserve structured, instructional results for empty and failure cases.

**Non-Goals:**

- Implement semantic or lexical ranking.
- Require `ozy index` before `findTool` works.
- Implement brokered `describeTool` or `callTool` behavior.
- Persist live-discovered tools to the catalog as part of `findTool`.
- Add a daemon/client split or background discovery worker.

## Decisions

### Route through the broker seam

`internal/mcp.Adapter.handleFind` will continue calling `broker.FindTool`.
The implementation behind that broker will gain access to the loaded config and
a downstream connector. This keeps the MCP adapter thin and preserves the
existing parity rule that adapter behavior is expressed through the shared broker
interface.

Alternative considered: call downstream discovery directly from `internal/mcp`.
That would be quicker, but it would create MCP-only behavior and weaken the
existing CLI/MCP boundary.

### Live discovery, not catalog lookup

For this feature, `findTool` will connect to the configured downstream servers
on each call, run `tools/list`, and return the discovered tools. It will not
depend on the persistent catalog or `ozy index`.

Alternative considered: reuse the catalog by requiring users to run `ozy index`
first. That contradicts the story: users should configure Ozy as MCP and
immediately call `findTool` from the harness.

### Return candidates, not a selected match

Because ranking is out of scope, a non-empty result will use the existing
decision language as a candidate-list response, for example
`decision: choose_from_candidates`, with a `tools` array containing every
live-discovered tool. Each tool entry should include stable Ozy identifiers
(`toolRef`, `serverId`, downstream tool name), display metadata, and input schema
when available.

Alternative considered: return `no_good_match` with a side-channel list. That is
less useful to agents because the decision says "do not use a tool" even though
tools were found.

### Partial failure is visible

If one configured server fails but others return tools, `findTool` should return
the tools it found plus structured per-server errors or diagnostics and an
instruction that some configured servers failed. If no server returns tools and
there were errors, the result should be a structured failure rather than
`catalog_empty`, because "nothing indexed" and "configured servers failed" are
different agent decisions.

Alternative considered: fail the whole call on the first downstream error. That
would hide useful tools from healthy servers and make one bad config entry block
all discovery.

## Risks / Trade-offs

- Live discovery can be slower than catalog lookup. Mitigation: reuse per-server
  timeout and concurrency behavior already present in the downstream connector.
- MCP clients may call `findTool` frequently. Mitigation: keep this change
  intentionally simple now; caching can be added later without changing the MCP
  surface.
- Returning full schemas can make large responses. Mitigation: this feature
  prioritizes correctness and visibility for the first working path; response
  size controls can be introduced with ranking/indexing later.
- Some configured servers may require auth flows Ozy does not implement yet.
  Mitigation: surface existing structured auth/config errors with redaction and
  clear agent instructions.
