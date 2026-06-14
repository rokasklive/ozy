## 1. Scaffold: package, `evals/` tree, and dataset schema

- [x] 1.1 Create the `internal/eval` Go package and the top-level `evals/` tree (`data/{catalog,discovery,invocation,ergonomics}`, `snapshots/`, plus placeholder `README.md`, `BENCHMARKS.md`, `METHODOLOGY.md`) per the design layout; add `evals/snapshots/*` except `baseline.json` to `.gitignore`.
- [x] 1.2 Define and document the versioned dataset schema (Go types + a short schema description in `evals/README.md`) for catalog entries, discovery labels (with `category` and `rationale`), invocation scenarios (with `expectedOutcome`/`expectedError`), and ergonomics cases, so a contributor can add data without reading harness code.

## 2. Test data: the synthetic downstream MCP catalog ("the world")

- [x] 2.1 Author `evals/data/catalog/world.json` — a realistic synthetic catalog of multiple servers and tools (e.g. Atlassian/Confluence, Jira, GitHub, Slack, Gmail, calendar, filesystem, code search, database) with real-looking toolRefs, titles, descriptions, and JSON input schemas.
- [x] 2.2 Ensure the catalog includes near-duplicate capabilities across servers (e.g. a "search" tool on more than one server) so wrong-server rate is measurable, and verify it loads into `catalog.NewMemory()` and indexes on both legs.

## 3. Test data: discovery gold sets

- [x] 3.1 Author discovery gold sets under `evals/data/discovery/` mapping intents → acceptable target tool(s)/server(s), tagged by category: lexical-overlap, semantic-paraphrase, no-match, ambiguous, and wrong-server trap; each label carries a one-line rationale.
- [x] 3.2 Seed the discovery set from the existing inline `TestDiscoveryEval_*` cases (`internal/cli/cli_test.go`), expressed as data, and add semantic-paraphrase intents that the lexical-only baseline does not already win by term overlap.
- [x] 3.3 Add a hygiene check (run by the loader) that flags any `semantic-paraphrase` intent the lexical baseline trivially wins, so "semantic" labels can't silently become lexical freebies.

## 4. Test data: invocation, ergonomics, and thresholds

- [x] 4.1 Author invocation scenarios under `evals/data/invocation/`: valid-argument success cases, invalid-argument cases paired with the expected corrected call, schema-drift fixtures (`expectedError: TOOL_SCHEMA_CHANGED`), and offline-server fixtures (`expectedError: DOWNSTREAM_SERVER_OFFLINE`).
- [x] 4.2 Author ergonomics cases under `evals/data/ergonomics/` exercising instructional-quality and §13 budget expectations across surfaces, plus the inputs used for CLI↔MCP parity.
- [x] 4.3 Author `evals/data/thresholds.json` — the gate thresholds for the tracked headline metrics, as ratchetable data.

## 5. Harness: loader and validation

- [x] 5.1 Implement the dataset loader in `internal/eval` that reads the corpus, validates every file against the schema, and aborts with a precise `file:field` error on any malformed entry or dangling toolRef reference.
- [x] 5.2 Add loader tests: a well-formed corpus loads; a missing required field and a dangling toolRef each fail fast with the offending path; the hygiene check from 3.3 fires on a planted leaky intent.

## 6. Harness: discovery metrics (lexical baseline)

- [x] 6.1 Implement the discovery runner driving `broker.FindTool` and `search.Engine`/`Decide`, computing top-1, top-3, MRR, wrong-server rate, and no-match correctness per gold set, with results keyed by category.
- [x] 6.2 Assert determinism (two runs over the same corpus/config produce identical metrics) and that `no_good_match`/`catalog_empty` count as correct refusals for no-match intents; remove the superseded inline `TestDiscoveryEval_*` tests.

## 7. Harness: semantic + hybrid leg with the real model (gated)

- [x] 7.1 Implement the semantic eval path: provision the real sidecar into a temp XDG state dir, embed the corpus and queries with the real FastEmbed model, and compute hybrid (RRF) discovery metrics alongside the lexical baseline.
- [x] 7.2 Gate the semantic leg behind `OZY_EVAL_SEMANTIC=1`; when off or the sidecar is unavailable, record the semantic-only metrics as `skipped: semantic unavailable` and keep the rest of the run green (never substitute the fake provider for committed numbers).
- [x] 7.3 Add a test proving a semantic-paraphrase intent changes the winner versus the lexical baseline when the real leg runs, and that the leg skips cleanly when the flag is unset.

