# Handoff — Bench measures LEXICAL-only ozy (semantic disabled)

> **RESOLVED 2026-07-16 → option (a) shipped.** Semantic is now baked into the bench
> image at build time (glibc base + FastEmbed venv + model, runtime stays hermetic)
> and the `historical-weather-report` scenario names DuckDuckGo specifically. See
> OpenSpec change `bench-enable-semantic` (`openspec/changes/bench-enable-semantic/`).
> The analysis below is kept for context; it describes the pre-change state.

**Owner decision pending. Everything below is verified against the code as of 2026-07-16.**

## TL;DR

The scenario bench runs ozy with **semantic search OFF → lexical-only retrieval**.
It's a deliberate hermetic-image tradeoff (D9), honestly labeled — but it disables
ozy's core feature and makes every "ozy vs direct / direct-lean" *success* and
*retrieval-quality* comparison structurally unfair to ozy. The measured "ozy loses
on success" result is **lexical ozy losing on retrieval — the exact axis semantic
ozy is designed to win.** Do not read current bench numbers as ozy's performance.

## The fork (what to decide)

- **(a) Enable semantic in the bench** — revisit D9 by baking the embedder at build
  time so runtime stays hermetic. The only way the comparison is fair. Scope as its
  own change. *Recommended.*
- **(b) Keep lexical, make reporting shout it** — verdict leads with "⚠️ ozy running
  LEXICAL-only; semantic (its default) not measured." Cheap, honest, ~1 file.

Recommended: (b) now, (a) as a proper follow-up change. **(a) must ship with the
companion scenario fix below** — otherwise a semantic re-run is uninterpretable
(any null result could be the near-synonym collision, not semantic itself).

**Failure-mode caveat (see analysis below):** 100% of ozy's failures are one
tool-selection collision (`brave-search` distractor vs required `duckduckgo`).
That's purely retrieval — so semantic *should* help — but the pair is a genuine
near-synonym, so the flip is likely-but-not-clean until the scenario ambiguity is
fixed too.

## Evidence (exact file:line)

- `bench/Dockerfile:52-61` — rationale comment + `ENV OZY_BENCH_RETRIEVAL=lexical`.
- `internal/bench/provenance.go:56-64` — `resolveRetrievalStack()` just returns the
  `OZY_BENCH_RETRIEVAL` env (a **pinned build-time label, not live detection**);
  "unknown" if unset.
- Every run records it: `environment.json` → `"retrieval":"lexical"`;
  `comparison.md` header → `Retrieval: lexical`.
- Ozy's real default is hybrid semantic+lexical, **semantic ON**:
  `internal/config/config.go:190-220` (semantic defaults to enabled when the
  `search.semantic` section is omitted), `internal/config/scaffold.go:56-59`.
- Embedding default: `internal/config/config.go:147` →
  `DefaultEmbeddingModel = "BAAI/bge-small-en-v1.5"`; vector backend default
  `turbovec` (`config.go:133`), faiss opt-in.

## Why it's lexical (the D9 rationale, from the Dockerfile)

Semantic embedder = FastEmbed = Python + onnxruntime + embedding model. The bench
runtime image is `node:alpine` (musl), where onnxruntime has no reliable wheels,
and the hermetic contract forbids fetching at runtime. So `ozy index` falls back to
lexical term-overlap ranking. D9 declined to bake the sidecar (Python + onnx on
musl + bundled model).

## Impact — measured (weather scenario, `opencode/big-pickle`, RUNS=5, two passes)

| mode | pass1 success | pass2 success | canonical hits /4 | tokens/run |
|---|---|---|---|---|
| direct (full 556 tools) | 3/5 | 5/5 | 3.8 / 4.0 | ~82k–90k (some 300–600s timeouts) |
| direct-lean (52 tools) | 5/5 | 4/5 | 4.0 / 3.8 | ~24k–26k |
| **ozy (lexical)** | **3/5** | **1/5** | **3.6 / 3.2** | ~16k (cheapest) |

Consistent signal: ozy is cheapest on tokens, least reliable on success, worst
canonical-hit rate — **because retrieval is lexical.** direct-lean is currently the
"best" mode. Expect this to move materially once semantic is on.

## Failure-mode analysis (why ozy fails — pass2, `130748` run dir)

Every ozy failure is the **same single miss**: the model called `brave-search`
(a corpus distractor) instead of the required `duckduckgo/search`. Every other
criterion passed in every run (weather, wikipedia, pdf, all answer facts, the PDF
artifact). Ozy's entire success gap is one tool-selection collision.

| ozy run | search tool called | pass |
|---|---|---|
| 1 | `brave-search/web_search` | ✗ |
| 2 | `duckduckgo/search` | ✓ |
| 3 | `brave-search/web_search` | ✗ |
| 4 | `brave-search/web_search` (+retries) | ✗ |
| 5 | `brave-search/web_search` (×2) | ✗ |

The task says research via *"a privacy-focused search engine"* (deliberately
unnamed). `findTool` for that query surfaces `brave-search` and the model picks it
4/5 times.

## Will semantic actually flip it? (sharpened)

**Supports the flip:** the failures are **100% retrieval-quality** — the exact axis
semantic is built to win — and ozy already dominates tokens (16k vs lean 25k vs
direct 90k). Better ranking → cheaper *and* more reliable at once.

