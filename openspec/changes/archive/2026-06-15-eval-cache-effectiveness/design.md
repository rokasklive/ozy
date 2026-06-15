## Context

`ozy eval run` drives families (`discovery`, `invocation`, `ergonomics`,
`tokens`, `performance`) over the embedded corpus, attaches each family's report
to `RunResult`, evaluates data-driven gates from `thresholds.json`, and renders
`BENCHMARKS.md` from the result (`internal/eval/run.go`, `report.go`, `gate.go`).
Brokers are built per-leg via `newCorpusBroker` → `broker.NewLive` over an
in-process fixture connector; the token-economy leg already does a single
`find→describe→call` (`economy.go`).

The result cache (`broker.NewCaching`, from `cache-calltool-results`) lives on the
shared seam, but the eval never wraps with it, and the corpus never repeats a
request — so the cache is invisible to the bench.

## Goals / Non-Goals

**Goals:**
- A deterministic `cache` family that exercises the production cache decorator and
  reports a gated redundant-call-reduction ratio plus a cached/uncached
  tokens-to-success delta.
- Read-only corpus annotations so the read-only `callTool` cache path is measured
  and write tools stay excluded.

**Non-Goals:**
- Real agent-trace replay. The workload is a fixed, corpus-derived sequence.
- Caching in the latency leg (latency is reported, not gated; the cache family
  owns the cache measurement).
- Changing the cache implementation. This change only measures it.

## Decisions

### Decision: Measure avoided work with a counting broker beneath the cache

Build the family broker as `cache( counter( corpusLive ) )`. The counter is a tiny
`Broker` decorator that increments per delegated call. Run the workload; the
counter only advances on cache misses. Then:

```
reductionRatio = 1 - delegated / totalCacheableOps
```

This measures real cache effectiveness without instrumenting `cachingBroker`
internals, and it is exactly the shape the unit tests already validate.

*Alternatives:* add hit/miss counters to `cachingBroker` and export them — more
invasive and couples the production type to eval needs.

### Decision: A fixed, corpus-derived repeated-call workload

For a representative read-only tool (e.g. `confluence.search_pages`), the workload
issues each of `findTool(q)`, `describeTool(ref)`, `callTool(ref, args)` twice,
and a write tool's `callTool` twice. Cacheable ops = the read-only repeats; the
second occurrence of each read-only op must be a hit, and both write calls must be
misses. The sequence is constant, so the metric is reproducible
(`Scenario: Workload is deterministic`).

### Decision: Cached vs. uncached tokens-to-success

Run the same workload twice: once through `corpusLive` (uncached) and once through
`cache(counter(corpusLive))`. Sum response tokens via the run's `TokenEstimator`
(reuse `estimateJSON`). Uncached pays for every op; cached pays only for misses.
The delta is the token saving the scoreboard reports.

### Decision: New family, report, gate, and scoreboard section — following the existing pattern

- `run.go`: `FamilyCache = "cache"`, add to `knownFamilies`, run when in scope,
  attach `result.Cache`.
- New `CacheReport`/`CacheEffectivenessMetrics` (reduction ratio, cached/uncached
  tokens-to-success, total/delegated op counts).
- `gate.go`: `CacheGate{ RedundantCallReductionMin *float64 }`, `EvaluateCache`,
  appended in `Run` like the others; skipped when the family didn't run.
- `report.go`: `writeCacheSection`.
- `thresholds.json`: a `cache` block with `redundantCallReductionMin` set
  conservatively (≈0.40) — a regression guard that the cache stays wired and
  effective, not a stretch target.
- `internal/cli`: add `cache` to `knownFamilies` validation (the `[family]` flag
  is already generic).

### Decision: Read-only intent in the corpus

Add `ReadOnly bool` (`json:"readOnly,omitempty"`) to `CatalogTool`, map it in
`Corpus.Store()` to `catalog.Tool.ReadOnly`, and set `readOnly: true` on the
naturally read-only `world.json` tools used by the workload. Omitted → false
(write-safe default).

## Risks / Trade-offs

- **Corpus tools have empty `SchemaHash`** (`Store()` doesn't compute one) → the
  describe/call cache key's content token is empty, but keys still include the
  toolRef so caching works for the workload (which never re-indexes). Acceptable;
  noted so a future schema-drift cache test doesn't assume otherwise.
- **Synthetic workload, not real traces** → the metric is a floor that proves the
  cache is wired and hits on repeats; it is not a model of production hit-rate.
  Framed as such in METHODOLOGY/scoreboard.
- **Default `ozy eval run` now includes the cache family** → the committed
  baseline gains a cache section on next regen. Expected; the gate is conservative
  so it won't flake.

## Migration Plan

Additive: a new family and a new gate. No change to existing metrics. Regenerating
the committed baseline (`--out evals`) adds the cache section. Rollback is removing
the family wiring and the `cache` thresholds block.

## Open Questions

- Exact `redundantCallReductionMin` value — start at 0.40 and ratchet once the
  measured number is known. Not blocking.
