## Context

`SPEC.md` makes evaluation a first-class gate (§4.8, §14, §19.5) and even names the
interface (`ozy eval run <scenario>`, §14.3/§15), but the suite does not exist:

- The only evals are inline Go unit tests with hardcoded tool fixtures —
  `TestDiscoveryEval_GoldIntentMatchesExpectedToolRef` and
  `TestDiscoveryEval_SemanticIntentChangesWinner` in
  `internal/cli/cli_test.go`. The catalog, the intents, and the expected answers
  all live in Go source, so the "gold set" cannot grow without code edits.
- The semantic leg is proven only with a `semanticFakeProvider` returning
  hand-written scores; the **real** FastEmbed model (`BAAI/bge-small-en-v1.5`,
  384-dim) has never been exercised by an eval.
- `ozy eval run` is a `NOT_IMPLEMENTED` stub (`internal/cli/commands.go`), and the
  `cli-interface` spec literally cites it as the example of a deferred command.
- The search constants `internal/search/fusion.go` calls "calibrated by evals"
  (`RRFK`, `LexicalRelevanceFloor`, `SemanticRelevanceFloor`, `SeparationMargin`,
  `HighConfidenceThreshold`) have no eval to calibrate against.
- There is no committed corpus, no token-economy measurement, and nothing a reader
  can consult to see how the system performs or whether a change improved it.

The seams the harness needs already exist and are clean: `broker.Broker`
(`FindTool`/`DescribeTool`/`CallTool`), `search.Engine` + `search.Decide`, the
`search.Semantic` query seam satisfied by `internal/sidecar`, the `contract.*`
result types, the `mcp.Adapter` (in-process), and `catalog.NewMemory()` for
loading a corpus without a live downstream. This change builds the suite on top of
those seams rather than touching them.

This design is also grounded in the project's house methodology, available as
skills in this repo: the **agent-ergonomics** taxonomy (5 domains / 5 laws) for
the ergonomics family, and **evaluation-harness-designer** discipline (gold-set
hygiene, metric validity over convenient proxies, deterministic-vs-sampled
metrics, cost-per-task) for the harness itself.

## Goals / Non-Goals

**Goals:**

- A definitive, repeatable eval suite covering the four areas the request names:
  agent ergonomics (CLI + MCP), lexical accuracy + performance, semantic accuracy
  + performance (real model), and end-to-end happy/unhappy smoke tests.
- Test data as committed **data**, not inline fixtures — a synthetic-but-realistic
  MCP catalog plus labeled gold/scenario sets that grow without code edits.
- Deterministic computation of the `SPEC.md` §14.2 metrics, plus latency/throughput
  benchmarks.
- Two output registers: a machine-readable per-run JSON snapshot for tracking and
  gating, and an easy-to-consume public `BENCHMARKS.md` scoreboard; the math lives
  apart in `METHODOLOGY.md`.
- Real FastEmbed model as the source of truth for committed semantic numbers,
  gated so default `go test ./...` stays fast.
- `ozy eval run` / `ozy eval report` wired to the harness.

**Non-Goals:**

- Changing the `findTool`/`describeTool`/`callTool` contracts or the MCP surface —
  the harness observes them; it does not reshape them.
- Recalibrating the search constants. This suite produces the **evidence** to
  calibrate `RRFK` and the floors; an actual retune is a separate change so the
  baseline numbers are measured against today's behavior first.
- Shipping an automated LLM-as-judge as a gate. `SPEC.md` §14 grades judgment-heavy
  metrics (instruction quality, repair usefulness) by human/review-board until a
  calibrated judge exists; we build the rubric scaffold, not the judge.
- A hosted dashboard or time-series database. Tracking is git-diffable JSON
  snapshots + a regenerated Markdown scoreboard.
- Full ContextSpy productization — at most a clearly-marked optional hook (§14.3).

## Decisions

### Layout: `internal/eval` engine + `evals/` data and reports, one module

The harness engine lives in `internal/eval` (Go, reusing the existing internal
seams); the corpus, thresholds, and generated reports live in a top-level `evals/`
directory:

```
internal/eval/            # engine: loader, runners, metrics, reporters, token est.
evals/
  data/
    catalog/world.json     # the synthetic downstream MCP catalog ("the world")
    discovery/*.jsonl      # labeled intents, tagged by category
    invocation/*.json      # valid / invalid / schema-drift / offline scenarios
    ergonomics/*.json      # instructional-quality + parity + budget cases
    thresholds.json        # gate thresholds (data, ratchetable)
  snapshots/baseline.json  # committed baseline; timestamped runs gitignored
  BENCHMARKS.md            # public scoreboard (generated, committed)
  METHODOLOGY.md           # the math: formulas, calibration, rubrics, CIs
  README.md                # how to run, how to add data
```

Keeping it in the main module lets the harness import `internal/broker`,
`internal/search`, `internal/sidecar`, and `internal/contract` directly and lets
performance benchmarks be ordinary `testing.B` functions. _Alternative
considered:_ a separate Go module or a Python harness — rejected; it would
duplicate the contract types and re-cross the very boundary the broker keeps
clean.

### Data-driven corpus over inline fixtures

Every scenario is a data file validated against a documented, versioned schema;
the loader maps catalog entries into `catalog.Tool` and loads them via
`catalog.NewMemory()` so the real engine ranks them. This directly retires the
inline-fixture anti-pattern in `cli_test.go` (those two tests become the seed of
the discovery gold set, expressed as data). _Alternative considered:_ keep growing
Go-coded fixtures — rejected; a gold set that needs a compiler to grow will not
grow, and it tempts authoring tests the system is guaranteed to pass.

### Drive the real seams, in-process

- **Discovery** runs through `broker.FindTool` (and `search.Engine`/`Decide`
  directly for ranked-list metrics like MRR that need the full order).
- **Invocation/repair** runs through `broker.CallTool` against fixture downstream
  servers (the in-process fake MCP server already used by the acceptance tests),
  including offline and schema-drift variants.
- **Ergonomics + parity** run the same inputs through both the CLI `--format json`
  path and the in-process `mcp.Adapter`, then compare.

No subprocess orchestration for the core metrics; the CLI/MCP paths are exercised
in-process for determinism, with at least one real end-to-end `ozy` smoke
invocation to prove the wiring.

### Real embedding model, opt-in gated

The semantic and hybrid accuracy evals provision the real sidecar into a temp XDG
state dir, embed the corpus, and query with the real model — the committed
semantic numbers must be real, per the request. The leg is gated behind an
explicit opt-in (`OZY_EVAL_SEMANTIC=1`, mirroring the sidecar's
`@pytest.mark.slow`); when off or unprovisionable, the harness records the
semantic-only metrics as `skipped: semantic unavailable` and still runs lexical +
structural + token + e2e. This keeps `go test ./...` fast and CI green without a
model download, while the definitive benchmark run (and a dedicated CI job) sets
the flag. _Alternative considered:_ always run the real model — rejected; it makes
the default test loop slow and network-dependent and contradicts §10.1's
lexical-baseline-must-stand principle.

### Metrics: deterministic point metrics, defined once in METHODOLOGY

The §14.2 families are deterministic retrieval/invocation metrics (no sampling),
so they are computed exactly:

- Discovery: top-1, top-3, MRR (`1/rank` of the first acceptable target, 0 if
  absent), wrong-server rate (selected a tool from the wrong server), no-match
  correctness (refused when it should refuse).
- Invocation: valid-argument rate, first-call success, repair success (corrected
  call after a structured error succeeds), schema-error rate, error clarity (a
  structural proxy — error has type + actionable, conditional, grounded
  `agentInstruction`).
- Token economy: startup schema tokens, tokens-to-success, largest payload, broker
  calls, for direct-MCP baseline vs Ozy.

Because the metrics are deterministic, Pass@k / Pass^k reliability sampling
(evaluation-harness-designer) does **not** apply here and is intentionally
reserved for any future agent-in-the-loop scenario. The exact formulas, the floor
/ `k` calibration procedure, and confidence-interval treatment for the judged
metrics live in `METHODOLOGY.md`.

### Token measurement: documented heuristic estimator behind a seam

Token-economy metrics use a documented, deterministic estimator behind a small
interface (default: a character/whitespace heuristic calibrated to typical BPE
ratios). Agent context tokens are model-specific, so the estimator is explicitly
swappable and the methodology doc states which estimator produced the committed
numbers; a real BPE tokenizer (e.g. tiktoken-go) can replace it without touching
metric code. The headline token story — Ozy exposes 3 tool schemas vs. the full
downstream universe at startup — is robust to estimator choice, which is the point
of §13. _Alternative considered:_ mandate a real tokenizer dependency now —
rejected as premature; the seam makes it a drop-in later.

