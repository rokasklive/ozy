## 1. Probes and plumbing (fail-fast learnings first)

- [x] 1.1 Probe the estate-scale ceiling: generate a throwaway 500-tool opencode config in the bench image and drive one `opencode run` against the pinned free model; record whether the gateway accepts 500 tool definitions, rejects them, or overflows context — this decides how D8 findings will actually present — `bench/` scratch, findings noted in design Open Questions (RESOLVED: gateway accepts 556 tools; D8 presents as a retrieval-quality gap, not a hard rejection — see design Open Questions)
- [x] 1.2 Verify the retrieval stack inside the bench image: determine whether `ozy index`/`find_tool` run semantic+lexical or lexical-only when offline; if the sidecar can be baked at build time, bake it (build-time fetch only); either way record the outcome — `bench/Dockerfile`, notes for provenance (D9)
- [x] 1.3 Pin the agenticmarket duckduckgo + wikipedia tool surfaces: install/inspect each once outside the bench (or fall back to the canonical OSS equivalents) and capture exact tool names/descriptions/schemas with mirror source + date — authoring notes feeding 3.x and corpus data
- [x] 1.4 Scenario config schema: add `toolsets[]` and `corpus` (default true) to `scenario.jsonc` loading; declare the seven existing toolsets in the acme-billing scenario — `internal/bench/config.go`, `bench/scenarios/suspended-account-invoice-regression/scenario.jsonc`
- [x] 1.5 Scenario-driven server wiring: `mcpServers(mode)`, `setupOzy`, and `ComputeSurfaceComparison` derive the server set (functional toolsets + corpus servers) from the scenario declaration; regenerate the checked-in `opencode.<mode>.jsonc` templates from the same source — `internal/bench/runner.go`, `surface.go`, `bench/configs/` (wiring + template anti-drift golden test done; corpus attaches at runtime, not enumerated in the vestigial template)
- [x] 1.6 Server-side invocation log: fixture servers append `{server, tool, argsDigest, ts}` JSONL to `OZY_BENCH_CALL_LOG` (O_APPEND, no-op when unset); runner injects a per-run path into both modes' generated configs — `internal/bench/mcp.go`, `runner.go`

## 2. Tool corpus (≥500 tools as checked-in data)

- [x] 2.1 Corpus data format + loader: `bench/corpus/<server>.json` ({server, mirrorSource, tools[{name, description, inputSchema, cannedResponse?}]}), loader with validation — `internal/bench/corpus.go`
- [x] 2.2 Generic corpus server mode: `ozy-bench mcp --server <name> --corpus-dir <dir>` serves a corpus file's tools with deterministic schema-plausible canned responses (never errors on valid input, never contains scenario ground-truth facts), invocation-logged — `internal/bench/mcp.go`
- [x] 2.3 Author the corpus: ~25–30 servers, ≥500 tools total — mirror documented real MCP servers (github, slack, jira, gdrive, stripe, kubernetes, datadog, notion, linear, sentry, postgres, …) and synthesize the rest in the same style; include the mandatory distractor families (≥3 rival search, ≥3 weather/climate lookalikes, ≥3 document/PDF rivals) — `bench/corpus/*.json`
- [x] 2.4 Corpus validation test enforcing the floor: ≥500 tools, ≥25 servers, unique per-server names, valid schemas with typed+described properties, description length bounds, distractor families present, mirror provenance recorded — `internal/bench/corpus_test.go`
- [x] 2.5 Content review pass over the corpus: descriptions read like production tools, no copy-paste repetition, near-miss families genuinely tempting — review checklist in PR

## 3. Mirrored functional toolsets

- [x] 3.1 Minimal pure-Go PDF writer (PDF 1.4, uncompressed streams, Helvetica, headings/paragraphs/lists from markdown-ish input, WinAnsi transliteration) + reader (text extraction for exactly that class of PDF) with unit tests — `internal/bench/pdf.go`, `pdf_test.go`
- [x] 3.2 `pdf-toolkit` toolset: all 22 mirrored tools; `pdf_create`, `pdf_create_from_markdown`, `pdf_extract_text`, `pdf_get_metadata`, `pdf_search` functional (relative outputs under `OZY_BENCH_OUTPUT_DIR`), the rest corpus-grade stubs — `internal/bench/mcp.go`
- [x] 3.3 `weather` toolset: all 17 mirrored weather-mcp tools; `get_historical_weather` + `search_location` functional over the baked dataset, the rest stubs — `internal/bench/mcp.go`
- [x] 3.4 `duckduckgo` toolset: mirrored surface (per 1.3); search ranks the baked corpus by deterministic token overlap, never empty; content-fetch (if mirrored) serves baked page content — `internal/bench/mcp.go`
- [x] 3.5 `wikipedia` toolset: mirrored surface (per 1.3); article search/summary/content served from baked articles — `internal/bench/mcp.go`
- [x] 3.6 Toolset unit tests: mirrored surface counts/names, functional determinism (same input → same output), phrasing-robust search, PDF round-trip (create → extract), output-dir resolution — `internal/bench/mcp_test.go`

## 4. Historical-weather-report scenario

