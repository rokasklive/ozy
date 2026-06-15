# eval-cache-effectiveness

## Purpose

Evaluate the effectiveness of Ozy's result cache by measuring redundant-call reduction ratio and tokens-to-success delta over a deterministic repeated-call workload.

## Requirements

### Requirement: Eval harness exercises the result cache

The eval harness SHALL be able to construct its corpus broker through the shared result-cache decorator, so a run measures the cached broker's behavior rather than only the uncached live broker. The wrapping SHALL reuse the same cache decorator used in production, not a test-only reimplementation.

#### Scenario: Cache family runs through a cache-wrapped broker

- **WHEN** the `cache` eval family runs
- **THEN** its workload is driven through a broker produced by the production cache decorator over the corpus broker

#### Scenario: Other families are unaffected

- **WHEN** the discovery, invocation, ergonomics, tokens, or performance families run
- **THEN** their measurements are unchanged by the addition of the cache family

### Requirement: Repeated-call cache workload

The `cache` family SHALL drive a deterministic workload that issues repeated `findTool`, `describeTool`, and read-only `callTool` requests with identical inputs, so the cache produces hits. Write tools in the workload SHALL be invoked live every time. The workload SHALL be fixed and corpus-derived so the metric is reproducible across runs.

#### Scenario: Repeated read-only requests hit the cache

- **WHEN** the workload issues the same `findTool` query, `describeTool` toolRef, or read-only `callTool` (same toolRef and arguments) more than once
- **THEN** the repeated requests are served from cache rather than re-executed against search or the downstream fixture

#### Scenario: Repeated write requests never hit the cache

- **WHEN** the workload issues the same write tool `callTool` more than once
- **THEN** every invocation executes live and none is served from cache

#### Scenario: Workload is deterministic

- **WHEN** the `cache` family runs twice over the same corpus
- **THEN** it reports identical cache-effectiveness numbers

### Requirement: Redundant-call-reduction metric

The `cache` family SHALL compute, deterministically, the fraction of broker operations served from cache over the workload (redundant-call-reduction ratio) and the cached-versus-uncached tokens-to-success for the workload, using the run's token estimator. The reduction ratio SHALL be derived by counting operations delegated to the underlying broker versus the total operations issued.

#### Scenario: Reduction ratio reflects avoided work

- **WHEN** the workload issues N cacheable operations of which H are served from cache
- **THEN** the reported redundant-call-reduction ratio equals H / N

#### Scenario: Tokens-to-success delta is reported

- **WHEN** the cache family completes
- **THEN** it reports tokens-to-success for the workload with the cache enabled and with it disabled, so the saving is visible

### Requirement: Cache effectiveness gate

The harness SHALL gate the redundant-call-reduction ratio with a data-driven threshold `cache.redundantCallReductionMin` read from `thresholds.json`. When the metric is below the threshold the run verdict SHALL fail; when the `cache` family did not run, the gate SHALL be skipped (not failed), consistent with the other families.

#### Scenario: Gate fails when reduction is below threshold

- **WHEN** the redundant-call-reduction ratio is below `cache.redundantCallReductionMin`
- **THEN** the run verdict is FAIL and the gate is listed as failed

#### Scenario: Gate is skipped when the family did not run

- **WHEN** a run scopes families to exclude `cache`
- **THEN** the cache gate is reported as skipped and does not affect the verdict

### Requirement: Scoreboard reports cache effectiveness

The generated `BENCHMARKS.md` scoreboard SHALL include a cache section rendered from the metric (reduction ratio, cached vs. uncached tokens-to-success, operation counts), so the document never drifts from the measured numbers.

#### Scenario: Scoreboard shows the cache section

- **WHEN** a run that includes the `cache` family writes the scoreboard
- **THEN** `BENCHMARKS.md` contains a cache section with the reduction ratio and the cached/uncached tokens-to-success

### Requirement: Corpus carries read-only tool intent

Corpus catalog tools SHALL support a `readOnly` annotation that maps to `catalog.Tool.ReadOnly`, so the eval can exercise read-only `callTool` caching and keep write tools correctly excluded. Tools that omit the annotation SHALL default to not-read-only.

#### Scenario: Read-only corpus tool is cacheable

- **WHEN** a corpus tool sets `readOnly: true`
- **THEN** the loaded catalog entry has `ReadOnly == true` and a read-only `callTool` to it can be served from cache

#### Scenario: Omitted annotation defaults to write-safe

- **WHEN** a corpus tool omits `readOnly`
- **THEN** the loaded catalog entry has `ReadOnly == false` and `callTool` to it is never cached