### Ergonomics: structural invariants now, rubric scaffold for judged metrics

Automatable checks encode the §4.5 quality bar and §9 contract shapes as
invariants over real responses, organized by the agent-ergonomics domains:
every `findTool` result carries an explicit decision and a grounded/conditional/
actionable next step; errors carry a type and a non-amplifying retry disposition;
responses respect §13 budgets; CLI and MCP agree (parity). The judgment-heavy
metrics (instruction usefulness, repair usefulness) get a versioned rubric and a
results template for human/review-board scoring — not an automated judge gate.
This matches §14's grading approach and keeps the gate trustworthy.

### Reporting: JSON snapshot is source of truth; Markdown is generated

A run emits one structured JSON snapshot (metrics + provenance + verdict). The
public `BENCHMARKS.md` is generated from a snapshot so the prose can never drift
from the numbers, and the committed `baseline.json` lets `git diff` show
regressions. Thresholds live in `thresholds.json` and are ratchetable as data.
`METHODOLOGY.md` holds the math, linked from (not inlined into) the scoreboard.

### CLI: wire `ozy eval` through the harness

Replace the `NOT_IMPLEMENTED` stub with `ozy eval run [scenario]` (optionally
scoped to a family) and `ozy eval report`, both honoring `--format`. The command
calls the same `internal/eval` engine and routes discovery/invocation through the
shared broker seam, so the evaluator cannot drift from the evaluated system. Gate
failure → non-zero exit, so CI can call `ozy eval run`.

## Risks / Trade-offs

- **Synthetic corpus may not reflect real downstream tools** → model entries on
  real MCP servers, include near-duplicate cross-server capabilities, document
  provenance, and keep the schema open so real-world catalogs can be added later.
- **Gold set authored to pass (leakage)** → hygiene rules in `METHODOLOGY.md`:
  semantic-paraphrase intents must not echo tool names, every label needs a
  rationale, and a check flags intents the lexical baseline already trivially wins
  in a "semantic" category.
- **Real-model flakiness / download cost on the semantic leg** → opt-in gating,
  clean skip when unavailable, temp state dir per run; the lexical baseline always
  stands (§10.1).
- **Token estimator is approximate** → seam + documented estimator; the relative
  direct-MCP-vs-Ozy comparison, not absolute counts, is the claim.
- **Benchmarks rot if not run** → wire `ozy eval run` into CI (lexical/structural
  always; semantic on a dedicated job), and treat `baseline.json` drift as a
  review signal.
- **Determinism of RRF ties / ANN error** → `SeparationMargin` already classifies
  true ties as `ambiguous`; the harness asserts on the decision, not on fragile
  exact score equality; ANN small-error tolerance is why RRF + two-best is used.
- **Scope is large** → the four areas land as independent scenario families behind
  one loader/reporter, so a partial run (e.g. lexical-only) is still useful and the
  families can be implemented and gated incrementally.

## Migration Plan

Purely additive. New `internal/eval` package and `evals/` tree; the only behavior
change to existing surfaces is `ozy eval` going from `NOT_IMPLEMENTED` to wired
(`cli-interface` delta). The inline `TestDiscoveryEval_*` tests are superseded by
the data-driven discovery gold set and can be removed once the harness reproduces
their coverage. Rollback is deletion of `internal/eval` + `evals/` and reverting
the `eval` command to the stub; nothing else depends on the suite. No data
migration; the corpus and snapshots are self-contained files.

## Open Questions

- **Token estimator:** ship the documented heuristic now (current choice) or pull
  in tiktoken-go for exactness from day one? The seam makes either fine.
- **Snapshot retention:** commit only `baseline.json` (current choice) or also keep
  a rolling history directory in-repo for trend charts?
- **CI semantic cadence:** run the real-model leg on every main-branch push, or
  nightly/release-only to bound cost? (Lexical + structural always run.)
- **Constant calibration ownership:** this change measures the floors/`k`; should
  the follow-up retune live here behind a flag, or strictly as its own change
  (current lean: its own change, so the baseline is measured against today)?
- **ContextSpy:** include the optional measurement hook now (marked optional) or
  defer entirely to a later change?
