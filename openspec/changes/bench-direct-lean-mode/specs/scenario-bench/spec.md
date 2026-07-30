## ADDED Requirements

### Requirement: Direct-lean real-world baseline mode

The harness SHALL provide a `direct-lean` execution mode that wires **only** the
scenario's functional fixture toolsets — never the corpus servers — and exposes
**every** tool of each wired server directly to the agent, including that server's
non-task sibling tools. It is `direct` with the corpus omitted: the "installed
only the MCPs I need, and pay for every tool they ship" baseline that a careful
operator would actually run, giving an honest real-world comparison against `ozy`.

#### Scenario: Lean mode wires needed MCPs whole

- **WHEN** the agent starts in `direct-lean` mode for a scenario whose functional toolset servers together ship N tools (task-critical plus in-server siblings)
- **THEN** all N tools of those functional servers are advertised to the agent at startup, and no corpus server is wired

#### Scenario: Lean mode omits the corpus even when the scenario attaches it

- **WHEN** a scenario sets `corpus: true` and is run in `direct-lean` mode
- **THEN** the corpus servers are attached in `direct` mode but excluded from `direct-lean`, so the lean surface counts only the functional servers' tools

## MODIFIED Requirements

### Requirement: Identical-environment multi-mode execution

The harness SHALL run a scenario in `direct`, `direct-lean`, `ozy`, `all`, or
`both` modes such that the scenario definition, task prompt, fixture repository,
model endpoint, and process environment are identical across modes and only the
agent's MCP wiring differs. The `bench-runner` entrypoint SHALL accept
`--scenario <name>` and `--mode direct|direct-lean|ozy|all|both`, where `all`
runs `direct`, `direct-lean`, and `ozy` in a single pass and `both` remains a
back-compat alias for `direct` + `ozy`. When no mode is selected (unset `MODE`,
no flag) the harness SHALL default to `all`.

#### Scenario: All mode runs the three wirings from one fixture

- **WHEN** `bench-runner --scenario suspended-account-invoice-regression --mode all` is invoked
- **THEN** the harness runs the `direct`, `direct-lean`, and `ozy` sides against the same generated fixture, prompt, model endpoint, and environment, and produces one comparison covering all three

#### Scenario: Default is a single three-mode pass

- **WHEN** the harness is invoked with neither `--mode` nor `MODE` set
- **THEN** it runs `direct`, `direct-lean`, and `ozy` as one pass (equivalent to `--mode all`)

#### Scenario: Only wiring differs between modes

- **WHEN** the harness prepares the `direct`, `direct-lean`, and `ozy` runs
- **THEN** the task prompt, model endpoint, and fixture are byte-identical for all, recorded as a single scenario hash, and the only difference is the agent's MCP configuration file

### Requirement: Static surface tier independent of any model

The harness SHALL be able to enumerate and compare the startup tool/schema surface
of the `direct`, `direct-lean`, and `ozy` modes without contacting any model or
agent, and emit a surface-only comparison. The `direct-lean` surface SHALL count
only the functional toolset servers' tools (the corpus is excluded).

#### Scenario: Surface comparison runs with no model

- **WHEN** the harness is run in surface-only mode (`make bench-surface` or `--surface-only`)
- **THEN** it starts each mode's MCP wiring, counts the tools and schema bytes/tokens advertised at startup, and writes a comparison showing the direct, direct-lean, and ozy surfaces and their deltas, with live metrics marked skipped

#### Scenario: Lean surface excludes the corpus

- **WHEN** the static surface tier computes the `direct-lean` surface for a corpus-attached scenario
- **THEN** its tool count and schema tokens include every functional server tool but no corpus tool, sitting between the full `direct` surface and the `ozy` broker surface

## RENAMED Requirements

- FROM: `### Requirement: Two-mode identical-environment execution`
- TO: `### Requirement: Identical-environment multi-mode execution`
