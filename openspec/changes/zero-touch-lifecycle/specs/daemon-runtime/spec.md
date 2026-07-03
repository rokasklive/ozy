## MODIFIED Requirements

### Requirement: Daemon lifecycle

Ozy SHALL provide a runtime that starts, loads configuration, initializes the broker seam and catalog store, provisions optional subsystems, reports readiness, and shuts down cleanly on interrupt, per `SPEC.md` §6.1. This runtime SHALL be hosted by the serving process — the MCP adapter (`ozy mcp`) — rather than exposed as a standalone `ozy daemon` command; its lifetime is bound to the serving process.

#### Scenario: Runtime starts and reports readiness

- **WHEN** the serving process (`ozy mcp`) starts with valid configuration
- **THEN** the runtime initializes configuration, broker, and catalog store, provisions optional subsystems, and reports a ready state

#### Scenario: Runtime shuts down cleanly

- **WHEN** the serving process receives an interrupt or termination signal
- **THEN** the runtime stops accepting work and releases resources, including any embedding sidecar, without leaving the process hung

#### Scenario: Runtime refuses to start on invalid configuration

- **WHEN** the serving process starts with configuration that fails validation
- **THEN** it reports the structured `CONFIG_ERROR` and exits non-zero instead of starting in a broken state
