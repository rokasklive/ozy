# Ozy eval methodology

This is the rigorous companion to [BENCHMARKS.md](BENCHMARKS.md). The scoreboard is
the at-a-glance summary; this document holds the metric definitions, the
calibration procedures, the gold-set hygiene rules, and the known weaknesses the
suite has surfaced. If you want to know *exactly* what a number means or how a
gate was chosen, it is here.

The suite implements the eval framework in [`SPEC.md`](../SPEC.md) §14. It is
**definitive**: every change to Ozy is expected to measure against it, and the
committed [baseline snapshot](snapshots/baseline.json) is the reference the next
run is diffed against.

---

## Discovery metrics

For each labeled intent the harness asks the real search engine to rank the whole
corpus catalog, then scores the ranked order against the intent's `acceptable`
toolRef set. Definitions, over the set of *matchable* cases (those with a
non-empty `acceptable` list):

| Metric | Definition |
| --- | --- |
| **Top-1** | fraction where rank 1 is an acceptable tool |
| **Top-3** | fraction where an acceptable tool appears in ranks 1–3 |
| **MRR** | mean of `1/rank` of the first acceptable tool (0 if none appear) |
| **Wrong-server rate** | among `wrong_server` cases, fraction where rank 1 is the *right capability on the wrong server* — i.e. not acceptable, different server, same leading operation token (e.g. another `search_*`). Lower is better. |
| **No-match correctness** | among `no_match` cases (empty `acceptable`), fraction where the broker decision is `no_good_match` or `catalog_empty` — i.e. it correctly refused. |

Metrics are reported **per category** and as an **overall** roll-up (matchable
metrics over all matchable cases; no-match correctness over all no-match cases;
wrong-server rate over all wrong-server cases).

Because retrieval is deterministic, these are exact point metrics — there is no
sampling. Reliability sampling (Pass@k / Pass^k) is intentionally *not* used here;
it is reserved for any future agent-in-the-loop scenario.

## Hybrid ranking and the calibrated constants

Lexical ranking is field-weighted BM25; the semantic leg (when run) is the real
FastEmbed model served by the sidecar. The two ranked lists are fused with
**Reciprocal Rank Fusion**:

```
score(tool) = Σ_lists  1 / (k + rank_in_list)      k = 60
```

RRF is rank-based, so BM25 scores and cosine similarities never need a common
scale. RRF gives *ordering* but not *absolute relevance*, so the `no_good_match`
floor is evaluated on the component signals, not the fused score:

- a tool is a "real" match if `normalizedLexical = s/(s+k_sat) ≥ LexicalRelevanceFloor`
  **or** `cosine ≥ SemanticRelevanceFloor`;
- `use` vs `ambiguous` is decided by the RRF gap between the top two.

The tunable constants live in [`internal/search/fusion.go`](../internal/search/fusion.go)
(`RRFK`, `LexSatK`, `LexicalRelevanceFloor`, `SemanticRelevanceFloor`,
`SeparationMargin`, `HighConfidenceThreshold`). This suite produces the **evidence**
to calibrate them; retuning them is a separate change so the baseline is first
measured against today's behavior.

## Invocation and repair metrics

The invocation family drives `broker.CallTool` against an in-process fixture
downstream (the same connector/session seam the broker uses in production) over
the `invocation/*.json` scenarios. Metrics, each a rate over the scenarios in its
family:

| Metric | Definition |
| --- | --- |
| **Valid-argument rate** | of calls expected to be schema-valid (success first calls + repair corrected calls), the fraction that validate against the cataloged schema |
| **First-call success** | of `success` scenarios, the fraction whose first broker call returns `ok` |
| **Repair success** | of `repair` scenarios, the fraction where the first call fails with the expected structured error **and** the corrected call then succeeds |
| **Schema-drift caught** | of `TOOL_SCHEMA_CHANGED` scenarios, the fraction correctly surfaced as drift |
| **Offline handled** | of `DOWNSTREAM_SERVER_OFFLINE` scenarios, the fraction correctly surfaced as offline |
| **Error clarity** | of all observed structured errors, the fraction that carry a type, a non-empty `agentInstruction`, and a non-retry-amplifying disposition |

**What runs through the broker vs. what is modeled.** Ozy's broker delegates
argument validation to the downstream server and does not yet detect schema drift
(retuning that is an explicit non-goal of this change). So the harness *models*
the agent-side checks an agent performs against the schema preview / describeTool
output: argument validation against the cataloged schema produces
`ARGUMENT_VALIDATION_FAILED`, and arguments valid against the catalog but invalid
against the scenario's `liveSchema` produce `TOOL_SCHEMA_CHANGED`. The corrected
repair call and the offline path run through the real `broker.CallTool` seam. A
"non-amplifying" disposition means the error is non-retryable, or retryable only
behind a precondition (backoff, a health check, a refresh) rather than inviting an
immediate unbounded retry.

## Agent ergonomics and parity

The ergonomics family exercises each `ergonomics/*.json` case on **both** the CLI
`--format json` path and the in-process MCP adapter over one shared broker, then
applies structural §4.5/§9/§13 checks, organized by the agent-ergonomics domains:

| Metric | Domain | Definition |
| --- | --- | --- |
| **Decision rate** | decision clarity | every `findTool` result carries an explicit, known decision value |
| **Instruction rate** | actionable guidance | every `findTool` result has a `nextAction` or a grounded `agentInstruction` that names a concrete step — a response that merely restates the query is flagged |
| **Error disposition rate** | recoverability | every structured error carries a type and a retry disposition |
| **Within-budget rate** | response economy | each rendered response stays under the per-surface §13 byte budget (findTool returns a schema *preview*, never full schemas) |
| **Parity rate** | surface consistency | the CLI and MCP surfaces produce the same decision, selected toolRef, and error type for the same input |

