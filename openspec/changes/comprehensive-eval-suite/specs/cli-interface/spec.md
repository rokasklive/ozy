## MODIFIED Requirements

### Requirement: Structured handling of unimplemented operations

Commands whose broker behavior is deferred to later changes SHALL return a structured, instructional `NOT_IMPLEMENTED` result instead of panicking or printing an unstructured stub message, preserving the instructional-response principle from `SPEC.md` §4.5.

#### Scenario: Unimplemented command returns an instructional result

- **WHEN** a user runs a command whose behavior is deferred to a later change
- **THEN** the output is a structured result with a `NOT_IMPLEMENTED` marker and an `agentInstruction` stating that the capability is pending implementation and what to do next, and the process exits with a non-zero status

Note: broker-backed commands that are wired (e.g. `ozy search`) return their real instructional decision such as `catalog_empty`, not `NOT_IMPLEMENTED`. `ozy eval run` is now wired to the eval harness and no longer returns `NOT_IMPLEMENTED`.

## ADDED Requirements

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
