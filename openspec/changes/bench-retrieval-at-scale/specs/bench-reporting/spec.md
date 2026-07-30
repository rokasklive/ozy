## MODIFIED Requirements

### Requirement: Cross-mode comparison with verdict

A `both` invocation SHALL write `comparison.json` and a human-readable
`comparison.md` combining the surface tier and both modes' aggregates: startup
tool count and schema tokens, success rates, duration, token totals, retrieval
quality, and per-metric deltas (ozy relative to direct), plus a descriptive
verdict block stating the headline facts. The live-tier table SHALL carry a
delta column (ozy − direct) for every metric row whenever both modes have
aggregates — the same treatment the surface table gets. When a mode has no
aggregate, its column SHALL be labeled with the reason (e.g. "not run
(MODE=ozy)") and the verdict SHALL state why the live delta is unavailable —
never a bare "—". The comparison SHALL label token sources and reference the
run's provenance. The comparison SHALL NOT apply pass/fail thresholds to live
metrics.

#### Scenario: Comparison answers the product question

- **WHEN** a `both` run completes
- **THEN** `comparison.md` shows, side by side with deltas, the startup surface, success k/N per mode, token economy, duration, and retrieval quality, with a verdict paragraph a reader can quote without opening any other file

#### Scenario: Live deltas are explicit

- **WHEN** both modes have aggregates
- **THEN** every live-tier metric row shows the ozy − direct delta (success in percentage points, counts and means as signed differences)

#### Scenario: Missing mode is explained

- **WHEN** the invocation ran only ozy mode
- **THEN** the direct column is labeled "not run" with the invocation's mode setting, and the verdict states that the live delta is unavailable because direct was not run

#### Scenario: Surface-only comparison

- **WHEN** the invocation is surface-only
- **THEN** `comparison.{json,md}` is still written from `surface.json` alone, with live sections marked skipped

## ADDED Requirements

### Requirement: Retrieval-quality metrics

Each run's `metrics.json` SHALL include, computed from the server-side
invocation log: `canonical_tools_hit` (how many of the scenario's required tools
were called, k of N), `distractor_call_count` (calls to corpus/distractor tools
outside the required set), and `downstream_call_count`. Aggregates SHALL
summarize these per mode, and the comparison SHALL report them with deltas.

#### Scenario: Retrieval quality is measured per run

- **WHEN** a run calls 3 of 4 required tools and 5 corpus stubs
- **THEN** its `metrics.json` reports `canonical_tools_hit: 3/4` and `distractor_call_count: 5`

#### Scenario: Retrieval quality reaches the comparison

- **WHEN** a `both` invocation completes
- **THEN** `comparison.md` shows canonical-tool hit rates and distractor calls per mode with deltas

### Requirement: Named estate-scale failure reasons

Run metrics and aggregates SHALL distinguish `toolset_rejected` and
`context_overflow` failures (classified from the transcript's error output) from
task failures and timeouts, and the comparison SHALL report failure-reason
counts per mode.

#### Scenario: Rejection is not a task failure

- **WHEN** all direct runs fail with gateway tool-definition rejection
- **THEN** aggregates report N × `toolset_rejected` (not task failures), and the comparison names the finding

### Requirement: Retrieval-stack provenance

`environment.json` SHALL record which retrieval stack ozy used
(`retrieval: semantic|lexical`), and the comparison SHALL display it. The value
SHALL reflect what the run actually exercised, verified at build time — never
assumed.

#### Scenario: Retrieval stack is visible

- **WHEN** any invocation completes
- **THEN** `environment.json` names the retrieval stack and `comparison.md` displays it alongside the model ID
