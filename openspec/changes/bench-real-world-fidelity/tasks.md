## 1. Outcome-based grading (D1)

- [x] 1.1 In `internal/bench/grader.go` `Grade()`, compute `Overall` from **result** criteria only (`answer_must_contain`, `commit_check`, artifact `must_contain`, `forbidden_answer_patterns`); move `required_tools` + `forbidden_tool_patterns` results into an `informational` section of `GradingResult` that is recorded but never flips `Overall`
- [x] 1.2 Update `GradingResult` shape (separate `criteria` gating vs `informational` tool checks) and `WriteGradingResult`; keep `canonical_tools_hit` / `distractor_call_count` in `metrics.json` unchanged (already informational)
- [x] 1.3 Update `grader_test.go`: a run that satisfies result criteria but calls a non-canonical / "forbidden" tool now PASSES; a run missing an answer/artifact fact FAILS

## 2. Efficiency-first reporting (D2)

- [x] 2.1 In `internal/bench/report.go`, reorder the `comparison.md`/verdict so it leads with startup schema tokens → total tokens/run → tool-call count → duration → success k/N; move canonical-hit / distractor-call to an informational footer (no metric removed)
- [x] 2.2 Update `report_test.go` for the new verdict ordering; confirm the ozy − direct-lean primary delta and per-mode columns still render

## 3. De-specify the scenario task (D5)

- [x] 3.1 Revert `bench/scenarios/historical-weather-report/task.md` step 2 to a capability-level ask ("research via a privacy-focused search engine") — undo the DuckDuckGo naming from `bench-enable-semantic`
- [x] 3.2 In `expected/ground_truth.json`, remove the `brave-search`/`kagi-search`/`bing-search` `forbidden_tool_patterns`; keep `required_tools` present but now informational (grading no longer gates on them)

## 4. Real-world tool behavior — three tiers (D3)

- [x] 4.1 Add a data-driven `behavior` field to `CorpusTool` (`internal/bench/corpus.go`): `functional:<backend>` | `auth_error` | `stub` (default `stub`); validate it
- [x] 4.2 In the corpus server mode (`internal/bench/mcp.go`), route by behavior: `auth_error` → a realistic `authentication required / missing API key` error; `functional:search` → the shared search ranker over the fixture corpus; `stub` → unchanged generic response. Wire `--fixture-dir` into corpus servers that host a functional tool
- [x] 4.3 Refactor the search ranker (and PDF writer if needed) out of `toolsets.go` into a shared handler callable from both the functional toolset and the corpus `functional` backend
- [x] 4.4 Mark the same-capability rivals in `bench/corpus/*.json`: `brave-search`/`kagi-search`/`bing-search` web-search tools → `auth_error`; any no-auth same-capability search rival → `functional:search`. Audit weather (`climate-analytics`, `aviation-weather`, `air-quality-index`) and PDF (`doc-converter`, `office-export`) families and set each rival's behavior to match the real service (auth vs no-auth)
- [x] 4.5 Update corpus tests (`corpus_test.go`) and the surface-count expectations (556 tools unchanged; behavior added is data)

## 5. Record-and-replay capture (D4)

- [x] 5.1 Add an opt-in capture step to fixture generation (`internal/bench/fixture*.go`): with network allowed at generation time, invoke the real no-auth MCP for a scenario's capabilities, capture tool schemas + representative responses, and bake them into the fixture/corpus (pinned, cached in-repo)
- [x] 5.2 Ensure runtime replay reads only the baked capture (no network); assert byte-stable replay
- [x] 5.3 Document capture usage (how to (re)capture, that it is generation-time only) in `bench/README.md` / `bench/corpus/README.md`

## 6. Verify + document

- [x] 6.1 `gofmt`, `go vet`, `go build ./...`, `go test ./internal/bench/...` — all green
- [x] 6.2 Rebuild the bench image and run a live E2E: confirm success is outcome-based (a run that picks a different search engine but produces the right answer/PDF passes), an auth-gated pick returns a legible auth error and the agent routes around it, and `comparison.md` leads with efficiency metrics
- [x] 6.3 Update `bench/README.md`: outcome-based grading, three-tier tool behavior, capture pipeline; note the grading change is BREAKING vs pre-change pass/fail numbers
