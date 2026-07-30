## 1. Harness foundation fixes

- [x] 1.1 Fix run-directory double nesting: `Runner.Run` writes into the run dir it is given (`ozy/run-1/transcript.jsonl`, no `ozy-run-1/` re-join) — `internal/bench/runner.go`
- [x] 1.2 Write `fixture-meta.json` (culprit hash + generator version) from `ozy-bench fixture`; the generator already resolves the hash — `internal/bench/fixture.go`, `fixture_cmd.go`
- [x] 1.3 Plumb the culprit hash from `fixture-meta.json` into grading (replace the hardcoded `culpritHash := ""`) — `internal/bench/runner.go`
- [x] 1.4 Transcript parse self-check: non-empty transcript with zero parsed events marks the run `parse_failed` with a named error, distinct from task failure — `internal/bench/runner.go`
- [x] 1.5 Align defaults to `MODE=both`, `BENCH_RUNS=5` with precedence flag > env > scenario > default, single source of truth; update compose, `run.go`, README table — `internal/bench/run.go`, `bench/docker-compose.yml`, `bench/README.md`

## 2. Reporting pipeline

- [x] 2.1 Wire the static surface tier: enumerate direct-mode tools in-process from the fixture toolsets and ozy-mode tools from the adapter's three definitions; write `surface.json` at the top of every invocation (also when `MODEL_BASE_URL` is unset) — `internal/bench/surface.go`, `run.go`
- [x] 2.2 Per-run `metrics.json`: success/timed_out/parse_failed, duration, tool-call count, token totals with `token_source` — `measured` parsed from OpenCode usage events in the JSON transcript (verify the pinned version emits them), `estimated` otherwise — new `internal/bench/report.go`
- [x] 2.3 Estimated-tokens fallback: derive token totals from transcript bytes + enumerated startup schemas using the eval token estimator when no usage events exist — `internal/bench/report.go` (reuse `internal/eval/token.go`)
- [x] 2.4 Per-mode `aggregate.json`: success k/N, timed-out/parse-failed counts, mean/min/max/stdev over duration and token totals; measured and estimated runs reported separately or labeled mixed — `internal/bench/report.go`
- [x] 2.5 Cross-mode `comparison.json` + `comparison.md`: surface delta, success rates, duration and token deltas (ozy vs direct), descriptive verdict block, token-source labels, provenance reference; surface-only variant when the live tier was skipped — extend `internal/bench/surface.go` `WriteComparison` or replace in `report.go`
- [x] 2.6 Exit-code semantics: 0 for completed invocations (including surface-only), non-zero only for harness errors; live-tier task failures never affect the exit code — `internal/bench/run.go`, `cli.go`
- [x] 2.7 Delete `internal/bench/ledger.go` and its test; remove ledger references from docs
- [x] 2.8 Unit tests: aggregate math (k/N, stdev), comparison rendering (both + surface-only), estimated-fallback labeling, parse self-check — `internal/bench/report_test.go`

## 3. Free built-in models and hermeticity

- [x] 3.1 Switch the live tier to OpenCode built-in models: drop the custom `provider` block, the `@ai-sdk/openai-compatible` dependency, the model-limit plumbing, and the `auth.json` writer; `opencode.json` = model ID (`BENCH_MODEL`, pinned free default) + mode MCP servers + instructions; unresolvable model fails fast before runs — `internal/bench/runner.go`
- [x] 3.2 Pick and pin the default free model — pinned `opencode/deepseek-v4-flash-free` (the real slug; `opencode models` in the pinned image lists it alongside `nemotron-3-ultra-free` + 4 others). Verified in Docker: resolves, drives both modes to task success, and **works fully anonymously — no credential/secret needed** (confirmed by a live run with no `OPENCODE_API_KEY`). Steadiness: 3/4 live runs passed cleanly, one direct run hit the free-tier latency ceiling (recorded as `timed_out`). Nemotron is documented as the one-variable alternative; a formal head-to-head wasn't run (DeepSeek proved steady enough) — `internal/bench/runner.go`, `bench/README.md`, `bench/.env.example`
- [x] 3.3 Delete the ContextSpy integration: `internal/bench/contextspy.go` + test, `Spy` wiring in `runner.go`, `bench/contextspy-config.toml`, `CONTEXTSPY_API` in compose; update the root README attribution to past tense
- [x] 3.4 Build-time offline smoke test: image build fails if the agent cannot reach ready-to-run state with the npm registry unroutable; if the pinned OpenCode fetches anything at first run, perform that fetch at build time — `bench/Dockerfile`
- [x] 3.5 Extend `environment.json` provenance: OpenCode version, model ID, ozy git describe, token estimator name, usage source (agent-reported vs estimated); assert no credential fields — `internal/bench/provenance.go`

