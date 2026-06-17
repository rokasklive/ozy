## ADDED Requirements

### Requirement: callTool surfaces the downstream payload directly

On a successful `callTool`, the MCP adapter SHALL surface the downstream tool's
payload directly as the primary readable content of the response, and SHALL NOT
JSON-stringify Ozy's §9.3 envelope into a text block while also duplicating the
same envelope into structured content. A textual downstream result SHALL be
returned as text content the agent can read without parsing an outer Ozy
envelope; a structured downstream result SHALL be preserved as structured
content rather than re-stringified. Ozy's call metadata (for example `toolRef`
and `resultSummary`) SHALL be carried once alongside the payload, not wrapped
around it. Error responses SHALL remain §9.3 structured failure envelopes.

#### Scenario: Textual result is directly readable

- **WHEN** an agent calls `callTool` and the downstream tool returns text content
- **THEN** the response's readable content is that text, not a JSON-stringified
  Ozy envelope the agent must unwrap to reach the result

#### Scenario: Structured result is preserved as structured content

- **WHEN** an agent calls `callTool` and the downstream tool returns structured
  content
- **THEN** the response carries that structured payload as structured content
  rather than re-encoding it as a string nested inside another envelope

#### Scenario: Call metadata is not double-wrapped

- **WHEN** an agent reads a successful `callTool` response
- **THEN** the downstream payload appears once and Ozy's metadata (`toolRef`,
  `resultSummary`) is attached alongside it, with no second copy of the full
  envelope stringified into the content channel

#### Scenario: Errors stay structured failures

- **WHEN** a `callTool` invocation fails with a resolution, connection, or
  downstream error
- **THEN** the response is still a §9.3 structured failure envelope with
  `ok: false`, `error.type`, and `agentInstruction`
