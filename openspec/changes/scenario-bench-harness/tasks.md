## 1. Scaffold: directories, binary, package

- [x] 1.1 Create the `bench/` tree (`scenarios/`, `configs/`, `runs/`) and add `bench/runs/` to `.gitignore`.
- [x] 1.2 Add `cmd/ozy-bench` (cobra skeleton with `run` and `mcp --toolset` subcommands) and the `internal/bench` package; add a `make bench` build target consistent with the existing `Makefile`.
- [x] 1.3 Add the pure-Go `modernc.org/sqlite` dependency (read-only use), run `go mod tidy`, and confirm `go build ./...`.

## 2. Fixture generator and ground truth (bench-fixtures)

- [x] 2.1 Write the deterministic generator under `bench/scenarios/suspended-account-invoice-regression/` that emits the `acme-billing` source (status enum, status mapper holding the bug, invoice-eligibility service, billing-run processor), a test file, and the docs, then commits a fixed multi-commit history with pinned author/committer identity and `GIT_*_DATE` — the culprit commit maps `SUSPENDED → ACTIVE`, with unrelated docs/cleanup commits as noise.
- [x] 2.2 Generate `db/incident.sqlite` via the `sqlite3` CLI with rows for suspended accounts incorrectly invoiced on 2026-06-14, and make the lifecycle doc state plainly that SUSPENDED accounts are not invoice eligible.
- [x] 2.3 Author `expected/ground_truth.json` (`root_cause_file`, `root_cause_function`, `culprit_commit_subject`, `expected_test`, `expected_patch_file`, `forbidden_behaviors`) and the rigid `task.md` prompt (five required outputs + the rules); have the generator record the resolved culprit hash for the run.
- [x] 2.4 Test: two generations produce an identical tree and the same culprit subject; every `ground_truth.json` path/symbol exists in the fixture; the culprit commit diff is `SUSPENDED → ACTIVE`.

## 3. Fixture MCP servers (bench-mcp-fixtures)

- [x] 3.1 Implement `ozy-bench mcp --toolset` with `code-search` (`search_text`/`search_symbol`/`read_file`/`find_references` via `rg`/file reads over the fixture) and `filesystem` (`read_file`/`list_dir`).
- [x] 3.2 Implement the `git` toolset (`git_log`/`git_show`/`git_blame`/`git_diff`, shelling `git` in the fixture repo).
- [x] 3.3 Implement the `incident-db` toolset (`list_tables`/`describe_table`/`query_readonly`) opening the SQLite DB read-only and rejecting any non-`SELECT`/`PRAGMA`/`EXPLAIN` statement with a structured error.
- [x] 3.4 Implement the `time`, `memory`, and `notes` distractor toolsets.
- [x] 3.5 Test: each toolset advertises only its own tools; `search_text "SUSPENDED"` returns the mapper source; `git_show <culprit>` returns the `SUSPENDED → ACTIVE` diff; write statements are rejected and leave the DB unchanged; distractors are callable but irrelevant.

## 4. Scenario config, loader, and provenance (scenario-bench)

- [x] 4.1 Define the JSONC scenario schema and a loader using `tailscale/hujson` with `{env:VAR}` substitution (name, task file, fixture generator, model env keys, per-mode agent configs, limits incl. `runs` and per-run `timeout_seconds`, expected ground truth, forbidden tools/behaviors); run count resolves from `--runs` / `BENCH_RUNS` / scenario config (default 5).
- [x] 4.2 Compute the scenario hash and build `environment.json` — model name, sanitized base URL (reuse `internal/config` redaction), temperature, context window, max tokens, timestamp, Ozy git SHA, mode(s), run count.
- [x] 4.3 Test: loader resolves env and sanitizes the base URL (no API key leaks); the scenario hash changes when the prompt changes.

## 5. Static surface tier and surface comparison (scenario-bench, context-ledger)

- [x] 5.1 Start each mode's MCP wiring (direct = the full fixture server set; ozy = `ozy index` then `ozy mcp` over the downstream config) and enumerate the tools and schemas advertised at startup, tokenized with the reused `internal/eval.TokenEstimator`.
- [x] 5.2 Emit startup `tool_schema` ledger items (`estimated`) and per-mode `metrics.json` (startup tools visible, schema bytes/tokens, irrelevant schema tokens).
- [x] 5.3 Emit `comparison.json`/`comparison.md` with the startup surface section (tools visible, schema tokens, reduction ratio, deltas); mark live metrics `skipped` when no endpoint is configured.
- [x] 5.4 Test: ozy startup tools-visible reflects only the broker interface vs the full direct surface; the reduction ratio is computed; the surface comparison runs with no `MODEL_BASE_URL`.

