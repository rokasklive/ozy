## MODIFIED Requirements

### Requirement: Configuration discovery and loading

Ozy SHALL load configuration from a single `ozy.jsonc` or `ozy.json` file (JSONC: JSON permitting comments and trailing commas), supporting an override path and a documented default location with defined precedence. Downstream servers SHALL be declared under an `mcp` key using the opencode shape, and Ozy's own `search`, `embedding`, and `budgets` sections SHALL be sibling keys.

#### Scenario: Loading a valid JSONC configuration

- **WHEN** Ozy starts with a valid `ozy.jsonc` containing an `mcp` map and Ozy sections
- **THEN** it parses each `mcp` entry (`type`, and `command`/`environment` or `url`/`headers`, and `enabled`) and the `search`/`embedding`/`budgets` sections into a typed in-memory model without error

#### Scenario: JSONC comments and trailing commas are accepted

- **WHEN** the configuration file contains `//` or `/* */` comments and trailing commas
- **THEN** Ozy parses it successfully rather than rejecting it as invalid JSON

#### Scenario: Explicit config path override

- **WHEN** a config path is provided via flag or environment variable
- **THEN** Ozy loads that file instead of the default location

#### Scenario: Missing configuration file

- **WHEN** no configuration file exists at the resolved path
- **THEN** Ozy reports a structured `CONFIG_ERROR` indicating the expected path and the repair action (run `ozy init`) rather than crashing

### Requirement: Environment reference resolution

Configuration SHALL support environment-variable references in `mcp` server `environment` values and `headers` values using the opencode `{env:NAME}` syntax, resolving them from the process environment at load time, and SHALL NOT require literal secrets in the file.

#### Scenario: Resolving a present environment reference

- **WHEN** a remote server's `headers` contains `"Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}"` and that variable is set
- **THEN** the resolved in-memory value contains the environment value

#### Scenario: Missing environment reference is diagnosable

- **WHEN** a value references an environment variable that is not set
- **THEN** Ozy records the missing variable as a structured diagnostic naming the variable and the server it belongs to

### Requirement: Configuration validation

Ozy SHALL validate each `mcp` server entry and report structured errors that identify the offending server and field: `type` MUST be `local` or `remote`; a `local` server MUST have a non-empty `command`; a `remote` server MUST have a `url`.

#### Scenario: Local server without a command is rejected

- **WHEN** an `mcp` entry has `type: local` but no `command`
- **THEN** Ozy returns a structured `CONFIG_ERROR` naming the server and the missing `command`, and does not start brokering

#### Scenario: Remote server without a url is rejected

- **WHEN** an `mcp` entry has `type: remote` but no `url`
- **THEN** Ozy returns a structured `CONFIG_ERROR` naming the server and the missing `url`

#### Scenario: Unknown server type is rejected

- **WHEN** an `mcp` entry has a `type` other than `local` or `remote`
- **THEN** Ozy returns a structured `CONFIG_ERROR` naming the server and the invalid type

### Requirement: Redaction in diagnostics

Ozy SHALL redact resolved secret values originating from `{env:NAME}` references in `headers` and `environment` when surfacing configuration in diagnostics or logs, showing the reference form or a masked placeholder instead of the secret.

#### Scenario: Diagnostics show redacted configuration

- **WHEN** configuration is rendered for `ozy doctor` or logging
- **THEN** values originating from `{env:NAME}` references appear redacted (e.g. `{env:ATLASSIAN_MCP_TOKEN}` or `****`) and never expose the resolved secret
