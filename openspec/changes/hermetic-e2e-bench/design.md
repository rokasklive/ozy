## Context

The bench today is split between what works and what was never wired. Working: deterministic fixture generation (`internal/bench/fixture.go`), seven fixture MCP toolsets served by one binary (`internal/bench/mcp.go`), a Docker image with OpenCode pinned at 1.17.7, per-run agent isolation (fresh HOME, project-scoped opencode.json), ContextSpy session control + per-run `context-breakdown.json` (`internal/bench/contextspy.go`), transcript capture, and grading (`internal/bench/grader.go`). Never wired: `MeasureSurface`/`WriteComparison` (`surface.go`), `LedgerWriter` (`ledger.go`), culprit-hash plumbing into grading (`runner.go` hardcodes `""`), and any aggregation. Non-hermetic: each run re-fetches `@ai-sdk/openai-compatible` from npm into a fresh per-run HOME; ContextSpy runs as an undocumented host-side prerequisite (proxy :8889 + capture API :5173). Evidence: 20 run directories, 19 incomplete, no `comparison.md` ever produced, 2.1 GB of local debris.

Constraints: the deterministic eval suite (`internal/eval`, `evals/`) is the gating system and stays untouched; the product claim this harness must prove is "smaller agent-facing surface, preserved task success, measured token economy"; the maintainer's hard rule is that live runs happen in Docker, never on the host. (The earlier per-run ContextSpy rule is revised by this change: dropping the BYO endpoint removes the interceptable seam ContextSpy needed, and token accounting moves to OpenCode's own usage events.)

## Goals / Non-Goals

**Goals:**

- One command (`make bench`) runs: fixture → static surface tier → live tier (both modes) → aggregates → comparison → exit code — zero config on a clean checkout.
- Hermetic except the model gateway: after `docker compose build`, a run performs no network access other than OpenCode's gateway for the selected model, which provenance records as the one unpinned variable.
- Real models drive a real agent (OpenCode) — the live tier is never simulated. Defaults are OpenCode's built-in free models (e.g. DeepSeek V4 Flash Preview, Nemotron 3); any model ID the pinned OpenCode resolves is selectable via `BENCH_MODEL`.
- Decision-grade output: `surface.json` (always), per-run `metrics.json`, per-mode `aggregate.json`, cross-mode `comparison.{json,md}` with measured-vs-estimated token labeling and provenance.
- Extension = data, not code: new scenario = new directory; new mode = new opencode config template; new model = env change.

**Non-Goals:**

- No BYO OpenAI-compatible endpoint and no local model serving — deliberately dropped: the custom-endpoint path is what dragged in the provider npm package, the per-run auth file, and the ContextSpy proxy indirection. OpenCode's built-in models replace all of it.
- No multi-scenario matrix runner, no results database, no dashboard — one scenario per invocation.
- No LLM-as-judge grading (ground-truth string/hash checks only, as today).
- No CI gating of the live tier (LLM nondeterminism); only the static surface tier is gateable.
- No threshold/regression engine for live metrics — the comparison reports deltas and success rates; humans decide.
- No changes to ozy product code, the deterministic eval suite, or `evals/` outputs.

## Decisions

### D1 — Reuse the existing harness; complete it rather than rewrite

~80% of the needed machinery exists and passes its tests (fixture, MCP toolsets, runner, ContextSpy client, grader, Docker image). The gap is ~300 lines of reporting plus packaging. *Alternative considered:* adopting a general eval framework (promptfoo, Inspect) — rejected: the differentiating machinery is the two-mode MCP wiring and the frozen fixture, which no framework provides; we'd keep all our code and add a dependency. *Alternative:* greenfield rewrite — rejected: the broken parts are the unbuilt parts, not the built ones.

### D2 — Hermeticity = "pinned everything except OpenCode's model gateway", enforced at build time

- The image pins OpenCode (exact version) and builds `ozy`/`ozy-bench` from the working tree. The live tier uses OpenCode's built-in models, so there is no provider package to install and no auth file to write.
- Runtime performs no package fetches. Enforcement: a build-time smoke test starts a stub OpenCode invocation with `npm_config_registry` pointed at an unroutable address — if the agent cannot reach its ready-to-run state from build-time layers, the *build* fails, not the run. (If a pinned OpenCode version turns out to fetch anything at first run, that fetch is performed at build time into the image; the smoke test decides.)
- `environment.json` provenance records: image content (ozy git describe), OpenCode version, model ID, token estimator name, and whether usage was agent-reported or estimated. No credentials, ever.
- *Alternative considered:* baking a small GGUF model into the image for total hermeticity — rejected: contradicts the "real models" requirement and bloats the image. The model gateway is the accepted, recorded egress.

