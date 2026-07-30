## Context

The scenario bench is fully hermetic (memories `bench-docker-non-negotiable`,
`hermetic-e2e-bench-planned`): runtime egress is the model gateway only, and
`npm_config_registry` is pinned unroutable for the agent process. To honor that,
D9 declined to provision ozy's FastEmbed semantic sidecar (Python + onnxruntime +
model), because the runtime image is `node:alpine` (musl, no reliable onnxruntime
wheels) and provisioning would need a runtime fetch. So `ozy index` falls back to
lexical term-overlap ranking, and the bench measures **lexical** ozy. Full
analysis and file:line evidence: `bench/docs/semantic-fork.md`.

Two mechanisms already exist and are reused unchanged:

- `internal/sidecar/provision.go` — resolves a Python interpreter (uv → python3),
  creates a venv, pip-installs `fastembed==0.8.0` + `turbovec==0.8.0` (faiss-cpu
  only for the opt-in faiss backend), and writes a `.ozy-provisioned` marker keyed
  on versions + model + backend. A subsequent `Provision` is a **no-op** when the
  marker matches — this is what makes a build-time bake reusable at run time.
  Overrides: `OZY_SIDECAR_PYTHON`, `OZY_SIDECAR_SOURCE`, venv via `XDG_STATE_HOME`.
- `sidecar/sidecar/` — the Python package. `FastEmbedEmbedder` downloads the model
  into `<data_dir>/models` (`TextEmbedding(cache_dir=...)`) on first embed;
  `data_dir` = `--data-dir` / `$OZY_SIDECAR_STATE_DIR` / `~/.local/state/ozy/sidecar`.

Ozy config already defaults semantic **ON** and the embedding model + `turbovec`
backend to the same values the provisioner installs, so the config the bench
writes needs no semantic-specific shape change — the gap is purely the missing
toolchain + baked model + offline env in the image.

## Goals / Non-Goals

**Goals:**
- Ozy exercises real semantic retrieval in the bench, with the runtime still
  hermetic (no embedder or model fetch at run time).
- Provenance honestly reports `retrieval: semantic`.
- The `historical-weather-report` search-tool criterion is winnable and stops
  confounding the semantic-vs-lexical delta.

**Non-Goals:**
- Shipping semantic to any non-bench image (production install is unchanged).
- Re-running or publishing new scoreboard numbers (a follow-up once the stack is
  verified live; this change lands the capability, not the results).
- Making FastEmbed work on musl/alpine — the base-image swap is the point.

## Decisions

