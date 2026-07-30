## ADDED Requirements

### Requirement: Always-on static surface measurement

Every invocation SHALL compute `surface.json` before any live run and without any
model endpoint: for each mode, the advertised tool count, total schema bytes,
estimated schema tokens (estimator recorded), and a per-server breakdown. Direct
mode enumerates the fixture toolsets in-process; ozy mode enumerates the three
broker tool definitions actually served by the MCP adapter.

#### Scenario: Surface is computed on every run

- **WHEN** any benchmark invocation starts (with or without a model endpoint)
- **THEN** `surface.json` exists in the run directory before the first live run begins, and its ozy-mode numbers derive from the real adapter tool definitions

#### Scenario: Estimator is recorded

- **WHEN** `surface.json` is written
- **THEN** it names the token estimator used, so a future estimator swap is visible in the data

### Requirement: Per-run metrics artifact

Each live run SHALL write `metrics.json` containing: `success`, `timed_out`,
`parse_failed`, `duration_sec`, `tool_call_count`, token totals (input, output,
and finer buckets where the source distinguishes them), and `token_source`
(`measured` from OpenCode's agent-reported usage events, `estimated` from
transcript + schema enumeration).

#### Scenario: Measured run

- **WHEN** a run's transcript carries OpenCode usage events
- **THEN** its `metrics.json` token totals match the agent-reported usage and carry `token_source: "measured"`

#### Scenario: Estimated run

- **WHEN** a run completes without usable usage events
- **THEN** `metrics.json` still contains all fields, with totals derived from transcript and schemas and `token_source: "estimated"`

### Requirement: Per-mode aggregates

After a mode's runs complete, the harness SHALL write `aggregate.json` for that
mode: success count `k` of `N`, timed-out and parse-failed counts, and
mean/min/max/stdev for duration and each token total. Aggregates SHALL only pool
runs with the same `token_source`, or report the sources separately.

#### Scenario: Aggregate summarizes the batch

- **WHEN** ozy mode finishes 5 runs with 4 successes
- **THEN** `ozy/aggregate.json` reports `success: 4/5` and statistics over the runs' durations and token totals

#### Scenario: Mixed sources are not silently pooled

- **WHEN** a batch contains 3 measured and 2 estimated runs
- **THEN** the aggregate either reports the two groups separately or labels the pooled statistic as mixed-source

### Requirement: Cross-mode comparison with verdict

A `both` invocation SHALL write `comparison.json` and a human-readable
`comparison.md` combining the surface tier and both modes' aggregates: startup
tool count and schema tokens, success rates, duration, token totals, and per-metric
deltas (ozy relative to direct), plus a descriptive verdict block stating the
headline facts. The comparison SHALL label token sources and reference the run's
provenance. The comparison SHALL NOT apply pass/fail thresholds to live metrics.

#### Scenario: Comparison answers the product question

- **WHEN** a `both` run completes
- **THEN** `comparison.md` shows, side by side, the startup surface delta, success k/N per mode, and token-economy deltas, with a verdict paragraph a reader can quote without opening any other file

#### Scenario: Surface-only comparison

- **WHEN** the invocation is surface-only
- **THEN** `comparison.{json,md}` is still written from `surface.json` alone, with live sections marked skipped

### Requirement: Grading uses the recorded culprit hash

The runner SHALL read the fixture's recorded culprit commit hash from the fixture
metadata artifact and pass it to grading, so the culprit-identification check
grades against the real hash instead of an empty string.

#### Scenario: Culprit check is live

- **WHEN** an agent's final answer names the culprit commit by hash or unambiguous subject
- **THEN** `grading.json` credits the culprit criterion using the fixture-recorded hash

### Requirement: Exit-code semantics

The harness SHALL exit 0 when the invocation completed and produced its promised
artifacts (including surface-only invocations), and non-zero only for harness
errors (fixture generation failure, config errors, no artifacts). Live-tier task
success or failure SHALL be expressed in the reports, never in the exit code.

#### Scenario: Losing the benchmark is not an error

- **WHEN** all ozy-mode runs fail the task but the harness completes and writes the comparison
- **THEN** the invocation exits 0 and the comparison carries the bad news

#### Scenario: Harness failure is loud

- **WHEN** fixture generation fails
- **THEN** the invocation exits non-zero with a named error and writes no misleading partial comparison
