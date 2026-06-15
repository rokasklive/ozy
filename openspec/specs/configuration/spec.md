# configuration

## Purpose

Define how Ozy loads its single, explicit, inspectable configuration file
(`SPEC.md` §11): discovery and override with user-config-home default,
`{env:NAME}` reference resolution so secrets stay out of the file, structural
validation with repair-oriented errors, opencode MCP shape compatibility, and
redaction so diagnostics never leak resolved secrets.

## Requirements

### Requirement: Configuration discovery and loading

Ozy SHALL load configuration from a single explicit, inspectable `ozy.jsonc` or `ozy.json` file. Unless an override path is provided, the default configuration location SHALL be the Ozy user config home: `$XDG_CONFIG_HOME/ozy` when set, otherwise `~/.config/ozy` on Unix-like systems, and the OS roaming user config equivalent such as `%AppData%\ozy` on Windows. Ozy SHALL NOT implicitly prefer `./ozy.jsonc` or `./ozy.json` from the current working directory.

#### Scenario: Loading a valid default user configuration file

- **WHEN** Ozy starts without a config override and a valid config file is present at the resolved user config path
- **THEN** it parses the `version`, `mcp`, `embedding`, `search`, `budgets`, and `cache` sections into a typed in-memory model without error

#### Scenario: Explicit config path override

- **WHEN** a config path is provided via flag or environment variable
- **THEN** Ozy loads that file instead of the default user config location

#### Scenario: Project-local config is not discovered implicitly

- **WHEN** `./ozy.jsonc` exists but no config override is provided
- **THEN** Ozy resolves the default path under the user config home rather than loading the project-local file

#### Scenario: Missing configuration file

- **WHEN** no configuration file exists at the resolved path
- **THEN** Ozy reports a structured `CONFIG_ERROR` indicating the expected path and the repair action (run `ozy init`) rather than crashing

#### Scenario: Cache section defaults are applied when omitted

- **WHEN** Ozy loads a configuration file that omits the `cache` section
- **THEN** the resolved in-memory model treats the result cache as enabled with the documented default TTL and maximum entry count

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

### Requirement: Configuration initialization writes to user config home

Ozy SHALL write new starter configuration files to the resolved Ozy user config home by default, creating the directory when needed and refusing to overwrite an existing file.

#### Scenario: Init writes to default user config file

- **WHEN** a user runs `ozy init` without an explicit config override and no config file exists
- **THEN** Ozy creates the Ozy user config directory and writes `ozy.jsonc` there with owner-private permissions

#### Scenario: Init honors explicit config override

- **WHEN** a user runs `ozy init --config ./ozy.jsonc`
- **THEN** Ozy writes the starter config to `./ozy.jsonc` instead of the user config home

#### Scenario: Init refuses to overwrite config

- **WHEN** a config file already exists at the resolved path
- **THEN** Ozy reports a structured failure and leaves the existing file unchanged

### Requirement: Opencode MCP section compatibility

Ozy SHALL accept opencode-compatible JSONC configuration for the top-level `mcp` section only, so users can copy MCP server snippets into Ozy config without reshaping. Local MCP entries SHALL support `type`, `command`, `cwd`, `environment`, `enabled`, and `timeout`. Remote MCP entries SHALL support `type`, `url`, `headers`, `oauth`, `enabled`, and `timeout`.

#### Scenario: Example fixture loads successfully

- **WHEN** Ozy loads `examples/test_mcp_examples.jsonc`
- **THEN** it parses the `searxng`, `javadoc`, and `opengrok` MCP server entries without a syntax or structural validation error

#### Scenario: Local command entries are preserved

- **WHEN** Ozy loads an enabled local MCP server from the example fixture
- **THEN** the resolved in-memory model preserves the command array and environment map needed to launch that server

#### Scenario: Local cwd is preserved

- **WHEN** Ozy loads a local MCP server containing a `cwd` value
- **THEN** the resolved in-memory model preserves that working directory for launching the server process

#### Scenario: Remote headers are preserved

- **WHEN** Ozy loads a remote MCP server containing a `headers` object
- **THEN** the resolved in-memory model preserves those headers for remote MCP transport setup with secret redaction rules still applying

