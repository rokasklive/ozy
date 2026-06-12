## ADDED Requirements

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

- **WHEN** a user runs a command whose behavior is not yet implemented (e.g. `ozy index --format json` or `ozy eval run`)
- **THEN** the output is a structured result with a `NOT_IMPLEMENTED` marker and an `agentInstruction` stating that the capability is pending implementation and what to do next, and the process exits with a non-zero status

Note: broker-backed commands that are wired (e.g. `ozy search`) return their real instructional decision such as `catalog_empty`, not `NOT_IMPLEMENTED`.

### Requirement: CLI mirrors broker operations

The CLI SHALL invoke the same in-process broker seam used by the MCP adapter, so that CLI and MCP behavior cannot drift, per `SPEC.md` §4.9 and §15.

#### Scenario: CLI routes through the shared broker

- **WHEN** a broker-backed command runs
- **THEN** it calls the shared broker interface rather than a CLI-private code path
