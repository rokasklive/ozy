## ADDED Requirements

### Requirement: Two-mode identical-environment execution

The harness SHALL run a scenario in `direct`, `ozy`, or `both` modes such that the
scenario definition, task prompt, fixture repository, model endpoint, and process
environment are identical across modes and only the agent's MCP wiring differs.
The `bench-runner` entrypoint SHALL accept `--scenario <name>` and
`--mode direct|ozy|both`.

#### Scenario: Both mode runs each side from one fixture

- **WHEN** `bench-runner --scenario suspended-account-invoice-regression --mode both` is invoked
- **THEN** the harness runs the `direct` side and the `ozy` side against the same generated fixture, prompt, model endpoint, and environment, and produces a comparison

#### Scenario: Only wiring differs between modes

- **WHEN** the harness prepares the `direct` and `ozy` runs
- **THEN** the task prompt, model endpoint, and fixture are byte-identical for both, recorded as a single scenario hash, and the only difference is the agent's MCP configuration file

### Requirement: Mode wiring contract

In `direct` mode the agent SHALL be configured with every fixture MCP server
exposed directly. In `ozy` mode the agent SHALL be configured with only the Ozy
MCP server, and Ozy SHALL be configured with the same fixture servers as its
downstream and indexed before the agent runs.

#### Scenario: Direct mode exposes the full surface

- **WHEN** the agent starts in `direct` mode
- **THEN** the tools of all fixture MCP servers (useful and distractor) are advertised to the agent at startup

#### Scenario: Ozy mode exposes only the broker

- **WHEN** the agent starts in `ozy` mode
- **THEN** the agent is advertised only Ozy's stable interface (`findTool`/`describeTool`/`callTool`), and the same fixture servers are reachable as Ozy downstream after `ozy index`

### Requirement: Mode extension via config templates

The orchestrator SHALL resolve a mode name to an agent config template at
`bench/configs/opencode.<mode>.jsonc`, so adding a mode variant requires a new
config file and no orchestrator code change. Mode-specific setup hooks (today:
`ozy` writes its config and indexes) SHALL be keyed by mode name.

#### Scenario: New mode is a data change

- **WHEN** a new config template `bench/configs/opencode.<variant>.jsonc` is added and the variant is passed as `--mode`
- **THEN** the harness runs it through the same runner, run layout, and reporting with no Go changes beyond an optional named setup hook

### Requirement: OpenCode built-in model selection

The live tier SHALL drive one of OpenCode's built-in models, selected by model ID
via `BENCH_MODEL`, with a free model as the built-in default (e.g. DeepSeek V4
Flash Preview or Nemotron 3) — no provider endpoint, no API-key plumbing, no
provider package, and no auth file baked into the harness. Any model ID the
pinned OpenCode resolves SHALL work. An unresolvable model ID SHALL fail fast
with a named error before any live run is attempted.

#### Scenario: Zero-config free model

- **WHEN** `make bench` runs with no model configuration
- **THEN** both modes drive the default free OpenCode model and the model ID is recorded in provenance

#### Scenario: Model override is one variable

- **WHEN** `BENCH_MODEL` names another model OpenCode can resolve
- **THEN** the live tier uses it with no other configuration change

#### Scenario: Unresolvable model fails fast

- **WHEN** `BENCH_MODEL` names a model the pinned OpenCode cannot resolve
- **THEN** the invocation exits non-zero with a named error before any live run starts

### Requirement: One-command invocation

A single documented command (`make bench`) SHALL build the image, bring up the
harness, generate the fixture, run the configured modes and runs, write all
reports, and exit with the harness exit code — with working defaults requiring no
configuration at all (a free built-in model is the default). A companion
`make bench-surface` SHALL run the static surface tier natively with no Docker
and no model, and `make bench-clean` SHALL prune `bench/runs/`.

#### Scenario: Zero-to-report in one command

- **WHEN** `make bench` is run on a clean checkout
- **THEN** with no other setup the invocation ends with `comparison.md` on the host and the command's exit code reflecting harness completion

#### Scenario: Surface tier needs nothing

- **WHEN** `make bench-surface` is run on a clean checkout
- **THEN** it produces the surface comparison without Docker or any model