### D3 — Model selection via OpenCode built-ins; agent config shrinks to almost nothing

The per-run `opencode.json` drops the entire custom `provider` block (and with it the `@ai-sdk/openai-compatible` npm dependency, the `MODEL_BASE_URL`/`MODEL_API_KEY`/context-limit plumbing, and the `auth.json` writer) and keeps only: the model ID, the mode's MCP servers, and the `AGENTS.md` instruction. The model is `BENCH_MODEL` (default: one of OpenCode's free models — DeepSeek V4 Flash Preview or Nemotron 3, fixed during implementation and recorded in provenance); any model ID the pinned OpenCode resolves works unchanged. An unresolvable model fails fast before runs start. *Alternative considered:* keeping the BYO OpenAI-compatible endpoint alongside built-ins — rejected: it is the single largest source of harness complexity (provider install, auth scaffolding, capture proxy) and the free built-ins are sufficient for the task.

### D4 — Token accounting from OpenCode's own usage events; ContextSpy exits the harness

Dropping the BYO endpoint removes the interceptable HTTP seam ContextSpy proxied, so ContextSpy (client code, config, compose wiring, host-side services) is deleted from the harness. Token metrics come from the usage events the pinned OpenCode emits in its `run --format json` output (`token_source: "measured"`); when a transcript carries no usable usage events, metrics are computed from transcript bytes plus enumerated startup schemas (`token_source: "estimated"` — the same fallback the retired ledger spec required). Missing usage never fails a run. Tool-definition token attribution does not depend on capture at all — the static surface tier measures it deterministically. *Alternative considered:* HTTPS-MITM proxying of the gateway traffic to keep ContextSpy — rejected: cert trust inside the agent image is exactly the kind of complexity this change exists to remove.

### D5 — Reporting lives in `ozy-bench` (Go), computed at run end from already-captured artifacts

- `surface.json` — computed **always**, before the live tier, with no model: direct-mode tools enumerated in-process from the fixture toolsets (ozy-bench *is* the fixture server binary — no subprocesses needed), ozy-mode tools from the three broker tool definitions already mirrored in `internal/eval/economy.go`. An explicit surface-only mode (`make bench-surface` / `--surface-only`) produces this tier alone, exit 0.
- Per run: `metrics.json` (success, timed_out, duration, tool-call count, token totals + breakdown source).
- Per mode: `aggregate.json` — success k/N, mean/min/max/stdev for duration and token totals.
- Cross-mode: `comparison.json` + human `comparison.md` — startup tool-def tokens, total input/output, tool calls, success rate, duration, per-metric deltas, and a descriptive verdict block. No thresholds engine (non-goal); the verdict states facts ("ozy: 5/5 success, 62% fewer tool-def tokens, +1.8 calls/run").
- `LedgerWriter` and the itemized-ledger concept are deleted — agent-reported usage (D4) is the measured source; the ledger was a second, unbuilt implementation of the same idea.

### D6 — One command, one env file, aligned defaults

`make bench` wraps `docker compose -f bench/docker-compose.yml up --build --exit-code-from bench-runner`. Defaults work with zero config on a clean checkout: `MODE=both`, `BENCH_RUNS=5`, `SCENARIO=suspended-account-invoice-regression`, `BENCH_MODEL=<the chosen free default>` — defined once in compose and documented identically in README (today README says `both`/5 while compose ships `direct`/1). Precedence: flag > env > scenario config > built-in default. `make bench-surface` runs the static tier natively (no Docker) for CI. `make bench-clean` prunes `bench/runs/`.

### D7 — Extension points are files, not code

- **Scenario** = `bench/scenarios/<name>/` (`scenario.jsonc`, `task.md`, `expected/ground_truth.json`). The fixture generator stays scenario-keyed.
- **Mode** = `bench/configs/opencode.<mode>.jsonc`; the orchestrator resolves mode names to config templates, with `ozy` remaining the only mode with a setup hook (config + index). A future `ozy-nocache` mode is a new jsonc file, zero Go.
- **Model** = one variable (`BENCH_MODEL`, any OpenCode-resolvable model ID; free default).