## 4. One command and docs

- [x] 4.1 `make bench` (compose up --build with exit-code propagation), `make bench-surface` (native, no Docker/model), `make bench-clean` (prune `bench/runs/`) — `Makefile`
- [x] 4.2 Rewrite `bench/README.md` so every promised artifact is one the harness writes: quick start = `make bench`, artifact layout matching the spec, defaults table matching compose, hermeticity statement (pinned everything, model endpoint is the recorded variable)
- [x] 4.3 Refresh `bench/.env.example` down to the surviving vars (`BENCH_MODEL`, `BENCH_RUNS`, `MODE`, `SCENARIO`, `BENCH_TIMEOUT`) with comments; zero-config default documented
- [x] 4.4 Add the surface tier to CI as a gateable PR job (`make bench-surface`, native, no Docker/model/secrets) — `.github/workflows/ci.yml`
- [x] 4.5 Add the live bench as a `workflow_dispatch` workflow: inputs (`mode` default `both`, `runs` default 3, `scenario`, `model`), no model secrets for the free default (one optional secret if 3.2 finds a credential is needed), `comparison.md` appended to `$GITHUB_STEP_SUMMARY`, run directory uploaded as an artifact with bounded retention, `concurrency` group + `timeout-minutes`, no `pull_request` trigger — new `.github/workflows/bench.yml`

## 5. End-to-end verification

- [x] 5.1 Surface-only e2e: `make bench-surface` (and `--surface-only` in-container) exits 0 and writes `surface.json` + surface-only `comparison.{json,md}` — verified natively: direct=19 tools/1204 tok, ozy=3 tools/509 tok, exit 0, all artifacts written
- [x] 5.2 Live smoke: `make bench` (`MODE=both`, `BENCH_RUNS=2`, DeepSeek free) in Docker — verified: `surface.json`/`environment.json`/`comparison.{json,md}`, per-mode `aggregate.json`, and per-run `metrics.json`/`grading.json`/`transcript.jsonl`/`tool-calls.jsonl`/`final-answer.md`/`task.md` all present; grading credits the real culprit (`f4d5adf`); all runs `token_source: measured`; **0 `auth.json`, 0 `.npm/_cacache`, 0 `node_modules`** under any run dir
- [x] 5.3 Offline-registry check: the harness now pins `npm_config_registry` unroutable for the agent process (`internal/bench/runner.go`), so every live run above ran with npm unreachable and completed normally — proving the model gateway is the only runtime egress (no `.npm/_cacache` produced). Directly diagnosed and fixed a real runtime-npm leak the first live smoke exposed.
- [x] 5.4 Verify run independence — verified in Docker: `setupOzy` moved inside the run loop so each run re-indexes its own catalog (`ozy/run-1/ozy-catalog.json` + `ozy/run-2/ozy-catalog.json`, `indexed 19 tools` logged per run); each run gets a fresh `agent-home` (HOME/session/db); no catalog, memory, or artifacts carry over — `internal/bench/runner.go`
- [ ] 5.5 Dispatch `bench.yml` once with defaults (`runs=1`, free model, no secrets) and verify the job summary shows `comparison.md`, the artifact downloads, and the job respects timeout/concurrency settings — **BLOCKED: requires the change merged to GitHub + a manual workflow dispatch**

## 6. Process cleanup

- [ ] 6.1 Archive `openspec/changes/scenario-bench-harness` as superseded by this change (its fixture/mcp-fixture specs carried forward verbatim; context-ledger retired) — **DEFERRED to merge time: the proposal archives it "once this lands"; doing it before this change is validated/merged risks spec conflicts**
- [x] 6.2 Purge legacy `bench/runs/` debris locally (`make bench-clean`) and note the layout change in the README — 2.1 GB pruned, `.gitkeep` preserved; README documents the new layout