### Requirement: Hermetic runtime

After image build, a benchmark run SHALL perform no network access other than
OpenCode's model gateway for the selected model: the agent binary, ozy, ozy-bench,
and any state the agent needs to start are all resolved at build time and pinned.
Hermeticity SHALL be enforced at build time by a smoke check that fails the image
build if the agent cannot reach its ready-to-run state without package-registry
access.

#### Scenario: No runtime package fetches

- **WHEN** a live run executes with the npm registry unreachable
- **THEN** the agent starts from build-time state and the run completes normally, with no package fetches

#### Scenario: Broken build-time state fails the build, not the run

- **WHEN** an OpenCode version bump changes what the agent needs at startup
- **THEN** the image build fails with a named error before any benchmark is attempted

### Requirement: Token accounting from agent-reported usage

Per-run token metrics SHALL be taken from the usage events the pinned OpenCode
emits in its JSON output (labeled `token_source: "measured"`). When a run's
transcript carries no usable usage events, token metrics SHALL be computed from
the transcript plus enumerated startup schemas and labeled
`token_source: "estimated"`. Missing usage events MUST NOT fail a run.

#### Scenario: Usage comes from the agent

- **WHEN** a run's transcript contains OpenCode usage events
- **THEN** its token metrics reflect those events and carry `token_source: "measured"`

#### Scenario: Missing usage degrades to estimation

- **WHEN** a run's transcript carries no usable usage events
- **THEN** the run completes, its token metrics derive from transcript plus schema enumeration, and every affected artifact carries `token_source: "estimated"`

### Requirement: GitHub Actions operation

The surface tier SHALL run as a gating CI job on pull requests (native, no Docker,
no model, no secrets). The live tier SHALL be runnable as a manually dispatched
GitHub Actions workflow that takes mode/runs/scenario/model inputs, requires no
model secrets for the default free model (any credential a non-default model
needs is supplied as a single optional secret), publishes `comparison.md` to the
workflow job summary, and uploads the full run directory as a build artifact. The
live workflow MUST be bounded by a job timeout and a concurrency group, and MUST
NOT be a required check or triggered by pull requests.

#### Scenario: Surface gates a pull request

- **WHEN** a pull request runs CI
- **THEN** the surface job produces the surface comparison natively and fails the check only on harness failure, with no Docker, model, or secrets involved

#### Scenario: Live bench on demand

- **WHEN** the live workflow is dispatched with default inputs and no secrets configured
- **THEN** the compose stack runs both modes against the default free model, the job summary shows `comparison.md`, and the run directory is downloadable as a workflow artifact

#### Scenario: Live workflow cannot gate

- **WHEN** the live workflow fails a task (agent loses the benchmark) or is not dispatched
- **THEN** no pull request or merge is blocked by it

### Requirement: Aligned defaults with single precedence order

Defaults SHALL be `MODE=both`, `BENCH_RUNS=5`, and
`SCENARIO=suspended-account-invoice-regression`, defined once and documented
identically in compose and README. Effective values SHALL resolve as: CLI flag >
environment variable > scenario config > built-in default.

#### Scenario: Defaults agree everywhere

- **WHEN** the README, compose file, and `ozy-bench run --help` are compared
- **THEN** they state the same defaults, and a bare `make bench` runs both modes five times each

#### Scenario: Precedence is deterministic

- **WHEN** `--runs 3` is passed while `BENCH_RUNS=7` is set and the scenario config says 5
- **THEN** the harness performs 3 runs per mode

### Requirement: Recorded run provenance

Each run SHALL write an `environment.json` recording the model ID, run
timestamp, Ozy version or git SHA, OpenCode version, the token estimator name,
whether usage was agent-reported or estimated, scenario hash, mode(s), and
number of runs. No credential of any kind SHALL appear in provenance.

#### Scenario: Provenance is recorded and credential-free

- **WHEN** a run completes
- **THEN** `environment.json` contains the model/runtime/tooling metadata and no credential or token of any kind

#### Scenario: Scenario hash tracks the scenario

- **WHEN** the scenario definition or prompt changes
- **THEN** the recorded scenario hash changes, so runs are comparable only within the same scenario version

