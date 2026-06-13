## 1. Catalog last-index time

- [x] 1.1 Extend `catalog.Store` with `LastIndexedAt(ctx) (time.Time, bool, error)` and a setter for the run-level index time; implement on the file and in-memory stores so the `bool` distinguishes "never indexed" from a zero time.
- [x] 1.2 Persist the last-index timestamp in the file store's metadata and add a test proving it survives a process restart and reads without contacting any downstream server.

## 2. Indexer records successful runs

- [x] 2.1 Record the run timestamp via the new store setter when an index pass completes successfully (including a reachable run that discovers zero tools).
- [x] 2.2 Add an `index` test asserting that after `Indexer.Run` the catalog's `LastIndexedAt` returns the run time, and that a fully-failed run does not advance it.

## 3. Search engine — lexical baseline

- [x] 3.1 Create `internal/search` with `Engine.Find(ctx, query) -> Ranking` reading `catalog.Store`; implement a tokenizer and a field-weighted BM25-style lexical scorer over the `SPEC.md` §10.2 indexed fields (boost `toolRef`/name/title and capability aliases above description and schema-field text).
- [x] 3.2 Retain matched terms and top contributing fields on each ranked entry for the `reason`; unit-test ranking order and that the explanation names the matched basis rather than echoing the query.

## 4. Hybrid fusion and decision model

- [x] 4.1 Add a `search.Semantic` seam with an "unavailable" default and map each signal to an absolute `[0,1]` scale (lexical via `s/(s+k)` saturation; semantic via `(cos+1)/2`).
- [x] 4.2 Implement weighted-sum fusion with weights renormalized when semantic is absent (lexical → 1), exposing the relevance floor `F`, separation margin `M`, and `w_lex`/`w_sem` as named tunable constants.
- [x] 4.3 Map fused scores to a decision — `use` (top ≥ F and top−second ≥ M; `high`/`medium` confidence), `ambiguous` (top ≥ F, gap < M), `no_good_match` (top < F), `catalog_empty` (no indexed tools) — always exposing rank 1 as the best and rank 2 as the single runner-up; unit-test each band with a fake semantic scorer.

## 5. Broker findTool over the catalog

- [x] 5.1 Replace `live.FindTool`'s live discovery with a call to `search.Engine`; translate the `Ranking` into `contract.FindResult` — `selected` (with a bounded `schemaPreview` and live/freshness status), `confidence`, `reason`, exactly one `alternatives` entry (runner-up), and `nextAction` → `describeTool(selectedToolRef)`.
- [x] 5.2 Surface the semantic-degraded mode instructionally without failing, honor `budgets.findTool` (preview not full schema), and leave `describeTool` (exact catalog lookup) and `callTool` (live) delegation unchanged.
- [x] 5.3 Add broker tests for `use` + runner-up, `ambiguous`, `no_good_match`, `catalog_empty`, semantic-requested-but-unavailable (degraded), and a cataloged tool still discoverable while its downstream server is offline.

## 6. Daemon startup indexing

- [x] 6.1 Construct the `search.Engine` in the daemon and wire it into the broker so `findTool` ranks the catalog.
- [x] 6.2 In `daemon.Run`, compute staleness from `os.Stat(cfg.Path).ModTime()` vs. `store.LastIndexedAt`; when stale (or never indexed), run `index.Indexer.Run` synchronously before reporting ready, with a stat-error fallback of "index only if never indexed."
- [x] 6.3 Ensure startup indexing is non-blocking for readiness: on total failure or no reachable server the daemon still reports ready, serves the existing catalog, and surfaces the index summary.
- [x] 6.4 Add daemon tests: stale catalog triggers reindex on startup, fresh catalog skips it, and an indexing failure still yields a ready daemon serving the prior catalog.

## 7. Adapter and CLI parity

- [x] 7.1 Confirm `internal/mcp` and `ozy search` need no surface change and that the advertised MCP tool list is still exactly `findTool`, `describeTool`, `callTool`.
- [x] 7.2 Add a parity test proving `ozy search "<query>"` and the MCP `findTool` produce the semantically equivalent ranked decision through the shared broker.

## 8. Acceptance, eval, and documentation

- [x] 8.1 Add an integration test with a fixture downstream MCP server proving the end-to-end loop: daemon startup indexes the catalog, `findTool` returns `use` with a runner-up, `describeTool` returns the exact schema, and `callTool` succeeds — without a manual `ozy index`.
- [x] 8.2 Add a discovery eval scenario (gold intent → expected `toolRef`) asserting top-1 selection and the two-best response shape (`SPEC.md` §14.1).
- [x] 8.3 Update README/docs to describe catalog-backed `findTool`, conditional startup indexing, and the lexical-only default with graceful semantic degradation.
- [x] 8.4 Run `go test ./...`, `gofmt`, `golangci-lint run`, `openspec validate tool-index-hybrid-search`, and `graphify update .`.
