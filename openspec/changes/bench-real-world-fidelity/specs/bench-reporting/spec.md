## MODIFIED Requirements

### Requirement: Cross-mode comparison with verdict

An `all` (or `both`) invocation SHALL write `comparison.json` and a human-readable
`comparison.md` combining the surface tier and every run mode's aggregates. The
verdict SHALL **lead with the efficiency and outcome metrics** that reflect a
broker's value — startup schema tokens, total tokens/run, tool-call count, duration,
and success k/N — presented side by side with per-metric deltas. Tool-attribution
signals (canonical-tools-hit, distractor-call count) SHALL be reported as
**informational** and SHALL NOT lead the verdict or determine any headline. When
`direct-lean` ran alongside `ozy`, the comparison SHALL present the **ozy −
direct-lean** delta as the primary real-world comparison, and SHALL still present
the ozy − direct delta when `direct` also ran. The live-tier table SHALL carry a
column per run mode and a delta column for every metric row whenever the compared
modes have aggregates. When a mode has no aggregate, its column SHALL be labeled
with the reason (e.g. "not run (MODE=ozy)") and the verdict SHALL state why the
delta is unavailable — never a bare "—". The comparison SHALL label token sources
and reference the run's provenance. The comparison SHALL NOT apply pass/fail
thresholds to live metrics.

#### Scenario: Verdict leads with efficiency and outcome

- **WHEN** an `all` run completes
- **THEN** the verdict paragraph a reader can quote leads with startup schema tokens, total tokens/run, tool-call count, duration, and success k/N per mode, and any canonical-hit / distractor-call figures appear only as informational lines below, never as the headline

#### Scenario: Real-world delta is explicit

- **WHEN** `direct-lean` and `ozy` both have aggregates
- **THEN** the verdict and the live-tier table show the ozy − direct-lean delta (success in percentage points, counts and means as signed differences) as the primary comparison, and the ozy − direct delta is also shown when `direct` ran

#### Scenario: Missing mode is explained

- **WHEN** the invocation ran only ozy mode
- **THEN** the direct and direct-lean columns are labeled "not run" with the invocation's mode setting, and the verdict states that the live delta is unavailable because those modes were not run
