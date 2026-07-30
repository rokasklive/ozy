## Why

The hermetic scenario bench runs ozy with **semantic search OFF → lexical-only
retrieval** (D9: FastEmbed needs Python + onnxruntime, and the `node:alpine`
(musl) runtime image has no reliable wheels, while the hermetic contract forbids
a runtime fetch). That disables ozy's core feature. Every "ozy vs direct /
direct-lean" success and retrieval-quality comparison therefore measures
*lexical* ozy losing on the exact axis *semantic* ozy is built to win — the
numbers are not ozy's real performance.

Compounding this, the `historical-weather-report` scenario asks the agent to
research via "**a privacy-focused search engine**" (unnamed) while requiring the
`duckduckgo/search` tool. The corpus ships near-synonym distractors
(`brave-search`, `kagi-search`) whose branding also reads as "privacy-focused,"
so the criterion is a near-duplicate collision the model loses 4/5 times — an
unwinnable task that pollutes any semantic-vs-lexical delta. A semantic re-run is
uninterpretable until this ambiguity is removed, so both ship together.

## What Changes

- **Bake the semantic embedder at build time; keep runtime hermetic.** Swap the
  bench runtime image from `node:alpine` (musl) to a glibc base (e.g.
  `node:22-bookworm-slim`) with `python3`; at **build** time (network allowed)
  provision the FastEmbed venv (`fastembed` + `turbovec`, the pure-Python default
  backend — no faiss/C deps) and download the `BAAI/bge-small-en-v1.5` model into
  a baked cache. At **run** time ozy reuses the baked venv + model offline, so
  `ozy index` builds a real vector index and `findTool` ranks semantically.
- **Flip the provenance label** `OZY_BENCH_RETRIEVAL=lexical` → `semantic`;
  `environment.json` and `comparison.md` then honestly report `retrieval: semantic`
  and reporting drops the lexical-only caveat.
- **Re-verify hermeticity** — runtime egress stays model-gateway-only. The
  `npm_config_registry` unroutable guard stays; add `HF_HUB_OFFLINE=1` (+
  `TRANSFORMERS_OFFLINE=1`) so the cached model is never re-fetched, and a
  build-time smoke that `ozy index` yields a **semantic** index with the registry
  and HF hub unroutable.
- **Disambiguate the search-tool criterion** in `historical-weather-report`:
  replace the collision-prone "a privacy-focused search engine" instruction with
  one that unambiguously requires **DuckDuckGo** specifically, so the
  `required_tools` check is winnable and the near-synonym distractors no longer
  pollute the retrieval delta.
- Image grows ~200–400 MB (python + onnxruntime + model) — accepted, and noted in
  provenance/README.

Not in scope: shipping semantic to non-bench images, re-running/publishing new
scoreboard numbers (a follow-up once the stack is verified).

## Capabilities

### New Capabilities
<!-- none — this modifies existing bench capabilities -->

### Modified Capabilities
- `scenario-bench`: the **Hermetic runtime** requirement now bakes the semantic
  embedder (venv + model) at build time so ozy exercises **semantic** retrieval at
  run time with zero runtime fetch; the `historical-weather-report` scenario
  requires a **specific, unambiguous** search tool (DuckDuckGo) instead of a
  near-synonym paraphrase.
- `bench-reporting`: retrieval-stack provenance records `semantic` for ozy (the
  baked stack), and the comparison/verdict no longer frames ozy as lexical-only.

## Impact

- **Image / infra**: `bench/Dockerfile` (base swap, build-time venv + model bake,
  offline env, label flip), `bench/docker-compose.yml` if the corpus/state mounts
  change; larger image.
- **Harness code**: `internal/bench/runner.go` `setupOzy` — confirm/pin the
  embedding model + `turbovec` backend so the runtime marker matches the baked
  venv (semantic is already default-on in ozy config, so likely no config-shape
  change); `internal/bench/provenance.go` `resolveRetrievalStack()` needs no code
  change (reads the env label).
- **Scenario**: `bench/scenarios/historical-weather-report/task.md` (paraphrase →
  DuckDuckGo), optionally `expected/ground_truth.json` (`forbidden_tool_patterns`
  to make a wrong-engine pick an explicit failure). `required_tools` still
  requires `duckduckgo/search`.
- **Dependencies**: adds a Python toolchain + `fastembed`/`turbovec`/onnxruntime
  and the bundled model to the bench image only. Reuses the existing
  `internal/sidecar` provisioner and `sidecar/` package unchanged.
- **Docs**: `bench/docs/semantic-fork.md` (the source spec) resolved; `bench/README.md`
  retrieval note updated.
