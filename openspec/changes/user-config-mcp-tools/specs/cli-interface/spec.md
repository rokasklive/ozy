## ADDED Requirements

### Requirement: CLI exposes tools from explicit MCP configuration
The Ozy CLI SHALL load an explicit opencode-shaped MCP config path, index
reachable configured MCP servers, and expose the resulting tools through
broker-backed CLI commands.

#### Scenario: Indexing tools from the example config
- **WHEN** a user runs `ozy --config examples/test_mcp_examples.jsonc index
  --format json` in an environment where the enabled configured server commands
  are available and reachable
- **THEN** Ozy connects to the enabled MCP servers, calls `tools/list`, persists
  discovered tool metadata, and emits a JSON summary containing reached server
  and indexed tool counts plus any per-server failures

#### Scenario: Listing indexed tools from the CLI
- **WHEN** tools have been indexed from an explicit MCP config path and a user
  runs `ozy --config examples/test_mcp_examples.jsonc list --format json`
- **THEN** the CLI returns a JSON result containing the discovered toolRefs with
  their server ids and freshness status

#### Scenario: Describing an indexed tool from the CLI
- **WHEN** a toolRef returned by `ozy list` was discovered from the explicit MCP
  config path and a user runs `ozy --config examples/test_mcp_examples.jsonc
  describe <toolRef> --format json`
- **THEN** the CLI returns that tool's name, description, input schema, server
  status, freshness, and usage guidance instead of `TOOL_NOT_FOUND`

#### Scenario: Search uses the populated catalog
- **WHEN** at least one tool has been indexed from an explicit MCP config path
  and a user runs `ozy --config examples/test_mcp_examples.jsonc search <query>
  --format json`
- **THEN** the CLI returns a broker decision derived from the populated catalog
  rather than the `catalog_empty` decision
