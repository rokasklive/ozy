# configuration

## Purpose

Define how Ozy loads its single, explicit, inspectable configuration file
(`SPEC.md` §11): discovery and override, `${ENV}` reference resolution so
secrets stay out of the file, structural validation with repair-oriented
errors, and redaction so diagnostics never leak resolved secrets.

## Requirements

### Requirement: Configuration discovery and loading

Ozy SHALL load configuration from a single explicit, inspectable file matching the model in `SPEC.md` §11, supporting an override path and a documented default location.

#### Scenario: Loading a valid configuration file

- **WHEN** Ozy starts with a valid configuration file present at the default location
- **THEN** it parses the `version`, `servers`, `embedding`, `search`, and `budgets` sections into a typed in-memory model without error

#### Scenario: Explicit config path override

- **WHEN** a config path is provided via flag or environment variable
- **THEN** Ozy loads that file instead of the default location

#### Scenario: Missing configuration file

- **WHEN** no configuration file exists at the resolved path
- **THEN** Ozy reports a structured `CONFIG_ERROR` indicating the expected path and the repair action (run `ozy init`) rather than crashing

### Requirement: Environment reference resolution

Configuration SHALL support `${ENV_VAR}` references for secrets and environment-specific values, resolving them from the process environment at load time, and SHALL NOT require literal secrets in the file.

#### Scenario: Resolving a present environment reference

- **WHEN** a configuration value contains `${ATLASSIAN_MCP_TOKEN}` and that variable is set in the environment
- **THEN** the resolved in-memory value contains the environment value

#### Scenario: Missing environment reference is diagnosable

- **WHEN** a configuration value references an environment variable that is not set
- **THEN** Ozy records the missing variable as a structured diagnostic naming the variable and the server it belongs to

### Requirement: Configuration validation

Ozy SHALL validate loaded configuration for structural correctness and report structured errors that identify the offending field.

#### Scenario: Invalid configuration is rejected with a structured error

- **WHEN** a configuration file omits a required field or uses an unknown transport type
- **THEN** Ozy returns a structured `CONFIG_ERROR` naming the field and the reason, and does not start brokering

### Requirement: Redaction in diagnostics

Ozy SHALL redact resolved secret values when surfacing configuration in diagnostics or logs, showing the env-reference name or a masked placeholder instead of the secret.

#### Scenario: Diagnostics show redacted configuration

- **WHEN** configuration is rendered for `ozy doctor` or logging
- **THEN** values originating from secret env references appear redacted (e.g. `${ATLASSIAN_MCP_TOKEN}` or `****`) and never expose the resolved secret