### D1: Swap the runtime base `node:alpine` → glibc `node:22-bookworm-slim` + python3
onnxruntime (FastEmbed's engine) has no reliable musl wheels; Debian glibc has
prebuilt wheels for onnxruntime, fastembed, and turbovec. `node:22-bookworm-slim`
keeps the official Node runtime OpenCode needs and adds glibc; `apt-get install
python3 python3-venv` supplies the interpreter. Alternatives: (a) stay on alpine and
compile onnxruntime — rejected, no reliable path; (b) a separate embedder
sidecar container — rejected, breaks the single hermetic image and the in-process
`ozy index` flow. Cost: image grows ~200–400 MB (accepted, noted in provenance).

### D2: Provision the venv AND download the model at build time, into a baked state dir
Set `OZY_SIDECAR_STATE_DIR=/bench/sidecar-state` (and a fixed `XDG_STATE_HOME`) and
run one real `ozy index` over a tiny corpus at **build** time (network allowed).
That single call drives the existing provisioner (creates the venv, pip-installs
the pinned deps, writes the `.ozy-provisioned` marker) **and** triggers the first
embed, which downloads `BAAI/bge-small-en-v1.5` into `/bench/sidecar-state/models`.
At run time the marker matches and the model cache is present, so `Provision` is a
no-op and the embed loads from disk. Alternative: bake only the venv and download
the model at run time — rejected, that is exactly the forbidden runtime fetch.

### D3: Enforce offline model use at run time with `HF_HUB_OFFLINE=1` (+ `TRANSFORMERS_OFFLINE=1`)
A baked cache is necessary but not sufficient: FastEmbed / huggingface_hub may still
probe the hub for revisions. Setting these envs in the runtime image forces
local-files-only, turning any accidental network dependence into a clean local load
rather than a hang or a hermeticity breach. Belt-and-suspenders alongside the
existing `npm_config_registry` guard.

### D4: Build-time hermeticity smoke covers the semantic index
Extend the existing "broken build-time state fails the build" gate: after baking,
re-run `ozy index` with the npm registry **and** the model hub made unroutable and
assert it still produces a **semantic** index (non-zero vector count). A lexical
fallback here fails the build — a run must never silently degrade. This is the
build-time counterpart to the `retrieval: semantic` provenance assertion.

### D5: Keep `setupOzy` config minimal; pin only what makes the marker match
Semantic is already default-on and the model/backend defaults already equal the
provisioner's, so no `search.semantic` block is strictly required. The one hard
requirement is that the runtime embedding model + backend equal what was baked
(`BAAI/bge-small-en-v1.5` + `turbovec`) so the marker matches and no reprovision is
attempted. Pin them explicitly in the written config (cheap insurance) and set the
`OZY_SIDECAR_*` / `XDG_STATE_HOME` envs at the **image** level so both the
build-time bake and the run-time `ozy index` resolve the same venv + model.
`resolveRetrievalStack()` needs no code change — it reads `OZY_BENCH_RETRIEVAL`,
which we flip to `semantic`.

### D6: `turbovec` (default) backend, not faiss
`turbovec` is the pure-Python default with no external C deps; `faiss-cpu` carries C
deps and is opt-in. Staying on the default keeps the baked image simpler and the
provisioner's marker/versions aligned. Verify at build time that the
`fastembed` + `turbovec` wheels install cleanly on the chosen glibc base.

### D7: Disambiguate the scenario by **naming DuckDuckGo** in the task
`historical-weather-report` currently says research via "a privacy-focused search
engine" — which lexically and semantically matches the `brave-search` /
`kagi-search` distractors as well as DuckDuckGo, an unwinnable near-duplicate
collision (the model picks Brave 4/5 times). The user's intent and the doc's
preferred fix converge on making the required tool unambiguous: change the task to
name **DuckDuckGo**. `required_tools` still requires `duckduckgo/search`.
Alternatives considered: (a) relax `required_tools` to accept any web search —
rejected, the user wants DuckDuckGo specifically; (b) delete the distractors —
rejected, removing distractors is the structural freebie that inflates direct-lean
and is the opposite of a retrieval-at-scale test; (c) craft a non-naming paraphrase
unique to DuckDuckGo's tool description — viable but fragile against the model's
"Brave = privacy" brand prior, and less clear than naming it. Optional hardening:
add `brave-search`/`kagi-search` to `forbidden_tool_patterns` so a wrong-engine pick
is an explicit graded failure. Naming the tool does not gut the semantic test —
retrieval-at-scale is still exercised across the full 556-tool corpus for the
weather/geocode/wikipedia/pdf selections and for pulling `duckduckgo/search` out of
the corpus.

## Risks / Trade-offs

- **Runtime model fetch slips through (hermeticity breach)** → D3 offline envs +
  D4 build-time smoke with the hub unroutable; the run also keeps the npm guard.
- **Baked marker drifts from runtime config → reprovision attempt at run time
  (which would fail offline)** → D5 pins the exact model + backend in the written
  config so the marker matches; D4 smoke would catch a mismatch at build time.
- **Image bloat (~1.2 GB — measured, larger than the initial ~200–400 MB guess;
  onnxruntime + numpy + pillow + fastembed deps + model dominate)** → accepted;
  recorded in the README so the size is understood, not surprising.
- **Naming DuckDuckGo makes that one tool a near-exact lexical match, weakening it
  as a semantic probe** → accepted; the semantic signal is carried by the other
  selections and by 556-tool-scale retrieval. The alternative (keep the paraphrase)
  leaves the criterion unwinnable, which is worse.
- **Semantic may not cleanly beat lexical on the remaining task** → out of scope
  here (this change enables measurement, not a specific result); interpret the live
  re-run separately.

## Migration Plan

1. Land the image + scenario changes; `docker build` the bench image.
2. Build-time smoke (D4) gates the image — a non-semantic index fails the build.
3. Live E2E run in Docker: confirm `environment.json` → `retrieval: semantic`,
   non-zero vector count, zero runtime fetch, and the weather scenario passes both
   modes. Rollback is reverting to the alpine image + `OZY_BENCH_RETRIEVAL=lexical`
   (the prior state is a single Dockerfile revert).

## Open Questions

- Exact glibc base tag (`node:22-bookworm-slim` vs `debian-bookworm-slim` + nodejs
  apt) — pick whichever installs the fastembed/turbovec/onnxruntime wheels cleanly
  and keeps OpenCode working; decide at build time.
- Whether to add the distractors to `forbidden_tool_patterns` (stricter grading) or
  rely on `required_tools` alone — default to `required_tools` only unless the live
  re-run shows the model still wanders.