## 8. Harness: invocation and repair metrics

- [x] 8.1 Implement the invocation runner over fixture downstream MCP servers (reuse the in-process fake server from the acceptance tests), computing valid-argument rate, first-call success, repair success, schema-error rate, and structural error clarity.
- [x] 8.2 Implement the repair loop: an invalid call returns a structured error, the harness issues the corrected call from the error's repair guidance, and a success counts as a repair; cover offline and schema-drift fixtures asserting the expected error type and a non-amplifying retry disposition.

## 9. Harness: agent ergonomics, parity, and token economy

- [x] 9.1 Implement the ergonomics conformance checks (organized by the agent-ergonomics domains): every `findTool` result is an explicit decision with a grounded/conditional/actionable next step, every error has a type + retry disposition, and responses stay within §13 budgets; flag query-restating non-instructional responses.
- [x] 9.2 Implement CLI↔MCP parity: run the same `findTool`/`describeTool`/`callTool` inputs through the CLI `--format json` path and the in-process `mcp.Adapter` and assert semantically equivalent decisions, selected toolRefs, and error types.
- [x] 9.3 Implement the token-economy measurement behind a swappable estimator interface (default documented heuristic): startup schema tokens, tokens-to-success, largest payload, and broker-call count, reported for the direct-MCP baseline (all downstream schemas) vs the Ozy path.

## 10. Performance benchmarks

- [x] 10.1 Add `testing.B` benchmarks for the lexical ranker, RRF fusion, and end-to-end `broker.FindTool` over the corpus (`go test -bench`).
- [x] 10.2 Add harness-driven latency-distribution measurement (p50/p95 + throughput) for the lexical, semantic (gated), fusion, and end-to-end paths, reported into the run result.

## 11. Reporting: snapshot, scoreboard, methodology, and gate

- [x] 11.1 Implement the structured run result + JSON snapshot writer (metrics + provenance: corpus version, model, git commit, timestamp, semantic-ran flag) in the stable documented shape; write timestamped snapshots and update `evals/snapshots/baseline.json`.
- [x] 11.2 Implement the threshold gate: evaluate the run against `thresholds.json`, set an overall pass/fail verdict, and reflect failure in the process exit status.
- [x] 11.3 Implement the `BENCHMARKS.md` generator (per-family headline tables + thresholds, regenerated from a snapshot so prose can't drift) and write `METHODOLOGY.md` with the metric formulas, RRF `k`/floor calibration procedure, token-estimator note, gold-set hygiene rules, and the human/review-board rubric scaffold for judged metrics.

## 12. CLI: wire `ozy eval`

- [x] 12.1 Replace the `NOT_IMPLEMENTED` stub in `internal/cli/commands.go` with `ozy eval run [scenario]` (optionally scoped to a family) and `ozy eval report`, routed through `internal/eval` and the shared broker seam, honoring `--format` and exiting non-zero on gate failure.
- [x] 12.2 Update `internal/cli/cli_test.go`: `eval run` no longer returns `NOT_IMPLEMENTED`; `eval run --format json` emits a verdict-bearing result; `eval run discovery` scopes to one family; `eval report --format json` emits the latest snapshot.

## 13. Acceptance, docs, and validation

- [x] 13.1 Add an end-to-end acceptance test: `ozy eval run` over the committed corpus produces a snapshot and a verdict; run it once with the semantic leg gated off (fast path) and document running it with `OZY_EVAL_SEMANTIC=1` for the real-model numbers.
- [x] 13.2 Write `evals/README.md` (how to run the suite, how to add catalog/gold/scenario data, how the gate works) and generate the initial committed `BENCHMARKS.md` + `baseline.json` from a real run; reference the suite from `README.md`/`SPEC.md` §14 where appropriate.
- [x] 13.3 Run `go test ./...`, `gofmt`, `go vet`/`golangci-lint run`, `openspec validate comprehensive-eval-suite --type change --strict`, and `graphify update .`.