## 6. Context ledger schema and deterministic grading (context-ledger)

- [x] 6.1 Define the JSONL ledger item type (`run_id`, `mode`, `phase`, `source`, `kind`, `server?`, `tool?`, `bytes`, token count, `token_source`, `included_in_model_context`) and the writer; record the estimator name in the run.
- [x] 6.2 Implement the deterministic grader scoring the final answer + tool-call log against `ground_truth.json` (file, function, culprit commit by subject or recorded hash, patch target, regression test) plus the forbidden checks (distractor/web-class tool from the tool-call log, broad-refactor heuristic, no public internet); emit `grading.json`.
- [x] 6.3 Test: a correct, clean answer passes; a run that used a forbidden/distractor tool fails the forbidden check; the culprit commit matches by subject and by hash.

## 7. Docker Compose, ContextSpy, and agent configs

- [x] 7.1 Author `bench/configs/opencode.direct.jsonc` (all seven fixture servers), `bench/configs/opencode.ozy.jsonc` (only `ozy mcp`), and `bench/configs/ozy.downstream.jsonc` (the same seven as Ozy downstream).
- [x] 7.2 Add the `bench-runner` `Dockerfile` (ozy + ozy-bench + OpenCode + `sqlite3` + the fixture) and a minimal `docker-compose.yml` with `bench-runner` and `contextspy` (pinned `==0.2.0`) running `contextspy start-local`, the model reached on the host via `host.docker.internal`; drive config from a `.env` (`MODEL_*`, `SCENARIO`, `MODE`, `BENCH_RUNS`), **bind-mount `./bench/runs` to the host**, and support both `docker compose up` (streams logs) and `docker compose run bench-runner --scenario <name> --mode direct|ozy|both`.
- [x] 7.3 Generate `~/.contextspy/config.toml` with a `[[reverse_targets]]` entry (`target_url` from `MODEL_BASE_URL`, `provider = "openai"`, a `listen_port`) and point OpenCode's `MODEL_BASE_URL` at `http://contextspy:<listen_port>/v1` (reverse proxy, no CA cert); pin the OpenCode version and confirm its non-interactive run flag and transcript/tool-event shape.

## 8. Live agent tier and full comparison (scenario-bench, context-ledger)

- [x] 8.1 Implement the `Runner` adapter: build the per-mode config, launch OpenCode non-interactively with the task prompt, and capture `transcript.jsonl`, `tool-calls.jsonl`, and `final-answer.md`.
- [x] 8.2 Populate the ledger from ContextSpy's SQLite capture (`measured`) with the estimator fallback (`estimated`), and compute the live metrics: total input/output tokens, tokens before first useful evidence, tokens to success, largest context item, largest tool result, useful/irrelevant/failed calls, and Ozy discovery/describe overhead.
- [x] 8.3 Run grading on the live final answer and assemble the full `comparison.json`/`comparison.md` (surface + tool behavior + token economy + per-run values + aggregates + success rate per mode + deltas).
- [x] 8.4 Implement the N-run loop with per-run isolation: a fresh ephemeral agent session and Ozy state dir (re-`ozy index`) for each run, the fixture mounted read-only, in-process distractor state; compute `aggregate.json` per mode (mean/min/max/stdev for numeric metrics + success rate `k/N`); surface tier computed once.
- [x] 8.5 Bound each run with `timeout_seconds` (kill → record `timed_out` → continue the batch), fail fast on an unreachable endpoint, and stream per-run progress + a periodic heartbeat to stdout.
- [x] 8.6 End-to-end: `docker compose up` with `BENCH_RUNS=5` against a local OpenAI-compatible endpoint produces a complete run directory on the host; document the headline-metric shape in `bench/README.md` without hardcoding numbers.

## 9. Docs and integration

- [x] 9.1 Write `bench/README.md`: the hands-off flow (edit `.env`, `docker compose up`, watch logs, read the host-mounted report), the env vars incl. `BENCH_RUNS`, the three modes, the per-run + aggregate artifact layout, and the claim the benchmark proves — noting the static surface tier needs no model.
- [x] 9.2 Ensure `go test ./...` stays green with the live tier gated/opt-in (not run by default), and that `make bench` builds the binary.
