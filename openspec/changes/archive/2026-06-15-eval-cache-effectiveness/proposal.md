## Why

The `cache-calltool-results` change added a result cache to improve the benchmark
scoreboard, but a full `ozy eval run` shows the cache has zero effect on the
gated numbers — for two independent reasons:

1. **The cache is not in the eval path.** Every eval leg builds its broker with
   `broker.NewLive(...)` directly (`newCorpusBroker`, latency, parity); none go
   through the daemon's `wireBroker`, so `broker.NewCaching` is never applied.
2. **Nothing in the corpus repeats a call.** Each case measures a single-shot
   `findTool`→`describeTool`→`callTool` path (token economy reports exactly 3
   broker calls). A result cache only pays off on the 2nd+ identical request, so
   even a wired-in cache would never hit.

So the cache's value (avoided redundant work) is currently unobservable and
ungated. This change makes it measurable.

## What Changes

- Route the eval corpus broker through the shared cache seam so a run can
  exercise `broker.NewCaching`.
- Add a new `cache` eval family that drives a deterministic **repeated-call /
  multi-step workload** through a cache-wrapped broker layered over a
  call-counting broker, and measures how many broker operations are served from
  cache instead of re-executed.
- Compute a **redundant-call-reduction ratio** (and a cached vs. uncached
  tokens-to-success delta) for the workload, render it in a new scoreboard
  section, and gate it via a data-driven `cache.redundantCallReductionMin`
  threshold.
- Annotate corpus tools with a `readOnly` flag (mapped to `catalog.Tool.ReadOnly`)
  so the read-only `callTool` caching path — not just `findTool`/`describeTool` —
  is exercised, and write tools stay correctly uncached.

## Capabilities

### New Capabilities
- `eval-cache-effectiveness`: a benchmark family that exercises the result cache
  over a repeated-call workload and reports/gates the redundant-work reduction.

### Modified Capabilities
<!-- None: the eval harness has no published spec, and result-cache is still an
     unarchived change, so this introduces a new capability rather than editing
     an existing requirement. -->

## Impact

- Depends on `cache-calltool-results` (the `broker.NewCaching` decorator and
  `catalog.Tool.ReadOnly`).
- `internal/eval`: new `cache.go` leg + report struct; `run.go` family wiring;
  `dataset.go` (`CatalogTool.ReadOnly` + `Store()` mapping); `gate.go`
  (`CacheGate` + `EvaluateCache`); `report.go` (scoreboard section);
  `newCorpusBroker` gains an optional cache wrap.
- `evals/data`: `catalog/world.json` read-only annotations; `thresholds.json`
  cache gate.
- `internal/cli`: register `cache` in `knownFamilies` (the `eval run [family]`
  flag is already generic).
- No new third-party dependencies.
