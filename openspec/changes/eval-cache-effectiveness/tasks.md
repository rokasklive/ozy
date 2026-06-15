## 1. Read-only intent in the corpus

- [x] 1.1 Add `ReadOnly bool` with `json:"readOnly,omitempty"` to `CatalogTool` (`internal/eval/dataset.go`).
- [x] 1.2 Map it in `Corpus.Store()` to `catalog.Tool.ReadOnly`.
- [x] 1.3 Set `readOnly: true` on the naturally read-only `world.json` tools the workload uses (e.g. `confluence.search_pages`); confirm at least one write tool remains unannotated for the negative case.
- [x] 1.4 Test: `Corpus.Store()` maps `readOnly` true/false to `catalog.Tool.ReadOnly`, and omitted defaults to false.

## 2. Counting broker and cached corpus broker

- [x] 2.1 Add a small `countingBroker` decorator in the eval package that counts calls delegated to the inner broker.
- [x] 2.2 Add a helper that returns `broker.NewCaching(counter(newCorpusBroker(...)))` plus a handle to the counter, using an always-on `config.CacheConfig` with a large TTL and max so nothing expires mid-workload.

## 3. Cache family, report, and run wiring

- [x] 3.1 New `internal/eval/cache.go`: `CacheEffectivenessMetrics` (reduction ratio, cached/uncached tokens-to-success, total and delegated op counts) and `RunCacheEffectiveness(ctx, corpus, est)`.
- [x] 3.2 Implement the fixed repeated-call workload: a read-only tool's `findTool`/`describeTool`/`callTool` each issued twice, plus a write tool's `callTool` twice; derive the reduction ratio from the counter and the cached vs. uncached tokens-to-success via `estimateJSON`.
- [x] 3.3 Add `FamilyCache = "cache"` to `run.go`: register in `knownFamilies`, run when in scope, attach to `RunResult`.
- [x] 3.4 Add the cache report field to `RunResult` (and ensure it round-trips through the JSON snapshot).
- [x] 3.5 Test: over the fixed workload the reduction ratio equals the expected hit fraction, write-tool calls are never served from cache, and the numbers are identical across two runs (deterministic).

## 4. Gate and scoreboard

- [x] 4.1 `gate.go`: add `CacheGate{ RedundantCallReductionMin *float64 }`, a `Cache` field on `Thresholds`, and `EvaluateCache`; append it in `Run` like the other families and skip it (not fail) when the family did not run.
- [x] 4.2 `evals/data/thresholds.json`: add a `cache` block with `redundantCallReductionMin` set conservatively (~0.40).
- [x] 4.3 `report.go`: add `writeCacheSection` and call it from `Scoreboard` when the cache report is present.

## 5. Verify

- [ ] 5.1 `go test ./...` green and `go vet ./...` clean.
- [ ] 5.2 `ozy eval run cache --out /tmp/ozy-eval-cache` shows the cache section and a passing cache gate; a full `ozy eval run --out /tmp/ozy-eval-full` is still PASS with the new section present.
- [ ] 5.3 Run `graphify update .` to refresh the knowledge graph.
