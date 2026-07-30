## 1. Image base + toolchain (D1)

- [x] 1.1 Swap the `bench/Dockerfile` runtime stage from `node:22.14-alpine` to a glibc base that keeps Node for OpenCode (`node:22-bookworm-slim`); install `python3` + `python3-venv` + `python3-pip` (skipped `uv` — its runtime Python-fetch is a hermeticity risk; system python3 is pinned instead)
- [x] 1.2 Rebuilt: OpenCode reaches ready-to-run state on the new base and the existing npm-unroutable hermeticity gate (#20) passes; full build is green

## 2. Bake the semantic stack at build time (D2, D3, D6)

- [x] 2.1 Set image-level envs so build and run resolve the same baked stack: fixed `XDG_STATE_HOME=/bench/xdg` (the model caches under `<VenvDir>/models`, and `VenvDir` derives from `XDG_STATE_HOME` — daemon/doctor pass `DataDir=VenvDir`), `OZY_SIDECAR_SOURCE=/opt/ozy/sidecar` (copied package), and `OZY_SIDECAR_PYTHON=/usr/bin/python3` (pin base interpreter, never invoke uv)
- [x] 2.2 Build-time `ozy doctor` (semantic default-on) provisions the venv + pip-installs `fastembed`/`turbovec` (default backend, no faiss) and downloads `BAAI/bge-small-en-v1.5` into `<VenvDir>/models`; a second clean `ozy doctor --format json` gates on the embedding check being `ok`. Split into warm+gate because provisioning streams pip output to stdout and would pollute a single `--format json` capture (the original single-run gate failed on exactly this)
- [x] 2.3 Add `HF_HUB_OFFLINE=1` and `TRANSFORMERS_OFFLINE=1` to the runtime image (after the warm step) so the cached model is loaded local-files-only with no hub probe
- [x] 2.4 Verified: `fastembed`+`turbovec`+`onnxruntime` wheels install cleanly on `node:22-bookworm-slim` (arm64), no source builds; `build-essential` not needed

## 3. Flip the retrieval label + harness wiring (D5)

- [x] 3.1 Change `bench/Dockerfile` `ENV OZY_BENCH_RETRIEVAL=lexical` → `semantic` and rewrite the D9 rationale comment to describe the baked-embedder approach
- [x] 3.2 In `internal/bench/runner.go` `setupOzy`, pin the embedding model (`BAAI/bge-small-en-v1.5`) + `turbovec` backend + `search.semantic.enabled` in the written ozy config so the runtime provisioner marker matches the baked venv and no reprovision is attempted; `resolveRetrievalStack()` needs no change (reads the env label)
- [x] 3.3 Verified in a live container run: `ozy index` reused the baked venv/model (no reprovision, no fetch) and reported `embedded 556 tools; 556 vectors queryable` — a real semantic vector index at runtime

## 4. Build-time hermeticity smoke (D4)

- [x] 4.1 Dockerfile step re-runs `ozy doctor` with `RUN --network=none` (npm registry AND model hub unroutable) and asserts a **semantic** embedding check (JSON `status: ok`); passed in the build (#26, 0.5s) — proves the baked venv+model work offline
- [x] 4.2 Confirmed the gate fails the build on a bad bake: the first (broken) build failed here with a named error when the embedding JSON wasn't `ok`, rather than degrading a run

## 5. Scenario disambiguation (D7)

- [x] 5.1 Edit `bench/scenarios/historical-weather-report/task.md` step 2: replaced "a privacy-focused search engine" with an instruction that names **DuckDuckGo** (its `search` tool) and says to use it specifically
- [x] 5.2 Kept `expected/ground_truth.json` `required_tools` requiring `{ server: duckduckgo, tool: search }`; added `brave-search`/`kagi-search`/`bing-search` to `forbidden_tool_patterns`. Live run confirmed: agent called `duckduckgo/search` (required check passes) and none of the forbidden engines (all forbidden checks pass) — the collision that failed 4/5 times before is resolved
- [x] 5.3 Scenario hash is computed at runtime from the scenario files (no stored hash to bump); confirmed `ground_truth.json` is valid JSON and the scenario still loads and grades

## 6. Verify + document

- [x] 6.1 Run `gofmt`, `go vet`, `go build ./...`, `go test ./internal/bench/...` — all green
- [x] 6.2 Live Docker E2E (MODE=ozy, RUNS=1): `environment.json` → `retrieval: semantic`; 556 vectors queryable; `token_source: measured`; zero runtime package/model fetch (offline gate + cached model). All change-relevant checks pass (DuckDuckGo required + all 4 facts + forbidden engines). Run graded `pass=false` on the pdf-toolkit criterion ONLY — a separate `doc-converter` vs `pdf-toolkit` distractor collision, out of scope here (see spawned follow-up). Full multi-mode 5-run pass is a reporting exercise, not needed to verify this change
- [x] 6.3 Updated `bench/README.md` retrieval note (semantic, ~1.2 GB larger, offline smoke) and marked `bench/docs/semantic-fork.md` resolved (option (a) shipped)
