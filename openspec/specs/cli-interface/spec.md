# cli-interface

## Purpose

Define the `ozy` command surface (`SPEC.md` §15) and its output contract: the MVP
subcommands, the shared `--format` modes (human / JSON / concise), structured
handling of not-yet-implemented operations, and the rule that the CLI routes
through the same broker seam as the MCP adapter so the two cannot drift.

## Requirements

### Requirement: CLI command surface

The `ozy` CLI SHALL expose the MVP command surface defined in `SPEC.md` §15: `init`, `daemon`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, and `eval run`. Each command SHALL be registered with help text describing its purpose.

#### Scenario: All MVP commands are registered

- **WHEN** a user runs `ozy --help`
- **THEN** all of `init`, `daemon`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, and `eval` appear with usage descriptions

#### Scenario: Command-specific help

- **WHEN** a user runs `ozy search --help`
- **THEN** the command's expected arguments and flags are described

### Requirement: Output formats

The CLI SHALL support a `--format` selection covering at least human-readable, JSON, and concise modes, so that agents and evals can consume machine-readable output while humans get readable output.

#### Scenario: JSON output for agents

- **WHEN** a command is run with `--format json`
- **THEN** the command emits a single well-formed JSON document to stdout suitable for programmatic consumption

#### Scenario: Default human-readable output

- **WHEN** a command is run without a `--format` flag
- **THEN** the command emits human-readable text

### Requirement: Structured handling of unimplemented operations

Commands whose broker behavior is deferred to later changes SHALL return a structured, instructional `NOT_IMPLEMENTED` result instead of panicking or printing an unstructured stub message, preserving the instructional-response principle from `SPEC.md` §4.5.

#### Scenario: Unimplemented command returns an instructional result

- **WHEN** a user runs a command whose behavior is deferred to a later change
- **THEN** the output is a structured result with a `NOT_IMPLEMENTED` marker and an `agentInstruction` stating that the capability is pending implementation and what to do next, and the process exits with a non-zero status

Note: broker-backed commands that are wired (e.g. `ozy search`) return their real instructional decision such as `catalog_empty`, not `NOT_IMPLEMENTED`. `ozy eval run` is now wired to the eval harness and no longer returns `NOT_IMPLEMENTED`.

### Requirement: CLI mirrors broker operations

The CLI SHALL invoke the same in-process broker seam used by the MCP adapter, so that CLI and MCP behavior cannot drift, per `SPEC.md` §4.9 and §15.

#### Scenario: CLI routes through the shared broker

- **WHEN** a broker-backed command runs
- **THEN** it calls the shared broker interface rather than a CLI-private code path

### Requirement: Eval command runs the eval suite

The `ozy eval` command SHALL execute the eval harness rather than returning
`NOT_IMPLEMENTED`. `ozy eval run [scenario]` SHALL run the suite (optionally
scoped to a named scenario family) and `ozy eval report` SHALL emit the latest
benchmark summary, both honoring the shared `--format` modes so agents and CI can
consume `--format json`. A run whose gated metrics fall below their thresholds
SHALL exit with a non-zero status. The command MUST route through the same broker
seam the rest of the CLI and the MCP adapter use, so evaluating the system cannot
diverge from the system being evaluated.

#### Scenario: eval run executes the harness

- **WHEN** a user runs `ozy eval run --format json`
- **THEN** the harness runs over the committed corpus and emits a single JSON result containing the computed metrics and an overall pass/fail verdict

#### Scenario: eval run can scope to one family

- **WHEN** a user runs `ozy eval run discovery`
- **THEN** only the named scenario family is evaluated and reported

#### Scenario: Gate failure sets a non-zero exit status

- **WHEN** an `ozy eval run` invocation produces metrics that fall below the configured thresholds
- **THEN** the process exits with a non-zero status so CI can gate on it

#### Scenario: eval report emits the latest benchmark

- **WHEN** a user runs `ozy eval report --format json` after a run
- **THEN** the command emits the latest benchmark snapshot with its provenance

### Requirement: CLI exposes tools from explicit MCP configuration

The Ozy CLI SHALL load an explicit opencode-shaped MCP config path, index reachable configured MCP servers, and expose the resulting tools through broker-backed CLI commands.

#### Scenario: Indexing tools from the example config

- **WHEN** a user runs `ozy --config examples/test_mcp_examples.jsonc index --format json` in an environment where the enabled configured server commands are available and reachable
- **THEN** Ozy connects to the enabled MCP servers, calls `tools/list`, persists discovered tool metadata, and emits a JSON summary containing reached server and indexed tool counts plus any per-server failures

#### Scenario: Listing indexed tools from the CLI

- **WHEN** tools have been indexed from an explicit MCP config path and a user runs `ozy --config examples/test_mcp_examples.jsonc list --format json`
- **THEN** the CLI returns a JSON result containing the discovered toolRefs with their server ids and freshness status

#### Scenario: Describing an indexed tool from the CLI

- **WHEN** a toolRef returned by `ozy list` was discovered from the explicit MCP config path and a user runs `ozy --config examples/test_mcp_examples.jsonc describe <toolRef> --format json`
- **THEN** the CLI returns that tool's name, description, input schema, server status, freshness, and usage guidance instead of `TOOL_NOT_FOUND`

#### Scenario: Search uses the populated catalog

- **WHEN** at least one tool has been indexed from an explicit MCP config path and a user runs `ozy --config examples/test_mcp_examples.jsonc search <query> --format json`
- **THEN** the CLI returns a broker decision derived from the populated catalog rather than the `catalog_empty` decision

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
