## ADDED Requirements

### Requirement: Machine-readable run snapshot

Each eval run SHALL write a machine-readable JSON snapshot capturing every metric,
the run provenance (corpus version, embedding model, git commit, timestamp, and
whether the semantic leg ran), and the pass/fail verdict, in a stable shape so
snapshots can be diffed and trends tracked across runs over time.

#### Scenario: Snapshot is emitted per run

- **WHEN** an eval run completes
- **THEN** a JSON snapshot containing all metrics, provenance, and the verdict is written in the stable documented shape

#### Scenario: Snapshots are comparable across runs

- **WHEN** two snapshots from different commits are compared
- **THEN** corresponding metrics can be diffed field-by-field to show regression or improvement

### Requirement: Human-readable public benchmark scoreboard

The suite SHALL produce a human-readable public benchmark document
(`evals/BENCHMARKS.md`) that summarizes the headline metrics per eval family
(discovery, invocation/repair, agent ergonomics, token economy, performance) in a
form a reader can consume at a glance — tables with the current numbers and their
gate thresholds. The scoreboard MUST be readable without understanding the metric
mathematics.

#### Scenario: Scoreboard summarizes every family

- **WHEN** a reader opens the public benchmark document
- **THEN** they see, per eval family, the headline metric values and the threshold each must meet, without needing the underlying formulas

#### Scenario: Scoreboard reflects the latest run

- **WHEN** the benchmark document is regenerated from a run snapshot
- **THEN** its numbers match that snapshot and it records which corpus version, model, and commit produced them

### Requirement: Methodology separated from public benchmarks

The suite SHALL keep the detailed metric mathematics — formulas, the RRF `k` and
relevance-floor calibration, confidence intervals, and judge rubrics — in a
separate methodology document (`evals/METHODOLOGY.md`) and out of the public
scoreboard, so the public surface stays easy to consume while the rigorous detail
remains available and versioned.

#### Scenario: Math lives in the methodology document

- **WHEN** a reader wants the exact definition of a metric or the floor calibration
- **THEN** they find it in the methodology document, while the public scoreboard links to it rather than inlining the math

### Requirement: Threshold gates

The suite SHALL define explicit, versioned pass/fail thresholds for the tracked
headline metrics and evaluate each run against them, so an eval run is a clear
signal rather than a wall of numbers. Thresholds MUST be stored as data so they
can be ratcheted as the system improves, and a run whose metrics fall below
threshold MUST be reported as a failure.

#### Scenario: A run is gated

- **WHEN** a run's tracked metrics are evaluated against the configured thresholds
- **THEN** the run is marked pass only if every gated metric meets or exceeds its threshold, and fail otherwise

#### Scenario: Thresholds are data, not code

- **WHEN** a threshold is ratcheted upward after an improvement
- **THEN** the change is made in the threshold data file without modifying harness code

### Requirement: Reproducible benchmark provenance

Every published benchmark artifact SHALL be stamped with the inputs needed to
reproduce it — corpus version, embedding model id, git commit, and the host/run
context that materially affects performance numbers — so a reader can tell exactly
what produced a given number and re-run it.

#### Scenario: Numbers are attributable

- **WHEN** a benchmark number is read from the scoreboard or a snapshot
- **THEN** the corpus version, model, and commit that produced it are recorded alongside it