### D8 — Fix the run-directory contract

The runner receives its final absolute run directory and writes into it directly (killing the `ozy/run-1/ozy-run-1` double-nesting from re-joining `mode-runID`). Layout matches the README exactly; grading receives the culprit hash from the fixture's recorded metadata (`fixture-meta.json`, written by `ozy-bench fixture` per the existing bench-fixtures requirement).

### D9 — GitHub Actions: gate the surface tier, dispatch the live tier

Two jobs with different rules:

- **Surface job (PR gate)**: runs `make bench-surface` natively on `ubuntu-latest` in the existing CI workflow — no Docker, no model, no secrets, deterministic, so it can gate. It fails only on harness failure (the comparison cannot be produced); quality thresholds on the numbers can be added later once a few runs establish what "regression" means.
- **Live job (`workflow_dispatch`, informational)**: a separate `bench.yml` workflow with inputs (`mode`, `runs` — default 3 in CI to bound load, `scenario`, `model`) that runs `make bench` (compose works out of the box on `ubuntu-latest`), appends `comparison.md` to `$GITHUB_STEP_SUMMARY`, and uploads the run directory as a build artifact with bounded retention. The default free model needs **no secrets**; if a non-default model requires an OpenCode credential, it arrives as one optional secret. Guardrails: a `concurrency` group so runs never overlap, a hard `timeout-minutes`, and no `pull_request` trigger — rate limits on free models and LLM nondeterminism disqualify it as a gate.

*Why this works on hosted runners:* the model is served by OpenCode's gateway, so the CPU-only runner just drives the agent loop — no GPU, no endpoint infrastructure, and with the free default, no secrets. The hermeticity story holds — the image build pins everything, the run's only egress is the gateway, and provenance records the model ID. *Alternative considered:* scheduled (cron) live runs — deferred; trivially added to the same workflow once dispatch runs prove stable and free-model rate limits are understood.

## Risks / Trade-offs

- [OpenCode transcript format drift breaks `parseTranscript`] → version stays pinned; parser gains a loud self-check (zero parsed events from a non-empty transcript fails the run with a named error instead of silently grading an empty answer).
- [Pinned OpenCode may not emit usage events in `run --format json`] → verified early during implementation; the estimated fallback (D4) keeps every artifact flowing either way, and `comparison.md` labels the source — the harness is never blocked on token accounting.
- [Free preview models get renamed, retired, or requeued behind auth] → the default is one pinned ID recorded in provenance; `BENCH_MODEL` swaps it in one variable; an unresolvable model fails fast with a named error. Comparisons are only meaningful within one model ID — the reports state the model for exactly this reason.
- [Free-model rate limits / queueing skew latency numbers] → duration is reported per run with min/max/stdev; token counts and success are unaffected; CI default of 3 runs bounds pressure.
- [Live-tier numbers are noisy at N=5] → aggregates carry min/max/stdev and success k/N; the comparison is informational by design and never gates CI (non-goal).
- [The model gateway remains a network dependency] → deliberate: "real models" requires it; provenance records it as the one unpinned variable.
- [2.1 GB of legacy run debris] → `make bench-clean`; new runs are lean (no per-run npm cache, no provider install).

## Migration Plan

Additive and self-contained under `bench/` + `internal/bench/` + `cmd/ozy-bench`; no product code or eval-suite changes, so no rollback complexity — reverting the change restores the current harness. Old run directories stay readable (new reports live alongside old artifacts, not instead of them). When the change completes: archive `openspec/changes/scenario-bench-harness` as superseded, and update `bench/README.md` so every promised artifact is one the harness actually writes.

## Open Questions

- Which free model becomes the pinned default (DeepSeek V4 Flash Preview vs Nemotron 3) — decide during implementation by running both once and picking the one with steadier latency/rate limits; the other stays one `BENCH_MODEL` away.
- Whether the chosen free models work fully anonymously or need a baked OpenCode credential in CI — verified during implementation; if needed it is one optional repo secret, never in provenance or the image.
- Whether the pinned OpenCode emits per-message usage events in JSON output (D4) — verified during implementation; both outcomes have a designed path (measured vs estimated).
