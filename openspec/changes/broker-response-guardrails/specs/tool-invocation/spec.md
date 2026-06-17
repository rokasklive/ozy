## ADDED Requirements

### Requirement: Validate arguments against the cataloged schema before invoking

Ozy SHALL validate the agent-supplied `callTool` arguments against the tool's cataloged input schema before connecting to the downstream server, and SHALL return a structured `ARGUMENT_VALIDATION_FAILED` failure when the arguments do not satisfy that schema. The failure SHALL name the offending fields and instruct the agent to correct them and confirm the exact schema with `describeTool`, and SHALL be marked non-retryable for the same arguments. Validation SHALL be best-effort: when no input schema is cataloged for the `toolRef` (for example before `ozy index` has run), Ozy SHALL skip validation and proceed to invoke, so `callTool` keeps working without a prior index. Validation SHALL check declared `required` field presence and declared scalar/array/object `type`s, and SHALL allow undeclared extra fields (additionalProperties defaults to true).

#### Scenario: Missing required argument is rejected before any downstream call

- **WHEN** an agent calls `callTool` for a tool whose cataloged schema marks a field as required and that field is absent from the arguments
- **THEN** Ozy returns a structured `ARGUMENT_VALIDATION_FAILED` failure naming the missing field and instructing the agent to correct the arguments and confirm the schema with `describeTool`, without connecting to the downstream server

#### Scenario: Wrong argument type is rejected before any downstream call

- **WHEN** an agent calls `callTool` with an argument whose value does not match the type declared in the cataloged schema
- **THEN** Ozy returns `ARGUMENT_VALIDATION_FAILED` naming the field and its expected type and does not contact the downstream server

#### Scenario: Valid arguments pass through to invocation

- **WHEN** an agent calls `callTool` with arguments that satisfy the cataloged schema
- **THEN** Ozy proceeds to invoke the downstream tool as specified by the invocation requirement

#### Scenario: Missing cataloged schema skips validation

- **WHEN** an agent calls `callTool` for a `toolRef` that has no cataloged input schema (for example before `ozy index` has run)
- **THEN** Ozy skips argument validation and proceeds to invoke the downstream tool, preserving index-free invocation
