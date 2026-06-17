## ADDED Requirements

### Requirement: findTool description advertises available downstream servers

The MCP adapter SHALL include a bounded breadcrumb of the available downstream servers in the advertised `findTool` tool description, so an agent that lists Ozy's tools sees which capabilities Ozy can reach before making its first call. The breadcrumb SHALL be derived from Ozy's resolved configuration and/or catalog at adapter construction time, SHALL be bounded in length (a capped list of server identifiers or capability labels with an overflow indicator when truncated), and SHALL be omitted when no downstream servers are available or when disabled by configuration. The breadcrumb SHALL be enabled by default.

#### Scenario: findTool description names available servers by default

- **WHEN** Ozy is configured with one or more downstream servers and an MCP client lists Ozy's tools
- **THEN** the advertised `findTool` description includes a bounded summary naming the available downstream servers

#### Scenario: Breadcrumb is bounded when many servers are configured

- **WHEN** more downstream servers are configured than the breadcrumb cap
- **THEN** the description lists up to the cap and indicates that additional servers exist rather than emitting an unbounded list

#### Scenario: Breadcrumb can be disabled by configuration

- **WHEN** the capability breadcrumb is disabled in configuration
- **THEN** the advertised `findTool` description omits the server summary and matches the static description

#### Scenario: No available servers yields no breadcrumb

- **WHEN** no downstream servers are available
- **THEN** the advertised `findTool` description omits the breadcrumb rather than showing an empty list

### Requirement: Agent-facing responses emit a single representation per payload

The MCP adapter SHALL emit each agent-facing tool response payload in a single representation rather than duplicating the same payload across both the `content` text and `structuredContent` fields. Because Ozy's three tools declare no `outputSchema`, the adapter SHALL carry its `findTool`, `describeTool`, and structured-failure payloads as compact JSON text in `content` and SHALL NOT additionally set `structuredContent` to a copy of the same payload. A successful `callTool` result SHALL likewise be carried once, preserving the existing separation of Ozy's call metadata into `_meta`.

#### Scenario: findTool response is not duplicated across content and structuredContent

- **WHEN** an MCP client calls `findTool`
- **THEN** the response carries the decision payload once, as compact JSON in `content`, and does not also repeat the identical payload in `structuredContent`

#### Scenario: Structured failure is carried once

- **WHEN** a `describeTool` or `callTool` call returns a §9.3 structured failure
- **THEN** the failure envelope is carried in a single representation rather than duplicated across `content` and `structuredContent`

#### Scenario: callTool success result is not double-wrapped

- **WHEN** a `callTool` call succeeds with a structured downstream result
- **THEN** the downstream result is carried once and Ozy's call metadata rides in `_meta`, with no second stringified copy of the same payload
