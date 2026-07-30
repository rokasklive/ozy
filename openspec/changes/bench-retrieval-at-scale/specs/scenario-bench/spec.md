## MODIFIED Requirements

### Requirement: Mode wiring contract

In `direct` mode the agent SHALL be configured with every MCP server the
scenario declares — its functional fixture toolsets plus, when the scenario
attaches the corpus (the default), every corpus server — exposed directly. In
`ozy` mode the agent SHALL be configured with only the Ozy MCP server, and Ozy
SHALL be configured with the identical scenario-declared server set as its
downstream and indexed before the agent runs.

#### Scenario: Direct mode exposes the full surface

- **WHEN** the agent starts in `direct` mode
- **THEN** the tools of all scenario-declared MCP servers (functional, distractor, and corpus) are advertised to the agent at startup

#### Scenario: Ozy mode exposes only the broker

- **WHEN** the agent starts in `ozy` mode
- **THEN** the agent is advertised only Ozy's stable interface (`findTool`/`describeTool`/`callTool`), and the same scenario-declared servers are reachable as Ozy downstream after `ozy index`

### Requirement: Mode extension via config templates

The orchestrator SHALL resolve a mode name to an agent config wiring at
`bench/configs/opencode.<mode>.jsonc`, parameterized by the scenario's declared
server set, so adding a mode variant requires a new config template and no
orchestrator code change. Mode-specific setup hooks (today: `ozy` writes its
config and indexes) SHALL be keyed by mode name. The checked-in templates SHALL
be generated from the same source the runner uses, so they cannot drift from
what a run actually wires.

#### Scenario: New mode is a data change

- **WHEN** a new config template `bench/configs/opencode.<variant>.jsonc` is added and the variant is passed as `--mode`
- **THEN** the harness runs it through the same runner, run layout, and reporting with no Go changes beyond an optional named setup hook

#### Scenario: Templates match the runner

- **WHEN** the checked-in mode templates are compared with the config the runner generates for the same scenario
- **THEN** the wired server set is identical

## ADDED Requirements

### Requirement: Scenario-declared toolsets

`scenario.jsonc` SHALL declare the scenario's functional fixture toolsets and
whether the corpus attaches (`corpus`, default true). The direct-mode agent
config, ozy's downstream config, and the static surface enumeration SHALL all
derive the server set from the scenario declaration — no hardcoded toolset list
in the harness.

#### Scenario: Scenario drives all three consumers

- **WHEN** a scenario declares `toolsets: [weather, duckduckgo, wikipedia, pdf-toolkit]`
- **THEN** direct mode wires exactly those servers plus the corpus, ozy indexes the same set, and `surface.json` enumerates the same set — from one declaration

#### Scenario: Existing scenario keeps working

- **WHEN** `suspended-account-invoice-regression` declares its seven toolsets with the corpus attached
- **THEN** it runs end-to-end with unchanged task behavior and grading semantics

### Requirement: Estate-scale failures are recorded findings

The harness SHALL record a named failure reason (`toolset_rejected`,
`context_overflow`) when the model or gateway cannot operate a mode at the
scenario's estate size, aggregates SHALL count failure reasons, and the
comparison SHALL state the finding. The harness SHALL NOT reduce the estate for
one mode to make it runnable.

#### Scenario: Gateway rejects the direct toolset

- **WHEN** every direct-mode request fails because the gateway rejects the tool definitions
- **THEN** each run records `toolset_rejected`, the invocation still exits 0 with all artifacts, and `comparison.md` states that direct mode could not operate at this estate size

#### Scenario: Estate is never shrunk asymmetrically

- **WHEN** direct mode cannot run at the declared estate size
- **THEN** ozy mode still runs against the full estate and no mode-specific corpus reduction exists in the harness

### Requirement: Per-run invocation log wiring

The harness SHALL provide every fixture server in both modes with a per-run
invocation log path via `OZY_BENCH_CALL_LOG` — through the generated agent
config in direct mode and through the downstream config in ozy mode — so each
run directory contains a complete server-side record of downstream tool calls.

#### Scenario: Both modes produce the log

- **WHEN** a direct-mode run and an ozy-mode run each complete
- **THEN** each run directory contains an invocation log written by the fixture servers themselves, including calls made through Ozy's `call_tool`
