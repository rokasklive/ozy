## Why

The bench proves ozy's surface claim only at toy scale: direct mode exposes 19 tools across 7 fixture servers, so the harness never exercises the actual product claim — reliably resolving the *right* tools from a realistic estate of hundreds, where retrieval can fail. Meanwhile the comparison under-reports the live tier: the live table has Direct and Ozy columns but no delta column (only the startup-surface table computes deltas), and when a mode wasn't run its cells render as bare "—" with no explanation — so in practice readers see token/duration numbers for ozy but no ozy-vs-direct delta.

## What Changes

- **500+ tool estate**: a checked-in corpus of realistic MCP server/tool definitions (~25–30 servers, ≥500 tools total, each with production-grade names, descriptions, and JSON schemas) served as deterministic stubs by the existing `ozy-bench mcp` binary. Both modes face the identical estate: direct mode gets the flat tool list; ozy indexes all of it. The corpus deliberately includes near-miss distractor families (rival search engines, weather/climate lookalikes, document/PDF converters) so retrieval must pick *the* right tool, not the right neighborhood.
- **Four functional fixture MCPs** mirroring real public servers' tool surfaces — [AryanBV/pdf-toolkit-mcp](https://github.com/AryanBV/pdf-toolkit-mcp) (22 tools), [agenticmarket/duckduckgo](https://agenticmarket.dev/agenticmarket/duckduckgo), [agenticmarket/wikipedia](https://agenticmarket.dev/agenticmarket/wikipedia), and [weather-mcp](https://github.com/weather-mcp/weather-mcp) (17 tools) — with the task-critical tools actually working: historical weather from a baked dataset, search over a baked result corpus, wikipedia articles from baked content, and real PDF generation to disk. Everything stays hermetic (no runtime egress beyond the model gateway); the PDF is genuinely produced and verifiable.
- **New scenario `historical-weather-report`**: the agent must find the historical weather for a fixed city and date (repeatable by construction), research the city with a privacy-focused search engine and Wikipedia, and compose a PDF report as the deliverable. Success requires chaining 4 canonical servers picked out of 500+ tools.
- **Declarative per-scenario grading**: ground truth gains required-tool, forbidden-tool-pattern, answer-content, and artifact (PDF exists + contains facts) checks; the current hardcoded checks (`checkNoWebTools` bans anything matching `web_`/`fetch` — it would fail the new scenario for using DuckDuckGo) move into the old scenario's ground-truth data. **BREAKING** for the unmerged `ground_truth.json` shape only; no shipped behavior changes.
- **Retrieval-quality measurement**: fixture servers write a server-side invocation log per run (the only mode-symmetric way to see which downstream tool actually ran — in ozy mode the OpenCode transcript shows only broker calls, not what `call_tool` invoked). Grading and metrics gain canonical-tool hit and distractor/wasted-call counts.
- **Live-tier delta parity in `comparison.md`**: the live table gains a Delta column (ozy − direct) for every metric, mirroring the surface table; a mode that wasn't run is labeled "not run (MODE=…)" instead of bare "—", and the verdict states why the live delta is unavailable.
- **Retrieval-stack provenance**: verify which retrieval stack bench-ozy actually exercises at 500 tools (semantic sidecar vs lexical fallback inside the hermetic image), bake it into the image at build time if needed, and record it in `environment.json` and the comparison — a retrieval benchmark that silently measures the wrong retrieval stack is worthless.

## Capabilities

### New Capabilities

- `bench-tool-corpus`: the realistic tool estate — size floor (≥500), realism requirements (names/descriptions/schemas), distractor families, deterministic stub behavior, identical-environment attachment to both modes, and validation tests that enforce the floor.

### Modified Capabilities

<!-- These four exist as spec deltas in the unarchived `hermetic-e2e-bench` change; this change extends them and must land after it. -->

- `scenario-bench`: scenarios declare their functional toolsets and corpus attachment in `scenario.jsonc` (the 7-toolset direct config is no longer hardcoded); per-run invocation-log wiring reaches every fixture server in both modes; the identical-environment contract explicitly covers estate scale (a stack that cannot run 500 tools in direct mode is recorded as a finding, never worked around by shrinking ozy's estate).
- `bench-mcp-fixtures`: four new mirrored toolsets (pdf-toolkit, duckduckgo, wikipedia, weather) with pinned real-world surfaces and functional task-critical tools; a generic corpus-stub server mode; server-side call logging.
- `bench-fixtures`: the `historical-weather-report` scenario fixture (baked weather dataset, search-result corpus, wikipedia articles, ground truth with a PDF artifact check); ground truth becomes declarative (required tools, forbidden patterns, answer facts, artifacts).
- `bench-reporting`: live-tier delta column and not-run labeling in `comparison.{json,md}`; retrieval-quality metrics (canonical-tool hit, distractor calls) in `metrics.json`/`aggregate.json`; corpus-aware irrelevant-schema-token accounting; retrieval-stack field in provenance.

## Impact

- **Code**: `internal/bench/` (mcp.go toolsets + corpus stub server, grader.go declarative grading, report.go delta column + retrieval metrics, surface.go scenario-driven enumeration, runner.go scenario-declared server wiring + call-log env, config.go scenario schema, small pure-Go PDF writer + reader for the pdf toolset and grader), `bench/corpus/` (new checked-in corpus data), `bench/scenarios/historical-weather-report/` (new), `bench/configs/` (regenerated from scenario config), `bench/README.md`.
- **No product code changes**: `internal/broker`, `internal/mcp`, the CLI, and the deterministic eval suite stay untouched. If the sidecar can't be baked hermetically, the bench records `retrieval: lexical` rather than changing the product.
- **Dependencies**: none new — PDF generation/extraction is a minimal pure-Go writer (uncompressed streams), not a library.
- **Depends on**: the unarchived `hermetic-e2e-bench` change (this builds directly on its runner, reporting, and hermeticity machinery and must land after it).
- **Data**: old run directories remain readable but are not comparable to new runs (different estate scale); provenance already records enough to tell them apart.
- **Risk surfaced, not hidden**: direct mode at 500 tools may hit model/gateway tool-count or context limits; that outcome is captured as a named failure reason and reported as a finding — it is itself the measurement.
