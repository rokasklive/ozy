# uninstaller

## Purpose

The Ozy uninstaller — shared between `ozy uninstall` and
`go run …/cmd/ozy-install@<version> uninstall` — that detects all Ozy-managed
locations, presents a plan, and conservatively removes only the binary and
Ozy-managed runtime by default, requiring explicit consent before touching user
config, data, models, vector stores, MCP definitions, or shell `PATH` blocks.
It is plan-first, resumable, durable-logged, and uses the same progress
dashboard model as the installer.

## Requirements

### Requirement: Uninstall entrypoints

Ozy SHALL provide an uninstall flow runnable as `ozy uninstall` once installed,
and as `go run …/cmd/ozy-install@<version> uninstall` before or instead of a
working install. Both entrypoints SHALL share the same uninstall planner, state
machine, consent model, progress dashboard, and logging as the install flow.

#### Scenario: Uninstall via installed binary

- **WHEN** the user runs `ozy uninstall`
- **THEN** the uninstall flow starts with detection and a removal plan

#### Scenario: Uninstall via bootstrap

- **WHEN** the user runs `go run …/cmd/ozy-install@<version> uninstall`
- **THEN** the same uninstall flow runs even if the `ozy` binary is missing or
  broken

### Requirement: Plan-first detection and display

The uninstaller SHALL detect all Ozy-managed locations and display exactly what
would be removed before removing anything, separating generated/cache data from
user-authored config and downstream MCP definitions. `--dry-run` SHALL print
this plan and exit without removing anything.

#### Scenario: Plan shown before removal

- **WHEN** the uninstaller runs
- **THEN** it lists every detected Ozy-managed location grouped by category and
  asks before removing protected categories

#### Scenario: Dry-run removes nothing

- **WHEN** the uninstaller runs with `--dry-run`
- **THEN** it prints the removal plan and exits having deleted nothing

### Requirement: Conservative default removal scope

By default the uninstaller SHALL remove installed binaries and Ozy-managed
runtime files, and SHALL ask before removing config, logs, caches, downloaded
models, vector stores, or user-authored MCP definitions. It MUST preserve
user-created downstream MCP definitions unless the user explicitly confirms their
deletion.

#### Scenario: Default removes managed runtime only

- **WHEN** the uninstaller runs with no mode flags and the user declines the
  data/config prompts
- **THEN** the binary and Ozy-managed runtime files are removed while config,
  caches, models, vector stores, and MCP definitions remain

#### Scenario: User config preserved by default

- **WHEN** the uninstaller runs and the user does not confirm config deletion
- **THEN** `ozy.jsonc` and its downstream MCP definitions are left intact

### Requirement: Removal mode flags

The uninstaller SHALL support `--keep-config`, `--keep-data`, and `--purge`.
`--keep-config` and `--keep-data` SHALL exclude those categories from removal.
`--purge` MAY remove everything but only after an explicit, distinct
confirmation; `--yes` alone MUST NOT trigger purge of user config or data.

#### Scenario: Keep flags exclude categories

- **WHEN** the uninstaller runs with `--keep-config --keep-data`
- **THEN** config and data/model/vector directories are excluded from the removal
  plan

#### Scenario: Purge requires explicit confirmation

- **WHEN** the uninstaller runs with `--purge`
- **THEN** it requires a distinct explicit confirmation before deleting user
  config, data, and MCP definitions, even if `--yes` is also set

### Requirement: PATH and shell cleanup with consent

The uninstaller SHALL remove only the clearly marked PATH/rc block that the
installer added, and only with consent. It MUST NOT edit unrelated shell-profile
content.

#### Scenario: Marked block removed on consent

- **WHEN** the installer-added rc block is present and the user consents
- **THEN** only that marked block is removed and the change is reported

#### Scenario: No marked block, no edit

- **WHEN** no installer-added marked block is found
- **THEN** the uninstaller makes no shell-profile edits and says so

### Requirement: Resumable uninstall state machine

Uninstallation SHALL be modeled as discrete idempotent steps (DetectInstall,
PlanRemovals, RemoveBinary, RemoveManagedData, CleanPath, WriteSummary) and
SHALL be safe to rerun after a partial uninstall, skipping already-removed items.

#### Scenario: Rerun after partial uninstall

- **WHEN** an uninstall is interrupted and rerun
- **THEN** already-removed items are detected as absent and skipped, and the run
  completes without error

### Requirement: Uninstall logging and summary

Every uninstall run SHALL create a durable, redacted log (timestamps, version,
OS/arch, detected locations, per-step results, removals performed, PATH/shell
changes, final status) and SHALL print the log path at the end, using the same
progress dashboard model as install. On failure it SHALL print the safe-retry
command.

#### Scenario: Uninstall log written

- **WHEN** an uninstall run finishes
- **THEN** a log file recording detected locations and removals exists and its
  path is printed, with no secrets in the log
