## ADDED Requirements

### Requirement: Uninstall command

The `ozy` CLI SHALL provide an `uninstall` subcommand wired through the same app
and render plumbing as the other commands. It SHALL support `--dry-run`,
`--keep-config`, `--keep-data`, and `--purge`, and SHALL be plan-first and
consent-based per the uninstaller capability. Destructive removals (config,
data/models/vector stores, user MCP definitions, PATH edits) MUST require
explicit confirmation and MUST NOT be performed by `--yes` alone.

#### Scenario: Uninstall command is available
- **WHEN** the user runs `ozy uninstall`
- **THEN** the uninstall flow starts with detection and a removal plan, honoring
  the conservative default scope

#### Scenario: Uninstall dry-run
- **WHEN** the user runs `ozy uninstall --dry-run`
- **THEN** the removal plan is printed and nothing is deleted

### Requirement: Installer bootstrap is a separate entrypoint

The one-command setup bootstrap SHALL ship as a separate binary at
`cmd/ozy-install`, not as an `ozy` subcommand, so it can run before `ozy` exists.
The main CLI documentation SHALL reference `go run …/cmd/ozy-install@<version>`
as the install entrypoint alongside the existing commands.

#### Scenario: Bootstrap documented alongside CLI
- **WHEN** a user reads the CLI command surface documentation
- **THEN** it points to `go run …/cmd/ozy-install@<version>` for installation and
  `ozy uninstall` for removal