### Requirement: Run directory lifecycle

Each invocation SHALL create a run directory `bench/runs/<timestamp>-<scenario>/`
containing the scenario snapshot, `environment.json`, the once-computed
`surface.json`, and a per-mode subdirectory. Each per-mode subdirectory SHALL
contain one `run-<i>/` directory per live run (`transcript.jsonl`,
`tool-calls.jsonl`, `metrics.json`, `final-answer.md`, `grading.json`,
`task.md`) and an
`aggregate.json`, plus top-level `comparison.json` and `comparison.md`. Per-run
artifacts SHALL be written directly into `run-<i>/` with no additional nesting.
`bench/runs/` SHALL be git-ignored.

#### Scenario: Both mode produces a full run directory

- **WHEN** a `both` run with N live runs completes
- **THEN** the run directory contains `direct/` and `ozy/` subdirectories, each with `run-1/`…`run-N/` per-run artifacts and an `aggregate.json`, plus top-level `surface.json`, `comparison.json`, and `comparison.md`

#### Scenario: No doubled run nesting

- **WHEN** run 1 of ozy mode completes
- **THEN** its transcript is at `ozy/run-1/transcript.jsonl`, not under a further `ozy-run-1/` subdirectory

### Requirement: Repeated runs with isolation

The harness SHALL run the live tier a configurable number of times per mode
(default 5; via `--runs`, scenario config, or `BENCH_RUNS`) and report per-run
results plus aggregates. Each run MUST be an independent sample: the agent and Ozy
state SHALL be reset between runs (fresh ephemeral agent session and Ozy state dir,
re-indexed) so no run carries state or answers from a prior run, and the fixture
SHALL be treated as immutable across runs. The deterministic surface tier SHALL be
computed once regardless of run count.

#### Scenario: Runs are independent

- **WHEN** the harness performs run `i+1` for a mode
- **THEN** it starts from a fresh agent session and a fresh Ozy state with no memory, catalog, or artifacts carried over from run `i`

#### Scenario: Surface measured once

- **WHEN** N live runs are requested
- **THEN** the startup surface is enumerated a single time while each of the N live runs is executed and recorded separately

### Requirement: Hands-off operation

The benchmark SHALL be operable entirely from outside the container: configuration
via environment / `.env` and Compose, with no need to attach to or enter the
container. The runner SHALL stream progress and a periodic heartbeat to stdout, and
each run SHALL be bounded by a hard timeout so a hung agent is recorded as
`timed_out` and the remaining runs still execute; an unresolvable model SHALL
fail fast with a clear message. Run artifacts SHALL be written to a
host-accessible (bind-mounted) location.

#### Scenario: Progress is visible and a hang is bounded

- **WHEN** a run exceeds its configured `timeout_seconds`
- **THEN** that run is terminated and recorded as `timed_out`, the batch continues with the next run, and progress for each run is visible in the streamed logs

#### Scenario: Report lands on the host

- **WHEN** a run completes under Compose
- **THEN** the run directory and `comparison.md` are available on the host filesystem without entering the container

### Requirement: Agent output parsing fails loudly

Transcript parsing SHALL self-check: a non-empty transcript that yields zero
parsed events (no text, no tool calls) SHALL mark the run failed with a named
parse error rather than grading an empty answer, so an agent output-format drift
is caught as a harness failure, not scored as an agent failure.

#### Scenario: Format drift is a harness error

- **WHEN** the pinned agent's transcript format changes such that no events parse
- **THEN** the run's `metrics.json` records a parse failure distinct from task failure, and the comparison marks the affected mode's data as invalid

### Requirement: Static surface tier independent of any model

The harness SHALL be able to enumerate and compare the startup tool/schema surface
of both modes without contacting any model or agent, and emit a surface-only
comparison.

#### Scenario: Surface comparison runs with no model

- **WHEN** the harness is run in surface-only mode (`make bench-surface` or `--surface-only`)
- **THEN** it starts each mode's MCP wiring, counts the tools and schema bytes/tokens advertised at startup, and writes a comparison showing the direct-vs-ozy surface delta, with live metrics marked skipped