Parity is measured against the **real** MCP adapter: the harness stands up
`internal/mcp` over an in-memory transport and calls `findTool`/`describeTool`/
`callTool` through a client session, so a future divergence in either adapter is
caught, not assumed away.

## Token economy

Token-economy metrics (startup schema tokens, tokens-to-success, largest payload,
broker calls; direct-MCP baseline vs Ozy) use a documented, swappable estimator.
The default is a `chars/4` heuristic (see [`internal/eval/token.go`](../internal/eval/token.go));
the run provenance records which estimator produced the numbers. The headline §13
story — Ozy exposes 3 tool schemas vs. the whole downstream universe at startup —
is a ratio and is robust to estimator choice (the suite asserts this by re-running
with a one-token-per-rune estimator). A real BPE tokenizer can replace the default
without touching metric code. **Startup tokens** sum each tool's name +
description + full input schema (direct-MCP: every cataloged tool; Ozy: the three
Ozy tools). **Tokens-to-success** adds the per-task exchange to startup: direct
pays the full universe up front then one call; Ozy pays three small tool schemas
then `findTool`→`describeTool`→`callTool`.

## Performance (latency)

`RunLatency` samples each retrieval path `latencyIters` times (after a warmup) and
reports p50/p95 (microseconds) and throughput for the lexical ranker, RRF fusion
(timed on a precomputed ranking), the end-to-end broker `findTool` path, and — when
the gated leg ran — the real semantic path. Latency is **environment-dependent**,
so it is reported with the run host in the provenance line and is **never gated**;
the `testing.B` benchmarks in [`perf_test.go`](../internal/eval/perf_test.go)
remain the tool for local A/B comparison.

## Gold-set hygiene rules

The gold sets only stay meaningful if they are honest:

1. **No lexical freebies in the `semantic` category.** A semantic-paraphrase intent
   must not echo the target tool's name or distinctive description words. The
   harness enforces this: `Hygiene` runs the lexical-only baseline over every
   `semantic` intent and **flags any the lexical ranker already wins at rank 1**.
   Those are reported as warnings and must be reworded or recategorized. (Today:
   0 warnings — see the baseline.)
2. **Every label carries a rationale.** The loader rejects any case without one.
3. **`acceptable` refs must exist** in the corpus catalog; the loader rejects
   dangling references with a file-and-field error.
4. **`no_match` means empty `acceptable`** and tests refusal, not retrieval.
5. **Adding/retiring labels** is a data-only edit under `evals/data/`; never encode
   a case in Go. Justify the change in the PR so the set does not silently drift
   into a test the system is guaranteed to pass.

## Thresholds: seed low, ratchet up

Gate thresholds in [`data/thresholds.json`](data/thresholds.json) are seeded **at
or just below the measured baseline** so the baseline run is green, then
ratcheted upward as improvements land. A gate marked `requiresSemanticLeg` is
*skipped* (not failed) when a run did not exercise the real embedding model, so
the fast lexical CI stays green while the definitive semantic numbers come from
the opt-in run. Ratcheting is a data-only edit. `no_match` is deliberately
**tracked but not gated** today — gating a metric we know is broken (above) would
either pin it at 0 or wedge the baseline red; it earns a gate once calibration
lifts it.

## What the real-model semantic leg changes

Measured on the committed corpus with `BAAI/bge-small-en-v1.5` (`OZY_EVAL_SEMANTIC=1`),
hybrid RRF vs. the lexical-only baseline:

| Scope | Lexical top-1 | Hybrid top-1 | Lexical MRR | Hybrid MRR |
| --- | ---: | ---: | ---: | ---: |
| semantic (paraphrases) | 0.00 | **0.375** | 0.199 | **0.548** |
| wrong_server | 0.75 | **1.00** | 0.875 | **1.00** |
| lexical / ambiguous | 1.00 | 1.00 | 1.00 | 1.00 |

The dense leg is doing exactly what it should: it recovers paraphrased intents the
BM25 baseline misses entirely and sharpens server disambiguation, with no
regression on the lexical or ambiguous sets. This is the headline reason hybrid is
the default and the `semantic` gate requires the real leg.

## Known weaknesses surfaced by the suite

The point of an eval suite is to tell the truth. As of the committed baseline:

- **No-match refusal is the #1 weakness, and the semantic leg makes it worse.**
  Lexical-only refuses only ≈0.20 of absent-capability intents ("translate this
  document", "charge the card") because the tokenizer keeps common/stopword tokens
  that clear `LexicalRelevanceFloor` on incidental overlap. With the semantic leg
  on it drops to **0.00**: ANN always returns nearest neighbors, and
  `SemanticRelevanceFloor = 0.30` is too permissive to reject genuinely-absent
  capabilities. Concrete calibration targets, each validated by re-running this
  suite: a stopword filter, a higher `LexicalRelevanceFloor`, and a higher
  `SemanticRelevanceFloor` (or a margin between the top hit and the rest).
  Because it is currently broken, `no_match` is **tracked but not gated** (see
  below); it gets a gate once calibration lifts it.

## Judgment-heavy metrics

Instruction usefulness and repair usefulness (`SPEC.md` §14) are graded by
human/review-board against a rubric, **not** an automated LLM judge — there is no
calibrated judge yet, and a miscalibrated judge is worse than none. The rubric
scaffold and scoring template live alongside the ergonomics family as it lands.
