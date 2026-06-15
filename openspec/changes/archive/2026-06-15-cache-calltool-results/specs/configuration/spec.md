## MODIFIED Requirements

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
