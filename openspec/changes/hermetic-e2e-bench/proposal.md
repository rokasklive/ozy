## Why

The scenario bench collects everything and reports nothing: 20 recorded runs produced transcripts, ContextSpy breakdowns, and grading files, but zero of the promised decision-grade outputs (`surface.json`, `aggregate.json`, `comparison.md`) — the reporters (`MeasureSurface`, `WriteComparison`, `LedgerWriter`) are dead code, the model-free static tier is unreachable, and each run re-fetches npm packages into a fresh HOME, breaking hermeticity and depositing 2.1 GB of debris. Ozy's central product claim (smaller agent surface, preserved task success) remains unproven by the very harness built to prove it.

## What Changes

- **Wire the static surface tier** as a model-free `ozy-bench surface` path that always runs (CI-gateable): startup tool counts, schema bytes, estimated tokens per mode.
- **Complete the reporting pipeline**: per-run `metrics.json`, per-mode `aggregate.json` (success k/N, mean/min/max/stdev over duration and tokens), and a cross-mode `comparison.{json,md}` with a machine-readable verdict — built from grading, agent-reported usage events, and transcripts.
- **Switch the live tier to OpenCode's built-in free models** (e.g. DeepSeek V4 Flash Preview, Nemotron 3; any OpenCode-resolvable model via `BENCH_MODEL`) — **BREAKING for the harness config**: the BYO OpenAI-compatible endpoint (`MODEL_BASE_URL`/`MODEL_API_KEY`), the custom provider block, the per-run `auth.json`, and the ContextSpy proxy/capture stack are all removed. Token metrics come from OpenCode's own usage events (`measured`) with transcript estimation as fallback (`estimated`).
- **Make runs hermetic**: with no provider package or auth file left to fetch or write, runtime network access is the model gateway only — enforced by a build-time smoke test with the package registry unroutable; provenance records the model ID as the one deliberately unpinned variable.
- **One-command entrypoint**: `make bench` (wrapping compose) runs fixture generation → both modes → reporting → exit code, zero-config on a clean checkout (free model default).
- **Grading completeness**: feed the fixture's recorded culprit commit hash into grading (currently hardcoded `""`), per the already-specced fixture requirement.
- **Fix drift and warts**: README/compose default mismatches (`MODE`, `BENCH_RUNS`), the doubled `ozy/run-1/ozy-run-1` run path, and printed report paths that nothing writes.
- **GitHub Actions operation**: the surface tier becomes a PR-gating CI job (native, no Docker/model); the live tier becomes a `workflow_dispatch` job that runs the compose stack against the default free model — no model secrets — publishes `comparison.md` to the job summary, and uploads the run directory as an artifact — informational, never a PR gate.
- **Retire the context ledger**: delete `internal/bench/ledger.go` and drop the itemized-ledger requirement — agent-reported usage (measured) plus transcript estimation (fallback) supersedes it at a fraction of the complexity. **BREAKING** only for the unmerged `context-ledger` spec delta; no shipped behavior changes.
- **Supersede the `scenario-bench-harness` change**: its fixture and MCP-fixture specs carry forward verbatim; its execution spec is extended (one-command, hermeticity, defaults); its context-ledger spec is dropped. The old change is archived as superseded once this lands.

## Capabilities

### New Capabilities

- `scenario-bench`: two-mode identical-environment execution contract — mode wiring (direct vs ozy), one-command invocation, hermeticity guarantees, model endpoint configuration, run directory layout, and extension points (new scenarios, new modes as named config templates). Carries forward and extends the unmerged delta from `scenario-bench-harness`.
- `bench-reporting`: decision-grade outputs — always-on static surface measurement, per-run metrics, per-mode aggregates, cross-mode comparison with verdict, provenance, and exit-code semantics. Supersedes the unmerged `context-ledger` capability.
- `bench-fixtures`: deterministic acme-billing fixture with real git history and recorded culprit hash. Carried verbatim from `scenario-bench-harness` (requirements unchanged; the culprit-hash recording finally gets implemented).
- `bench-mcp-fixtures`: parameterized fixture MCP server (7 toolsets over one binary). Carried verbatim from `scenario-bench-harness` (requirements unchanged).

### Modified Capabilities

<!-- none — no bench capability exists in openspec/specs/ yet; eval-benchmarks (the deterministic suite) is untouched -->

## Impact

- **Code**: `internal/bench/` (wire `surface.go`, new `report.go`, delete `ledger.go` and `contextspy.go`, simplify `runner.go` config writing — no provider block, no `auth.json` — fix the path bug and `culpritHash`), `cmd/ozy-bench` (surface-only mode), `bench/Dockerfile` (build-time offline smoke test), `bench/docker-compose.yml` (default alignment, drop `CONTEXTSPY_API`), `bench/contextspy-config.toml` (deleted), `bench/README.md`, root README (ContextSpy attribution updated to past tense), `Makefile` (`bench` targets).
- **No product code changes**: `internal/broker`, `internal/mcp`, the CLI, and the deterministic eval suite (`internal/eval`, `evals/`) are untouched.
- **Dependencies**: ContextSpy and the `@ai-sdk/openai-compatible` provider package leave the harness entirely; no new Go dependencies.
- **CI**: the static surface tier becomes a gateable job (no model, no Docker-in-Docker needed — plain `go test`/CLI run); the live tier ships as a `workflow_dispatch` workflow needing no model secrets for the default free model (one optional secret if a chosen model requires an OpenCode credential), bounded by job timeout and concurrency group, and stays informational (LLM nondeterminism and free-model rate limits must not gate CI).
- **Process**: `openspec/changes/scenario-bench-harness` is archived as superseded when this change completes.
