## ADDED Requirements

### Requirement: Resolve toolRef to a downstream server and tool

Ozy SHALL resolve a `callTool` `toolRef` of the form `<serverId>.<downstreamToolName>` (`SPEC.md` §8) to a configured downstream server and downstream tool name, splitting on the first `.` so the server id is the prefix and the downstream tool name is the remainder. Resolution SHALL use Ozy's resolved configuration directly and SHALL NOT require a prior `ozy index`.

#### Scenario: A valid toolRef resolves to its server and tool

- **WHEN** an agent calls `callTool` with `toolRef` `atlassian.confluence_search`
- **THEN** Ozy resolves it to the configured server `atlassian` and downstream tool name `confluence_search` using the resolved configuration without consulting the catalog

#### Scenario: A malformed toolRef is rejected instructionally

- **WHEN** an agent calls `callTool` with a `toolRef` that has no `.` separator
- **THEN** Ozy returns a structured `TOOL_NOT_FOUND` failure whose `agentInstruction` tells the agent to call `findTool` to discover a valid `toolRef` before invoking

#### Scenario: An unknown or disabled server is rejected instructionally

- **WHEN** an agent calls `callTool` with a `toolRef` whose server id is not present in the configuration or is disabled
- **THEN** Ozy returns a structured `CONFIG_ERROR` failure that names the server and instructs the agent to call `findTool` again or fix the server configuration rather than retrying the same call

### Requirement: Invoke the downstream tool via tools/call

Ozy SHALL invoke the resolved tool by connecting to only the one target downstream server and calling MCP `tools/call` with the agent-supplied arguments, applying a per-call timeout derived from the server's configured timeout. Ozy SHALL NOT connect to other configured servers to satisfy a single `callTool`.

#### Scenario: A reachable tool is invoked and returns a success result

- **WHEN** an agent calls `callTool` for a tool on a reachable, enabled server with valid arguments
- **THEN** Ozy connects to that one server, calls downstream `tools/call`, and returns a response with `ok: true`, the `toolRef`, the normalized downstream result, and a short `resultSummary`

#### Scenario: An unreachable server yields a structured failure

- **WHEN** an agent calls `callTool` for a tool whose downstream server cannot be connected
- **THEN** Ozy returns a structured failure with a §9.3 `error.type` such as `DOWNSTREAM_SERVER_OFFLINE`, `retryable` set appropriately, and an `agentInstruction` stating whether to retry, choose another tool, or report the failure

#### Scenario: Only the target server is contacted

- **WHEN** an agent calls `callTool` for a tool on server `atlassian` while other servers are also configured
- **THEN** Ozy attempts a connection only to `atlassian` and does not connect to the other configured servers for that call

### Requirement: Normalize results and downstream errors

Ozy SHALL normalize a successful `tools/call` result into the §9.3 `callTool` success shape, and SHALL map a downstream-reported tool error into a structured §9.3 failure. Downstream error detail SHALL be preserved where useful but secret values from `headers` or `environment` SHALL NOT be leaked.

#### Scenario: Downstream tool error becomes a structured failure

- **WHEN** the downstream `tools/call` returns a tool-level error result
- **THEN** Ozy returns a structured failure with a §9.3 `error.type` (for example `DOWNSTREAM_CALL_FAILED`) carrying the downstream message and an `agentInstruction`, instead of reporting `ok: true`

#### Scenario: Secrets are redacted from invocation errors

- **WHEN** a `callTool` invocation fails for a server that uses a secret header or environment value
- **THEN** the reported error names the server and reason without including the resolved secret value

#### Scenario: Oversized results respect the configured budget

- **WHEN** a `callTool` result exceeds the configured `budgets.callTool.maxResultBytes`
- **THEN** Ozy returns a bounded, truncation-aware response that signals the result was truncated and instructs the agent to narrow the call rather than returning an unbounded payload

### Requirement: Invocation does not amplify retries

Ozy SHALL state in each `callTool` response whether agent-side retry is recommended, and SHALL NOT perform hidden internal retries of `tools/call` that would cause retry amplification.

#### Scenario: Failure response is explicit about retrying

- **WHEN** a `callTool` invocation fails
- **THEN** the response's `agentInstruction` explicitly states whether the agent should retry, avoid retrying, choose another tool, ask the user, or report the failure
