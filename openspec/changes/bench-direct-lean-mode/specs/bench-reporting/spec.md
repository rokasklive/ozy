## MODIFIED Requirements

### Requirement: Cross-mode comparison with verdict

An `all` (or `both`) invocation SHALL write `comparison.json` and a human-readable
`comparison.md` combining the surface tier and every run mode's aggregates:
startup tool count and schema tokens, success rates, duration, token totals,
retrieval quality, and per-metric deltas, plus a descriptive verdict block stating
the headline facts. When `direct-lean` ran alongside `ozy`, the comparison SHALL
present the **ozy − direct-lean** delta as the primary real-world comparison, and
SHALL still present the ozy − direct delta when `direct` also ran. The live-tier
table SHALL carry a column per run mode and a delta column for every metric row
whenever the compared modes have aggregates — the same treatment the surface table
gets. When a mode has no aggregate, its column SHALL be labeled with the reason
(e.g. "not run (MODE=ozy)") and the verdict SHALL state why the delta is
unavailable — never a bare "—". The comparison SHALL label token sources and
reference the run's provenance. The comparison SHALL NOT apply pass/fail
thresholds to live metrics.

#### Scenario: Comparison answers the product question

- **WHEN** an `all` run completes
- **THEN** `comparison.md` shows, side by side with deltas, the startup surface, success k/N per mode, token economy, duration, and retrieval quality for `direct`, `direct-lean`, and `ozy`, with a verdict paragraph a reader can quote without opening any other file

#### Scenario: Real-world delta is explicit

- **WHEN** `direct-lean` and `ozy` both have aggregates
- **THEN** the verdict and the live-tier table show the ozy − direct-lean delta (success in percentage points, counts and means as signed differences) as the primary comparison, and the ozy − direct delta is also shown when `direct` ran

#### Scenario: Lean mode shows structurally near-zero distractor calls

- **WHEN** the `direct-lean` aggregate is reported
- **THEN** its distractor-call metric reflects that no corpus servers were wired (calls can only reach the functional toolsets), so the comparison attributes its token economy to the functional servers' full tool surface rather than to corpus avoidance

#### Scenario: Missing mode is explained

- **WHEN** the invocation ran only ozy mode
- **THEN** the direct and direct-lean columns are labeled "not run" with the invocation's mode setting, and the verdict states that the live delta is unavailable because those modes were not run

#### Scenario: Surface-only comparison

- **WHEN** the invocation is surface-only
- **THEN** `comparison.{json,md}` is still written from `surface.json` alone (covering all three surfaces), with live sections marked skipped
