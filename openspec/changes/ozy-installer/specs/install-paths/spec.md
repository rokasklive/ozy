## ADDED Requirements

### Requirement: Single platform-paths abstraction

Ozy SHALL resolve every install location — binary, config, data, cache, logs,
Python venv, vector/model assets, and the user bin directory — through one
internal package. Callers MUST NOT hardcode these locations. The package MUST
reuse the existing resolution in `internal/config` (config home, `ozy.jsonc`
path) and `internal/sidecar` (venv under the state dir) rather than duplicating
or contradicting it.

#### Scenario: Config and venv match existing resolution
- **WHEN** the paths package resolves the config path and the sidecar venv path
- **THEN** they equal what `config.DefaultPath()` and the sidecar provisioner
  already produce for the same environment

#### Scenario: No hardcoded locations
- **WHEN** the installer or uninstaller needs any Ozy location
- **THEN** it obtains it from the paths package, and a search of the installer
  code finds no literal `.config/ozy`, `.local/share/ozy`, or similar path
  fragments outside that package

### Requirement: Platform-appropriate directory rules

The paths package SHALL apply OS-appropriate conventions. On Linux and macOS it
SHALL use XDG-style locations (honoring `XDG_CONFIG_HOME`, `XDG_DATA_HOME`,
`XDG_CACHE_HOME`, `XDG_STATE_HOME` when set, else `~/.config/ozy`,
`~/.local/share/ozy`, `~/.cache/ozy`, `~/.local/state/ozy`, user bin
`~/.local/bin`). On Windows it SHALL use `%APPDATA%\Ozy` (config),
`%LOCALAPPDATA%\Ozy` (data/cache, with a `Cache` subdir for cache), and
`%LOCALAPPDATA%\Ozy\bin` (user bin). Documented environment overrides
(e.g. `OZY_CONFIG`) SHALL take precedence.

#### Scenario: Linux honors XDG variables
- **WHEN** `XDG_CONFIG_HOME` and `XDG_DATA_HOME` are set on Linux
- **THEN** config resolves under `$XDG_CONFIG_HOME/ozy` and data under
  `$XDG_DATA_HOME/ozy`

#### Scenario: Linux falls back to home defaults
- **WHEN** the XDG variables are unset on Linux
- **THEN** config resolves under `~/.config/ozy` and data under
  `~/.local/share/ozy`

#### Scenario: Windows uses OS conventions
- **WHEN** locations are resolved on Windows
- **THEN** config resolves under `%APPDATA%\Ozy`, data and cache under
  `%LOCALAPPDATA%\Ozy`, and the user bin under `%LOCALAPPDATA%\Ozy\bin`

#### Scenario: Override variable wins
- **WHEN** `OZY_CONFIG` is set
- **THEN** the resolved config path equals `OZY_CONFIG` regardless of platform
  defaults

### Requirement: PATH reachability detection

The paths package SHALL report whether the resolved `ozy` binary is reachable on
the current `PATH`, and SHALL expose the user bin directory that would need to be
added if it is not. Detection MUST NOT modify the environment or any file.

#### Scenario: Binary already on PATH
- **WHEN** the installed binary's directory is present in `PATH`
- **THEN** detection reports reachable=true and recommends no PATH change

#### Scenario: Binary not on PATH
- **WHEN** the installed binary's directory is absent from `PATH`
- **THEN** detection reports reachable=false and names the exact bin directory to
  add, without altering `PATH`
