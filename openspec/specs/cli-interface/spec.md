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

- **WHEN** a user runs a command whose behavior is not yet implemented (e.g. `ozy index --format json` or `ozy eval run`)
- **THEN** the output is a structured result with a `NOT_IMPLEMENTED` marker and an `agentInstruction` stating that the capability is pending implementation and what to do next, and the process exits with a non-zero status

Note: broker-backed commands that are wired (e.g. `ozy search`) return their real instructional decision such as `catalog_empty`, not `NOT_IMPLEMENTED`.

### Requirement: CLI mirrors broker operations

The CLI SHALL invoke the same in-process broker seam used by the MCP adapter, so that CLI and MCP behavior cannot drift, per `SPEC.md` §4.9 and §15.

#### Scenario: CLI routes through the shared broker

- **WHEN** a broker-backed command runs
- **THEN** it calls the shared broker interface rather than a CLI-private code path

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
