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

### Requirement: OpenAI-compatible model endpoint configuration

The model connection SHALL be configured entirely through environment variables
(`MODEL_BASE_URL`, `MODEL_API_KEY`, `MODEL_NAME`, `MODEL_TEMPERATURE`,
`MODEL_MAX_TOKENS`) and/or scenario config, with no model provider baked into the
harness or image. The harness SHALL work against any OpenAI-compatible endpoint.

#### Scenario: Endpoint comes from the environment

- **WHEN** `MODEL_BASE_URL`/`MODEL_NAME` point at any OpenAI-compatible endpoint (Ollama, LM Studio, llama.cpp, vLLM, OpenRouter, …)
- **THEN** both modes drive that endpoint with the configured temperature and max tokens, and nothing else is required to select a provider

#### Scenario: Missing endpoint degrades, does not crash

- **WHEN** no model endpoint is configured
- **THEN** the live agent tier is skipped (recorded as skipped) and the static surface tier still runs and produces a comparison

### Requirement: Recorded run provenance

Each run SHALL write an `environment.json` recording the model name, the
**sanitized** base URL (no secrets/API key), temperature, context window if
known, max output tokens, run timestamp, Ozy version or git SHA, scenario hash,
mode(s), and number of runs.

#### Scenario: Provenance is recorded and sanitized

- **WHEN** a run completes
- **THEN** `environment.json` contains the model/runtime metadata with the base URL sanitized and no API key present

#### Scenario: Scenario hash tracks the scenario

- **WHEN** the scenario definition or prompt changes
- **THEN** the recorded scenario hash changes, so runs are comparable only within the same scenario version

### Requirement: Run directory lifecycle

Each invocation SHALL create a run directory `bench/runs/<timestamp>-<scenario>/`
containing the scenario snapshot, `environment.json`, the once-computed
`surface.json`, and a per-mode subdirectory. Each per-mode subdirectory SHALL
contain one `run-<i>/` directory per live run (`transcript.jsonl`,
`context-ledger.jsonl`, `tool-calls.jsonl`, `metrics.json`, `final-answer.md`,
`grading.json`) and an `aggregate.json`, plus top-level `comparison.json` and
`comparison.md`. `bench/runs/` SHALL be git-ignored.

#### Scenario: Both mode produces a full run directory

- **WHEN** a `both` run with N live runs completes
- **THEN** the run directory contains `direct/` and `ozy/` subdirectories, each with `run-1/`…`run-N/` per-run artifacts and an `aggregate.json`, plus top-level `comparison.json` and `comparison.md`

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
`timed_out` and the remaining runs still execute; an unreachable model endpoint
SHALL fail fast with a clear message. Run artifacts SHALL be written to a
host-accessible (bind-mounted) location.

#### Scenario: Progress is visible and a hang is bounded

- **WHEN** a run exceeds its configured `timeout_seconds`
- **THEN** that run is terminated and recorded as `timed_out`, the batch continues with the next run, and progress for each run is visible in the streamed logs

#### Scenario: Report lands on the host

- **WHEN** a run completes under Compose
- **THEN** the run directory and `comparison.md` are available on the host filesystem without entering the container

### Requirement: Static surface tier independent of any model

The harness SHALL be able to enumerate and compare the startup tool/schema surface
of both modes without contacting any model endpoint or agent, and emit a
surface-only comparison.

#### Scenario: Surface comparison runs with no model

- **WHEN** the harness is run in a surface-only mode (or no endpoint is configured)
- **THEN** it starts each mode's MCP wiring, counts the tools and schema bytes/tokens advertised at startup, and writes a comparison showing the direct-vs-ozy surface delta, with live metrics marked skipped
