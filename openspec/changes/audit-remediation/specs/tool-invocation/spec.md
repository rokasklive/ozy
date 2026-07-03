## MODIFIED Requirements

### Requirement: Invoke the downstream tool via tools/call

Ozy SHALL invoke the resolved tool by connecting to only the one target downstream server and calling MCP `tools/call` with the agent-supplied arguments, applying a per-call deadline from the server's configured `callTimeout` (default 60 seconds) — a budget distinct from and independent of the server's discovery `timeout`. When Ozy's own call deadline expires, the structured failure SHALL set `retryable: false` and name `callTimeout` as the cause, because retrying an identical call against the same deadline is deterministic. Ozy SHALL NOT connect to other configured servers to satisfy a single `callTool`.

#### Scenario: A reachable tool is invoked and returns a success result

- **WHEN** an agent calls `callTool` for a tool on a reachable, enabled server with valid arguments
- **THEN** Ozy connects to that one server, calls downstream `tools/call`, and returns a response with `ok: true`, the `toolRef`, the normalized downstream result, and a short `resultSummary`

#### Scenario: An unreachable server yields a structured failure

- **WHEN** an agent calls `callTool` for a tool whose downstream server cannot be connected
- **THEN** Ozy returns a structured failure with a §9.3 `error.type` such as `DOWNSTREAM_SERVER_OFFLINE`, `retryable` set appropriately, and an `agentInstruction` stating whether to retry, choose another tool, or report the failure

#### Scenario: Only the target server is contacted

- **WHEN** an agent calls `callTool` for a tool on server `atlassian` while other servers are also configured
- **THEN** Ozy attempts a connection only to `atlassian` and does not connect to the other configured servers for that call

#### Scenario: A slow call is not killed by the discovery timeout

- **WHEN** an agent calls `callTool` for a tool whose spawn-plus-execution takes longer than the server's discovery `timeout` but less than its `callTimeout`
- **THEN** the invocation completes successfully instead of failing at the discovery deadline

#### Scenario: Ozy's own deadline is reported honestly

- **WHEN** a `callTool` invocation exceeds the server's `callTimeout`
- **THEN** Ozy returns a structured failure whose message names `callTimeout` as the exceeded budget, with `retryable: false` and an `agentInstruction` to narrow the call or raise the configured `callTimeout` rather than retry unchanged

### Requirement: Normalize results and downstream errors

Ozy SHALL normalize a successful `tools/call` result into the §9.3 `callTool` success shape, and SHALL map a downstream-reported tool error into a structured §9.3 failure. Downstream error detail SHALL be preserved where useful but secret values from `headers` or `environment` SHALL NOT be leaked. When a result exceeds `budgets.callTool.maxResultBytes`, Ozy SHALL truncate at a structural boundary — dropping whole trailing elements of a top-level JSON array, or cutting a textual result at a line (fallback: word) boundary — and SHALL deliver the truncation notice and recovery guidance in the result content itself, never only in out-of-band metadata.

#### Scenario: Downstream tool error becomes a structured failure

- **WHEN** the downstream `tools/call` returns a tool-level error result
- **THEN** Ozy returns a structured failure with a §9.3 `error.type` (for example `DOWNSTREAM_CALL_FAILED`) carrying the downstream message and an `agentInstruction`, instead of reporting `ok: true`

#### Scenario: Secrets are redacted from invocation errors

- **WHEN** a `callTool` invocation fails for a server that uses a secret header or environment value
- **THEN** the reported error names the server and reason without including the resolved secret value

#### Scenario: An oversized array result is truncated element-wise

- **WHEN** a `callTool` result is a top-level JSON array whose encoding exceeds `budgets.callTool.maxResultBytes`
- **THEN** Ozy returns the leading elements that fit as valid JSON, and the response content states how many of the total items are shown and instructs the agent to narrow the call

#### Scenario: An oversized textual result is cut at a readable boundary

- **WHEN** a `callTool` result is textual (or non-array JSON) and exceeds the byte budget
- **THEN** Ozy cuts at the last line or word boundary under the budget and the response content states that the payload is partial (and, for JSON, not parseable as a whole) with guidance to narrow the call

### Requirement: Invocation does not amplify retries

Ozy SHALL state in each `callTool` response whether agent-side retry is recommended, and SHALL NOT perform hidden internal retries of `tools/call` that would cause retry amplification. Failures caused by budgets Ozy itself imposed (its call deadline, its byte budget) SHALL NOT be labeled `retryable: true`, because repeating the identical call cannot succeed.

#### Scenario: Failure response is explicit about retrying

- **WHEN** a `callTool` invocation fails
- **THEN** the response's `agentInstruction` explicitly states whether the agent should retry, avoid retrying, choose another tool, ask the user, or report the failure

#### Scenario: Self-imposed limits are never marked retryable

- **WHEN** a `callTool` failure is caused by Ozy's own configured deadline rather than a transient downstream fault
- **THEN** the structured error sets `retryable: false` and directs the agent to change something (arguments, scope, or configuration) before calling again