**Tempers "dramatically":**
1. **Near-synonym collision.** Brave markets itself as a *privacy-focused search
   engine* — an excellent match for the phrase both lexically **and semantically**.
   Embeddings rank near-duplicates close together, so semantic is **not guaranteed**
   to cleanly pick DuckDuckGo over Brave. This specific pair is hard for *any*
   retriever.
2. **direct-lean's win is partly structural, not retrieval skill.** Lean wires no
   corpus, so `brave-search` isn't present — the model *can't* pick it. Ozy always
   has the full corpus behind `findTool`, so it can never get lean's "distractor
   doesn't exist" freebie; its ceiling on this collision is inherently below lean's.
3. **The model completed the task without the "right" tool** (city facts from
   Wikipedia, correct PDF). The failing check is strict *retrieval-attribution*, not
   task failure.

**Read:** semantic makes ozy competitive-to-winning, but this scenario is rigged
against a *clean* flip. To prove or disprove the flip you must (i) enable semantic
AND (ii) fix the scenario ambiguity below — otherwise a null result is
uninterpretable (was it semantic, or the near-duplicate?).

## Companion item — fix the search-tool ambiguity (do alongside)

`historical-weather-report` tests near-duplicate disambiguation rather than
paraphrase→right-tool. The corpus ships **multiple privacy-adjacent search
distractors** — `bench/corpus/{brave-search,kagi-search,bing-search,elasticsearch}.json`
— and both Brave *and* Kagi market themselves as "privacy-focused search engines,"
so the unnamed *"a privacy-focused search engine"* phrasing collides with the
required `duckduckgo/search` against ≥2 equally-valid matches. That's unwinnable for
any retriever and pollutes the semantic-vs-lexical delta. Fix options (prefer the
first — deleting distractors just moves the collision):
- **Change the task paraphrase** so the correct tool is semantically **distinct**
  from every distractor (e.g. a phrasing that uniquely fits DuckDuckGo's tool
  description, not "privacy-focused" which several distractors share). Then semantic
  clearly beats lexical *and* neutralizes lean's structural luck. **or**
- **Relax the ground-truth `required_tools`** to accept any functional web-search
  call, if the intent is "used *a* search tool," not "duckduckgo specifically." **or**
- Remove *all* the privacy-adjacent search distractors (brave + kagi at least) —
  weakest option, since removing distractors is exactly the structural freebie that
  makes lean look good and is the opposite of a retrieval-at-scale test.

Ground truth: `bench/scenarios/historical-weather-report/expected/ground_truth.json`
(`required_tools` includes `{server: duckduckgo, tool: search}`).

## Implementation plan for (a) — build-time-baked semantic, runtime still hermetic

1. **Base image swap**: `bench/Dockerfile` runtime from `node:alpine` → a glibc base
   (e.g. `debian-bookworm-slim`); keep node (for opencode) + add `python3` + venv.
2. **Provision at BUILD time** (network allowed): create the FastEmbed venv and
   **download + cache the embedding model** (`BAAI/bge-small-en-v1.5`) into the image
   so runtime needs no fetch. Keep it in the *builder* or a cached layer.
3. **`setupOzy`** (`internal/bench/runner.go` ~line 558): write the per-run ozy config
   with semantic enabled (`search.semantic.enabled=true`, embedding model +
   `turbovec` backend), and set the `OZY_*` env so `ozy index` / `ozy mcp` find the
   baked venv/model. Verify `ozy index` builds the vector index **offline**.
4. **Flip the label**: `ENV OZY_BENCH_RETRIEVAL=semantic` (or `hybrid`) + update the
   Dockerfile comment; `resolveRetrievalStack()` needs no code change.
5. **Re-verify hermeticity**: runtime egress must stay model-gateway-only (the
   `npm_config_registry=http://127.0.0.1:0` guard stays; confirm zero embed fetch at
   run time). Build-time smoke: `ozy index` yields a semantic index; a live run's
   `environment.json` shows `retrieval:semantic`.
6. **Re-run** RUNS=5 weather; compare **semantic-ozy vs direct-lean** — the real
   product question.

## Constraints / gotchas

- HARD RULE: bench runs in Docker, hermetic; runtime egress = model gateway only
  (memories: `bench-docker-non-negotiable`, `hermetic-e2e-bench-planned`).
- Bake the model at **build** time; never fetch at runtime.
- Don't try to make FastEmbed work on alpine/musl — the base-image swap is the point.
- Verify `turbovec` is pure-Go (zero external C deps) before relying on it in the
  hermetic image; avoid `faiss` (C deps).
- Image grows ~200–400 MB (python + onnx + model). Acceptable for measuring the real
  feature; note it in the change.

## Surrounding state at handoff (uncommitted, all green: build/vet/test/gofmt)

On this branch, not yet committed: `direct-lean` mode, run-timeout process-group
teardown fix, `opencode/big-pickle` default model, `historical-weather-report`
default scenario, and deletion of the old `suspended-account-invoice-regression`
scenario + its toolsets/fixtures/sqlite. The OpenSpec change `bench-direct-lean-mode`
is 19/19. This **semantic fork is NOT started** — decision pending. See memory
`bench-direct-lean-mode-state`.
