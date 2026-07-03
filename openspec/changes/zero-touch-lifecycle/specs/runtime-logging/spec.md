## ADDED Requirements

### Requirement: Logs are written beside the configuration file

Ozy SHALL write its operational logs to a `logs/` directory located in the same directory as the loaded `ozy.jsonc` configuration file. When the configuration is loaded from a custom `--config` path, the `logs/` directory SHALL be resolved relative to that file's directory.

#### Scenario: Default config logs location

- **WHEN** ozy runs with configuration loaded from `~/.config/ozy/ozy.jsonc`
- **THEN** it writes operational logs under `~/.config/ozy/logs/`

#### Scenario: Custom config path logs location

- **WHEN** ozy runs with `--config /path/to/ozy.jsonc`
- **THEN** it writes operational logs under `/path/to/logs/`

### Requirement: Log lines are structured and agent-ergonomic

Ozy log lines SHALL be machine-parseable structured records that name the event and, for failures or degradations, name the cause and the next action a reader should take. A log record SHALL NOT consist solely of an unstructured message or a bare stack trace, and SHALL NOT include secret values such as tokens, headers, or environment values.

#### Scenario: A degradation log names cause and remedy

- **WHEN** semantic search is enabled but the embedding sidecar fails to provision
- **THEN** a log record states the event (sidecar provisioning failed), the cause, and the next action (for example, run `ozy doctor`), as structured fields

#### Scenario: Secrets are not logged

- **WHEN** a downstream connection error references a configured header or environment value
- **THEN** the logged record redacts the secret value rather than writing it in cleartext

### Requirement: Lifecycle and degradation events are logged

Ozy SHALL log the operational lifecycle events that explain what it is doing without the user attaching a debugger: startup phases (provisioning, indexing, ready), embedding sidecar transitions (provisioned, ready, shut down), degradation to lexical-only, partial embedding coverage, and shutdown.

#### Scenario: Startup is traceable from the log

- **WHEN** `ozy mcp` starts, provisions the sidecar, runs a conditional index, and reports ready
- **THEN** the log contains ordered records for each phase sufficient to explain what happened and roughly how long provisioning and indexing took

#### Scenario: Degraded session is explained in the log

- **WHEN** a session runs lexical-only because the sidecar is unavailable
- **THEN** the log contains a record naming the degradation and its cause
