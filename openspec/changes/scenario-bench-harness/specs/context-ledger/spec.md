## ADDED Requirements

### Requirement: Itemized JSONL context ledger

The harness SHALL emit a JSONL context ledger of itemized content, one object per
line, with at least the fields `run_id`, `mode`, `phase`, `source`, `kind`,
`server` (optional), `tool` (optional), `bytes`, a token count, `token_source`
(`measured` or `estimated`), and `included_in_model_context`. The `kind` field
SHALL cover `system_prompt`, `agent_instruction`, `tool_schema`, `tool_call`,
`tool_result`, `assistant_message`, `user_message`, `error`, and `final_answer`.

#### Scenario: Startup schemas are itemized

- **WHEN** a mode starts up
- **THEN** the ledger contains a `tool_schema` item per advertised tool, attributed to its server, with bytes and a token count

#### Scenario: Every item declares its token source

- **WHEN** any ledger item is written
- **THEN** it carries `token_source` of `measured` or `estimated`, and the run records which token estimator produced estimated counts

### Requirement: ContextSpy capture as an optional backend

When ContextSpy is available the ledger SHALL be populated from its captured
on-the-wire requests and marked `measured`; when ContextSpy or the endpoint is
unavailable the harness SHALL fall back to enumerated startup schemas plus the
agent transcript, marked `estimated`, and still produce the ledger. ContextSpy
MUST NOT be a hard runtime dependency of the harness.

#### Scenario: Measured ledger when ContextSpy is present

- **WHEN** the live agent tier runs with ContextSpy capturing model requests
- **THEN** ledger items reflect the real on-the-wire prompt composition and are marked `measured`

#### Scenario: Estimated ledger when ContextSpy is absent

- **WHEN** ContextSpy is not available
- **THEN** the run still completes and emits an `estimated` ledger from enumerated schemas and the transcript

### Requirement: Tool-surface and behavior metrics

`metrics.json` per mode SHALL report the number of tools visible at startup, the
number of tool schemas visible at startup, tool calls made, useful calls,
irrelevant/distractor calls, failed calls, calls before first useful evidence, and
whether a forbidden or public-internet tool was used.

#### Scenario: Startup tool visibility differs by mode

- **WHEN** direct and ozy metrics are compared
- **THEN** the ozy startup tools-visible count reflects only the broker interface while direct reflects the full fixture surface

#### Scenario: Distractor calls are counted as irrelevant

- **WHEN** the agent calls a distractor tool during a run
- **THEN** that call is counted in the irrelevant/distractor tool-call metric

### Requirement: Token-economy metrics

`metrics.json` SHALL report startup schema bytes/tokens, total input and output
tokens when available, tokens before the first useful tool call, tokens to
success, the largest single context item, the largest tool result, irrelevant
schema tokens exposed, and Ozy discovery/describe overhead; the comparison SHALL
report the direct-vs-ozy delta. All token numbers SHALL carry their token source.

#### Scenario: Startup token reduction is reported

- **WHEN** the comparison is produced
- **THEN** it reports startup schema tokens for each mode and the startup reduction ratio between them

#### Scenario: Broker overhead is accounted

- **WHEN** the ozy mode runs the live tier
- **THEN** the tokens spent on `findTool`/`describeTool` discovery are recorded as Ozy discovery/describe overhead

### Requirement: Deterministic success grading

`grading.json` SHALL score the final answer and the tool-call log against
`ground_truth.json` — root cause, source file, function, culprit commit (matched
by subject or recorded hash), patch target, regression test — and the forbidden
behaviors (no public internet, no architecture redesign, no unrelated refactor),
producing an overall pass/fail using no model.

#### Scenario: Correct, well-behaved answer passes

- **WHEN** the final answer names the correct file, function, culprit commit, patch target, and regression test, and no forbidden or distractor tool was used
- **THEN** grading records each criterion as met and the overall verdict as pass

#### Scenario: Forbidden tool use fails the rubric

- **WHEN** the agent used a public-internet or otherwise forbidden tool during the run
- **THEN** the forbidden-behavior check fails and the overall verdict reflects it, independent of answer correctness

### Requirement: Per-run and aggregate comparison artifacts

The harness SHALL emit a machine-readable `comparison.json` and a human-readable
`comparison.md` summarizing both modes side by side — startup tools visible,
startup schema tokens, total estimated input tokens, irrelevant tool calls, and
success per mode — with the direct-vs-ozy deltas, suitable for CI artifacts or
docs. When the live tier runs N times per mode, the comparison SHALL report both
the per-run breakdown and aggregates: mean, min, max, and standard deviation for
the numeric metrics, and a success rate `k/N` per mode. Aggregation SHALL be plain
arithmetic (no confidence intervals or significance tests in v0).

#### Scenario: Headline comparison is human-readable

- **WHEN** a `both` run completes
- **THEN** `comparison.md` shows the headline metrics for direct and ozy side by side, with both per-run values and aggregates, the success rate `k/N` per mode, and the direct-vs-ozy deltas

#### Scenario: Aggregates summarize N runs

- **WHEN** N live runs per mode have completed
- **THEN** `comparison.json` reports per-run metric values plus the mean/min/max/stdev and the `k/N` success rate for each mode

#### Scenario: Comparison is diffable across runs

- **WHEN** two `comparison.json` files from different benchmark invocations are compared
- **THEN** corresponding metrics can be diffed field-by-field
