## Why

`SPEC.md` §4.8 ("evaluation before confidence") and §14 prescribe a scenario-based
eval framework as a first-class gate, but it does not exist yet: evals today are a
handful of ad-hoc Go unit tests with tool fixtures hardcoded inline
(`TestDiscoveryEval_*` in `internal/cli/cli_test.go`), the semantic leg is proven
only with a fake provider, `ozy eval run` is a `NOT_IMPLEMENTED` stub, and there
is no committed test corpus, no real-embedding-model measurement, and no
public-facing benchmark anyone can read or track over time. Without this we cannot
honestly claim the token-economy savings §13 demands, calibrate the RRF `k` and
relevance floors §10.3 says are "calibrated by evals," or detect regressions in
discovery, invocation, repair, or agent ergonomics as the project moves. This
change builds the definitive suite that every future change measures against.

## What Changes

- Add a **gold-set / scenario corpus** as committed data (not inline fixtures):
  a synthetic-but-realistic downstream MCP catalog (servers + tools with real
  names, descriptions, and input schemas) and labeled scenario sets covering
  discovery, invocation, repair, offline/degraded, and ergonomics.
- Add a **Go eval harness** (`internal/eval`) that loads the corpus, drives the
  real broker / search / CLI / MCP seams, and computes the §14.2 metrics
  **deterministically** (discovery top-1/top-3/MRR/wrong-server/no-match;
  invocation valid-arg/first-call/repair/schema-error; token economy).
- Cover the four eval areas the suite must own:
  - **Agent ergonomics** for the CLI and MCP agent-facing interfaces — structural,
    automatable checks against the §4.5 instructional-quality criteria and §9
    contract shapes (every result is a decision with a grounded, conditional,
    actionable `agentInstruction` / `nextAction`; responses stay within §13
    budgets; **CLI ↔ MCP parity**), plus a rubric scaffold for judgment-heavy
    metrics graded by human/review-board per §14.
  - **Lexical search** accuracy (BM25 gold-set scoring) **and performance**
    (latency/throughput benchmarks).
  - **Semantic search** accuracy and performance, run against the **real
    FastEmbed model** (`BAAI/bge-small-en-v1.5`) through the actual sidecar — not
    the fake provider — so the numbers are real, with hybrid/RRF fusion measured
    against the lexical baseline.
  - **End-to-end smoke tests** simulating happy paths (index → find → describe →
    call) and unhappy paths (server offline, schema drift, bad arguments, empty
    catalog, semantic degraded), asserting structured repair behavior.
- Emit results in two registers: a **machine-readable JSON snapshot per run**
  (for tracking deltas over time and CI gating) and an **easy-to-consume public
  benchmark scoreboard** (`evals/BENCHMARKS.md`), with the "math spam" (metric
  formulas, floor/`k` calibration, confidence intervals, judge rubrics) quarantined
  in `evals/METHODOLOGY.md` so the public surface stays readable.
- **Wire `ozy eval`** so `ozy eval run <scenario>` and `ozy eval report` execute
  the harness and emit `--format json` results, replacing the `NOT_IMPLEMENTED`
  stub and honoring §14.3 / §15.
- Use the **real embedding model as the source of truth** for committed semantic
  benchmarks, gated behind an opt-in (mirroring the sidecar's `@pytest.mark.slow`)
  so the default `go test ./...` stays fast on lexical + structural evals.

## Capabilities

### New Capabilities
- `eval-harness`: The eval engine — the scenario/gold-set schema and loader, the
  runners that drive the broker, search engine, CLI, MCP adapter, and real
  sidecar, deterministic metric computation for the §14.2 families, run-result
  capture, and the real-model gating for semantic evals.
- `eval-datasets`: The committed test corpus — the synthetic downstream MCP
  catalog ("the world") and the labeled gold/scenario sets (lexical, semantic
  paraphrase, no-match, ambiguous, wrong-server, invocation valid/invalid,
  schema-drift, offline, ergonomics), plus the data schema and provenance /
  contribution rules that keep the gold sets honest.
- `eval-benchmarks`: The reporting and tracking surface — the per-run
  machine-readable JSON snapshot, the human-readable public benchmark scoreboard,
  the separation of public benchmarks from methodology/math, and the threshold
  gates that turn an eval run into a pass/fail signal.

### Modified Capabilities
- `cli-interface`: `ozy eval` becomes a real broker-adjacent command
  (`eval run <scenario>`, `eval report`) routed through the harness with
  `--format` support; the "structured handling of unimplemented operations"
  requirement stops citing `ozy eval run` as its example since it is now wired.

## Impact

- Affected code:
  - `evals/` (**new**): committed corpus data, gold/scenario sets, generated
    `BENCHMARKS.md` scoreboard, `METHODOLOGY.md`, and per-run JSON snapshots.
  - `internal/eval` (**new**, Go): scenario loader, metric computation, the
    broker/search/CLI/MCP/sidecar runners, token measurement, and the JSON +
    Markdown reporters.
  - `internal/cli` (**modified**): replace the `eval run` `NOT_IMPLEMENTED` stub
    with `eval run <scenario>` / `eval report` wired to the harness.
  - `internal/search`, `internal/broker`, `internal/sidecar` (**consumed**, not
    changed): the harness drives existing seams; the eval suite produces the
    calibration evidence for the §10.3 constants rather than changing them here.
- Affected behavior: contributors and CI gain `ozy eval run`/`ozy eval report`
  and a tracked benchmark scoreboard; the semantic accuracy numbers now come from
  the real embedding model instead of a stub.
- Dependencies: a deterministic token estimator for token-economy metrics
  (documented approximate tokenizer, swappable); the real FastEmbed model +
  provisioned sidecar for the opt-in semantic benchmark leg (already a project
  dependency). No new mandatory runtime dependency for the Go binary.
- Non-goals: changing the `findTool`/`describeTool`/`callTool` contracts or the
  MCP surface; shipping an automated LLM-as-judge as a gate (rubric scaffolding
  only, human/review-board grading per §14 until a calibrated judge exists);
  re-tuning the search constants (the suite measures; a follow-up change may
  recalibrate); ContextSpy integration beyond a clearly-marked optional hook.