#### Scenario: OAuth config is preserved

- **WHEN** Ozy loads a remote MCP server containing `oauth` as either an object or `false`
- **THEN** the resolved in-memory model preserves the OAuth setting instead of rejecting the configuration

#### Scenario: OAuth runtime is unavailable

- **WHEN** a remote MCP server requires OAuth authentication and Ozy has no runtime OAuth flow available
- **THEN** Ozy reports a structured authentication-unavailable diagnostic with repair guidance rather than treating the config as invalid

#### Scenario: Enabled defaults to true

- **WHEN** an MCP server entry omits `enabled`
- **THEN** Ozy treats the server as enabled

#### Scenario: Enabled false disables server

- **WHEN** an MCP server entry sets `enabled` to `false`
- **THEN** Ozy keeps the server in the config model but skips connecting to it

#### Scenario: Timeout is preserved

- **WHEN** Ozy loads a server entry containing `timeout: 180000`
- **THEN** the resolved in-memory model records that timeout as the server's total discovery timeout in milliseconds

#### Scenario: Default timeout is applied

- **WHEN** an MCP server entry omits `timeout`
- **THEN** Ozy applies the opencode-compatible default discovery timeout of 5000 milliseconds

#### Scenario: Non-MCP opencode sections are out of scope

- **WHEN** an Ozy config includes unrelated top-level opencode sections such as `agent`, `tools`, `theme`, `permission`, or `provider`
- **THEN** Ozy does not treat those sections as part of MCP server compatibility

### Requirement: Vector backend selection

Configuration SHALL allow selecting the semantic vector backend under the `embedding` section, defaulting to `turbovec` when unset and accepting `faiss` as the only alternative. Ozy SHALL validate the value and report a structured `CONFIG_ERROR` for an unknown backend. Ozy SHALL NOT require any vector-storage configuration for the default turbovec path.

#### Scenario: Default backend is turbovec when unset

- **WHEN** configuration enables semantic search but omits the vector backend
- **THEN** Ozy resolves the backend to `turbovec` without requiring any vector-storage configuration

#### Scenario: FAISS backend is accepted

- **WHEN** configuration sets the `embedding` vector backend to `faiss`
- **THEN** Ozy resolves the backend to FAISS for the semantic index

#### Scenario: Unknown backend is rejected

- **WHEN** configuration sets the vector backend to a value other than `turbovec` or `faiss`
- **THEN** Ozy reports a structured `CONFIG_ERROR` naming the invalid backend rather than starting with an undefined backend

### Requirement: Embedding model selection

Configuration SHALL allow selecting the FastEmbed embedding model under the `embedding` section, applying a documented CPU-friendly default when unset. The vector dimension SHALL be derived from the selected model rather than configured separately.

#### Scenario: Default model applied when unset

- **WHEN** configuration enables semantic search but omits the embedding model
- **THEN** Ozy uses the documented default FastEmbed model and derives the vector dimension from it

#### Scenario: Explicit model is honored

- **WHEN** configuration sets an explicit FastEmbed model under `embedding`
- **THEN** Ozy uses that model for both document and query embeddings and derives the dimension from it

### Requirement: Semantic search is enabled by default

The configuration model SHALL treat semantic search as enabled by default: when `search.semantic.enabled` is unset, Ozy SHALL treat semantic search as on and SHALL consult the embedding model and vector-backend settings (defaulting to the FastEmbed default model and turbovec), so the out-of-the-box experience is hybrid search with the sidecar auto-provisioned rather than lexical-only. A user SHALL be able to disable semantic search explicitly, in which case Ozy SHALL run lexical-only and SHALL NOT provision the sidecar. Default-on SHALL NOT make Ozy fail when the sidecar is unavailable: provisioning failures are handled by graceful degradation, not by changing the default.

#### Scenario: Semantic enabled by default when unset

- **WHEN** configuration omits `search.semantic.enabled`
- **THEN** Ozy treats semantic search as enabled and uses the default embedding model and turbovec backend for the semantic path

#### Scenario: Disabling semantic is honored

- **WHEN** configuration sets `search.semantic.enabled` to false
- **THEN** Ozy runs lexical-only and does not provision or launch the embedding sidecar