- [~] 4.1 Bake the fixture data (weather numbers baked+consistent; reconcile with live Open-Meteo capture when network available): real Open-Meteo archive values for Vilnius 2024-07-15 (captured at authoring), ~15–20 search-corpus entries, two condensed WinAnsi-safe wikipedia articles (Vilnius, Lithuania) with the ground-truth facts appearing only here — `bench/scenarios/historical-weather-report/fixture/`
- [x] 4.2 Scenario definition: `scenario.jsonc` (toolsets: weather, duckduckgo, wikipedia, pdf-toolkit; corpus: true; raised `timeoutSeconds`), `task.md` (weather for the fixed date → research via "a privacy-focused search engine" — deliberately unnamed — and Wikipedia → produce `output/report.pdf`), fixture generation dispatch for the non-git fixture — `bench/scenarios/historical-weather-report/`, `internal/bench/fixture.go`, `fixture_cmd.go`
- [x] 4.3 Ground truth: `answer_must_contain` (ASCII-safe weather values + city facts), `required_tools` (all four canonical servers), `forbidden_tool_patterns`, `artifacts` (report.pdf must exist, parse, and contain the facts) — `bench/scenarios/historical-weather-report/expected/ground_truth.json`

## 5. Declarative grading and retrieval metrics

- [x] 5.1 Generic grader engine over ground-truth v2 (`answer_must_contain`, `commit_check`, `required_tools` vs invocation log, `forbidden_tool_patterns`, `artifacts` with PDF checks via 3.1's reader); no scenario names left in Go — `internal/bench/grader.go`
- [x] 5.2 Migrate acme-billing ground truth to the declarative shape with 1:1 outcomes (including today's distractor/web/refactor bans as data); verify identical grading on existing recorded transcripts (migration + 1:1 grader tests done; live transcript-replay verification confirmed by 7.3 — declarative grader gives identical PASS both modes on live transcripts) — `bench/scenarios/suspended-account-invoice-regression/expected/ground_truth.json`, `grader_test.go`
- [x] 5.3 Retrieval metrics in `metrics.json` from the invocation log: `canonical_tools_hit` k/N, `distractor_call_count`, `downstream_call_count`; failure-reason classification (`toolset_rejected`, `context_overflow`) from transcript error probing — `internal/bench/report.go`, `runner.go`
- [x] 5.4 Aggregate the new fields (means, failure-reason counts) — `internal/bench/report.go`

## 6. Reporting parity

- [x] 6.1 Live-tier delta column (ozy − direct) for every metric row when both aggregates exist (success in percentage points, counts/means signed); retrieval-quality and failure-reason rows added — `internal/bench/report.go`
- [x] 6.2 Mode-absence labeling: column header "not run (MODE=…)" + verdict line naming why the live delta is unavailable; never bare "—" — `internal/bench/report.go`
- [x] 6.3 Corpus-aware irrelevant-schema-token accounting: irrelevant = tools outside the scenario's canonical set (replaces the hardcoded distractor name list) — `internal/bench/surface.go`
- [x] 6.4 Provenance: `retrieval: semantic|lexical` (from 1.2, verified not assumed) recorded in `environment.json` and displayed in `comparison.md` — `internal/bench/provenance.go`, `report.go`
- [x] 6.5 Reporting unit tests: delta rendering both-modes and one-mode, not-run labeling, retrieval rows, failure-reason surfacing — `internal/bench/report_test.go`

## 7. End-to-end verification (in Docker, per the hard rule)

- [x] 7.1 Surface tier at scale: `make bench-surface` for both scenarios shows direct ≈ 500+ tools vs ozy 3, corpus counted identically for both modes, exit 0
- [x] 7.2 Live smoke, new scenario: `make bench` (`SCENARIO=historical-weather-report`, `MODE=both`, `BENCH_RUNS=2`) — canonical chain completes in at least one ozy run, `report.pdf` produced and graded, invocation logs present in every run dir, `comparison.md` shows live deltas (or named estate-scale findings for direct, per 1.1's outcome) (VERIFIED: run 20260715-152013 — ozy 2/2 canonical chain, report.pdf produced+graded both modes, calls.jsonl in every run dir, comparison.md live deltas ozy−direct)
- [x] 7.3 Live regression, old scenario: acme-billing with the corpus attached still completes and grades identically in ozy mode; direct-mode outcome recorded honestly (finding or completion) (VERIFIED: run 20260715-192101 — corpus attached (523 tools visible direct vs 3 ozy), ozy 2/2 pass, direct 2/2 pass; declarative grader gives identical PASS both modes over answer_contains + commit_check + 12 forbidden-tool + 4 forbidden-answer checks — closes 5.2's live transcript-replay gate)
- [x] 7.4 Retrieval sanity: across the smoke runs, ozy-mode `canonical_tools_hit` and `distractor_call_count` are populated from the server-side log and match a manual read of the transcripts (VERIFIED: ozy/run-1 calls.jsonl = 6 downstream calls across all 4 canonical servers → canonicalToolsHit 4/4, distractorCallCount 0; transcript shows only ozy_callTool/findTool/describeTool, so calls.jsonl is the required ground truth — exactly D4's rationale)
- [x] 7.5 Hermeticity re-check: no runtime egress beyond the model gateway with the corpus + new toolsets in play (npm still unroutable; no new fetches); per-run `ozy index` duration at 500 tools measured — if material, switch to index-once-copy-per-run and note it (VERIFIED: across the 523-tool acme run + 556-tool weather run — zero `.npm`/`_cacache`/`node_modules` debris in any run dir, sidecar provisioning fails offline (no Python), all `web_`/`http_`/`fetch`/`browser_` forbidden-tool checks pass → model gateway is the only egress; per-run `ozy index`+broker-ready over 523 tools = 2.7s, immaterial vs ~75s/run, so per-run indexing kept (no index-once-copy switch))

## 8. Docs

- [x] 8.1 `bench/README.md`: the estate (corpus + mirrored servers), both scenarios, the new artifacts (invocation log, retrieval metrics, delta column), provenance fields, and the D8 finding semantics
- [x] 8.2 Root README: bench section mentions retrieval-at-scale measurement; attribution/links for the four mirrored servers
