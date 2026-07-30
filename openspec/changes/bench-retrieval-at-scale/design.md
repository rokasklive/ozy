## Context

Builds directly on the unarchived `hermetic-e2e-bench` change: one-command Docker bench, pinned OpenCode driving free built-in models, agent-reported token usage, decision-grade reports (`surface.json` → `metrics.json` → `aggregate.json` → `comparison.{json,md}`), and runtime hermeticity (model gateway is the only egress, npm pinned unroutable). What that change did not do: the estate is 19 tools across 7 fixture servers, so ozy's retrieval never faces realistic pressure; the live-tier table renders per-mode columns with no delta column (only the surface table computes deltas), and a mode that wasn't run shows bare "—"; the direct-mode server set, the surface enumeration, and the grader's checks are all hardcoded to the one code-archaeology scenario (`checkNoWebTools` would fail any scenario that legitimately searches the web).

Constraints carried forward unchanged: live runs happen in Docker, never on the host; runtime network access is the model gateway only; the deterministic eval suite (`internal/eval`, `evals/`) and all product code stay untouched; both modes must face byte-identical environments; grading is ground-truth string/artifact checks, never an LLM judge.

Reference surfaces for the mirrored servers (captured 2026-07-15): [AryanBV/pdf-toolkit-mcp](https://github.com/AryanBV/pdf-toolkit-mcp) — 22 tools (`pdf_create`, `pdf_create_from_markdown`, `pdf_extract_text`, `pdf_merge`, …); [weather-mcp](https://github.com/weather-mcp/weather-mcp) — 17 tools (`get_historical_weather` back to 1940 via Open-Meteo, `search_location`, `get_forecast`, …); [agenticmarket/duckduckgo](https://agenticmarket.dev/agenticmarket/duckduckgo) — structured search results (title, URL, snippet); [agenticmarket/wikipedia](https://agenticmarket.dev/agenticmarket/wikipedia) — article search and full-content retrieval. The agenticmarket servers are hosted behind a registry and their exact schemas are not publicly documented — see Open Questions.

## Goals / Non-Goals

**Goals:**

- Both modes face an identical ≥500-tool estate; ozy indexes all of it, direct mode advertises all of it. Retrieval reliability becomes a measured quantity: canonical-tool hits and distractor-call counts per run, aggregated and compared.
- A second scenario, `historical-weather-report`, exercises a 4-server chain (weather → search → wikipedia → PDF) with a real, verifiable PDF file as the deliverable — repeatable by construction (fixed city, fixed past date, baked data).
- `comparison.md` gains live-tier deltas with the same clarity as the surface table, and never renders an unexplained "—".
- Every hermeticity, one-command, and exit-code property from `hermetic-e2e-bench` holds unchanged.

**Non-Goals:**

- No live internet MCPs and no vendored Node/TypeScript servers in the image — the four real servers are mirrored as Go fixture toolsets (surfaces identical, task-critical behavior functional, data baked).
- Not porting the real servers' full behavior: only the tools the scenario needs are functional; mirrored siblings behave as corpus-grade stubs.
- No multi-scenario matrix per invocation, no LLM-judge grading, no threshold/regression engine — all unchanged non-goals.
- No product-code changes to make the bench pass: if direct mode cannot operate at 500 tools, or the semantic sidecar cannot be baked hermetically, the bench *records* that instead of engineering around it.

## Decisions

### D1 — Real MCPs enter the bench as mirrored functional fixtures, not live upstreams

The four named servers are reimplemented inside the existing `ozy-bench mcp` binary with tool names, descriptions, and input schemas pinned from the real servers (mirror source and capture date recorded in the corpus data). Task-critical tools are *functional*: historical weather answered from a baked dataset, search ranked over a baked result corpus, wikipedia served from baked articles, and PDF tools that genuinely write PDF files to disk. *Alternatives considered:* running the real npm servers (weather/DDG/wikipedia need live APIs at runtime — breaks the hermetic-runtime requirement and repeatability; DDG results change daily); record/replay HTTP proxying (TLS trust and capture complexity for strictly less determinism than baking the data). Baking is also what the task itself demands — the user requirement "a specific date, so it's repeatable" is only satisfiable with frozen data.

### D2 — The 500-tool corpus is checked-in data served by one generic stub implementation

- **Data:** `bench/corpus/<server>.json`, one file per fixture server (~25–30 files): server name, mirror provenance (real server or "synthesized"), and tools `[{name, description, inputSchema, cannedResponse?}]`. Total ≥500 tools.
- **Serving:** `ozy-bench mcp --server <name> --corpus-dir <dir>` loads the file and serves it; any stub call returns a deterministic, schema-plausible canned response (per-tool override or a generated default) and never errors on valid input.
- **Realism:** where a real public MCP server's surface is documented (github, slack, jira, gdrive, stripe, kubernetes, datadog, notion, linear, sentry, postgres, …), mirror it; otherwise synthesize in the same style. Descriptions are 1–3 sentences of production tone; every schema property is typed and described.
- **Distractor families are mandatory**, aligned against the scenario's canonical tools: ≥3 rival search tools (brave/bing/google-style), ≥3 weather/climate lookalikes (climate analytics, aviation weather, air-quality-only), ≥3 document/PDF rivals (converters, e-sign, office-suite export). Retrieval must pick *the* tool, not the neighborhood.
- **Enforcement:** a unit test loads the corpus and fails below the floor (≥500 tools, ≥25 servers, unique per-server tool names, schema validity, description length bounds, distractor families present). Authoring happens once at implementation time (LLM-assisted drafting reviewed by hand is fine — it is authoring-time work, never runtime); quality is anchored by the mirrored-from-real-servers subset.
- *Alternative considered:* generating tools programmatically at runtime from templates — rejected: non-reviewable realism, and the corpus must be stable data for run-to-run comparability.

### D3 — Scenarios declare their toolsets; the corpus attaches by default

`scenario.jsonc` gains `toolsets` (the functional fixture servers this scenario needs) and `corpus: true|false` (default `true`). `mcpServers(mode)`, `setupOzy`, and `ComputeSurfaceComparison` derive the server set from the scenario instead of the hardcoded seven. The old scenario declares its existing 7 toolsets and keeps `corpus: true` — the ≥500 estate is the default environment for *every* scenario, per the identical-environment contract. Mode remains a named wiring transform (direct = flat exposure of the scenario's server set; ozy = broker only, same set as downstream), so a mode variant is still a data/template change, now parameterized by the scenario's server set. The vestigial checked-in `opencode.<mode>.jsonc` templates are regenerated from the same source so they never drift from what the runner actually writes.

### D4 — Server-side invocation logging is the ground truth for tool selection

Every fixture server (functional and stub) appends `{server, tool, argsDigest, ts}` as one JSONL line to the file named by `OZY_BENCH_CALL_LOG` (O_APPEND, one line per call). The per-run env injects a per-run path in both modes: direct mode via the generated opencode config's server `environment`, ozy mode via the downstream config `setupOzy` writes. *Why:* in ozy mode the OpenCode transcript shows only `find_tool`/`describe_tool`/`call_tool` — the downstream tool identity lives inside `call_tool` arguments, which the transcript parser does not (and should not fragilely) reconstruct. A server-side log is mode-symmetric, format-drift-proof, and also gives distractor-call counting for free. *Alternative:* parsing tool arguments out of OpenCode's JSON events — rejected: pinned-version-specific, breaks silently on format drift, and asymmetric between modes.

### D5 — Grading becomes declarative; the check engine is generic

`ground_truth.json` v2: `answer_must_contain[]`, `commit_check{subject,hashFrom}` (old culprit check, now optional data), `required_tools[{server,tool}]` (matched against the invocation log), `forbidden_tool_patterns[]`, `artifacts[{path, type, must_contain[]}]`. The grader is a small generic engine over this data; no scenario-specific names remain in Go. The old scenario's five hardcoded criteria and three forbidden checks map 1:1 into its ground-truth file (including today's web-tool ban — which correctly becomes *scenario data*, since the new scenario legitimately uses a search engine). **BREAKING** only for the unmerged ground-truth shape; the acme-billing scenario's behavior is unchanged.

### D6 — PDF support is a minimal pure-Go writer/reader, not a dependency

`pdf_create`, `pdf_create_from_markdown`, `pdf_extract_text`, `pdf_get_metadata`, and `pdf_search` are functional; the other 17 mirrored pdf-toolkit tools are corpus-grade stubs. The writer emits PDF 1.4 with uncompressed content streams and base-14 Helvetica (markdown input is rendered as headings/paragraphs/lists — rich tables and images are out of scope); the reader extracts text from exactly that class of PDF, which is also what the grader uses for artifact checks. Relative `outputPath` arguments resolve under `OZY_BENCH_OUTPUT_DIR` (the per-run agent workspace), so the grader finds the deliverable at a known per-run path. Baked content is kept WinAnsi-safe (transliterate at write time) so Lithuanian diacritics can't corrupt encoding, and ground-truth `must_contain` strings are chosen ASCII-safe. *Alternative:* a Go PDF library — rejected: new dependency for what ~250 lines of stdlib covers, and the bench must also *read* the PDF back for grading, which libraries make heavier, not lighter.

### D7 — Scenario content: Vilnius, Lithuania, 2024-07-15

The task: find the historical weather in Vilnius on 2024-07-15, research the city using a privacy-focused search engine and Wikipedia, and produce `output/report.pdf` containing the weather numbers and referenced city facts. The prompt deliberately says "a privacy-focused search engine" without naming DuckDuckGo — resolving that phrasing to the right tool among rival search stubs *is* the retrieval test. The weather dataset bakes the real Open-Meteo archive values for that city/date (captured at authoring time); the search corpus (~15–20 entries) and two condensed wikipedia articles (Vilnius, Lithuania) contain the ground-truth facts; canonical facts appear *only* in the canonical servers' data, so a wrong-tool path cannot accidentally satisfy ground truth. Ground truth requires the four canonical servers in `required_tools` and the PDF artifact with `must_contain` facts.

### D8 — Direct-mode viability at 500 tools is a measurement, never a workaround

OpenAI-compatible gateways commonly cap tool definitions (historically 128) and free models have finite context; direct mode at 500+ tools may be rejected or overflow. The run then records a *named* failure reason (`toolset_rejected`, `context_overflow` — classified from the transcript's error output — alongside the existing `timed_out`), aggregates count reasons, and the comparison states the finding plainly ("direct mode could not operate at this estate size on this stack"). The harness SHALL NOT shrink the estate for one mode — identical environments are the bench's core contract, and "direct cannot run at realistic scale" is itself the product claim, measured. A probe task runs first during implementation so corpus authoring isn't finalized against an unrunnable stack; the scenario's `timeoutSeconds` rises (direct startup and per-turn costs grow with 500 schemas).

### D9 — The retrieval stack ozy uses is verified, baked, and recorded

A retrieval benchmark that silently exercises the wrong retrieval stack is worthless. Implementation verifies which stack `ozy index`/`find_tool` actually use inside the hermetic image (the product ships hybrid semantic+lexical with an auto-provisioned sidecar — provisioning must happen at image build time to stay hermetic, like every other fetch). If the sidecar genuinely cannot be baked, the bench pins lexical-only and says so. Either way `environment.json` gains `retrieval: semantic|lexical` and the comparison prints it. Related: per-run `ozy index` over 500 tools may be slow with embeddings; if so, index once per invocation and copy the catalog file per run — the fixture is immutable, so the index is deterministic and this preserves run isolation (same argument task 5.4 already established).

### D10 — Live-tier delta parity in the comparison

The live table gains a Delta column (ozy − direct) for every metric row, mirroring the surface table; success is compared in percentage points, counts and means as signed differences. When one mode has no aggregate the column header reads "not run (MODE=…)" and the verdict states "live delta unavailable: <mode> was not run" instead of leaving bare "—". New retrieval rows: canonical-tool hit k/N, distractor calls/run, plus failure-reason counts from D8. `metrics.json`/`aggregate.json` carry the underlying fields.

## Risks / Trade-offs

- [Gateway/model rejects 500 tool definitions, so direct live metrics don't exist at scale] → named failure reasons + explicit comparison finding (D8); the acme-billing scenario can still be run with `corpus: false` for a small-estate live delta if ever needed — but the shipped default stays `corpus: true`, identical for both modes.
- [30–35k tokens of schemas per direct-mode request slows runs and burns free-tier rate limits] → per-scenario `timeoutSeconds` raised; duration/stdev already reported per mode; runs stay informational, never CI gates.
- [Corpus authoring quality drifts (bland or repetitive descriptions weaken retrieval realism)] → structural floor enforced by tests; realism anchored by mirroring documented real servers; content review is part of the task list.
- [Canned DDG search is brittle against arbitrary agent queries] → token-overlap ranking over the baked corpus with a deterministic, never-empty fallback result order; phrasing-robust by construction.
- [Semantic sidecar can't be baked hermetically] → pin lexical, record `retrieval: lexical` in provenance and comparison; the claim is then explicitly tested on lexical retrieval only.
- [Lithuanian diacritics break the WinAnsi-only PDF writer] → transliteration at write time + ASCII-safe ground-truth strings (D6).
- [OpenCode startup cost with ~30 stdio MCP servers] → each server is the same small Go binary; spawn cost is milliseconds; measured in duration either way.
- [Old runs incomparable with new runs] → accepted and intended; provenance records estate size and retrieval stack, so mixed comparisons are detectable.

## Migration Plan

Additive under `bench/` + `internal/bench/`; no product code changes, so rollback is reverting the change. The acme-billing scenario migrates to declarative ground truth and scenario-declared toolsets in this change (behavior unchanged, verified by re-running it). Lands after `hermetic-e2e-bench`; its spec deltas apply on top of that change's specs. Old run directories remain readable; new provenance fields distinguish new-estate runs.

## Open Questions

- **Exact agenticmarket duckduckgo/wikipedia schemas** — hosted behind the registry, not publicly documented. Pin at authoring time by installing/inspecting each once (outside the bench); if not inspectable without a paid account, mirror the canonical open-source equivalents instead (nickclyde/duckduckgo-mcp-server `search`/`fetch_content`; Rudra-ravi/wikipedia-mcp) and record the actual mirror source per server in the corpus data.
- **The real tool-count ceiling** of the pinned OpenCode + free-model gateways — probed first during implementation; both outcomes have a designed path (D8). RESOLVED (2026-07-15): the `opencode/deepseek-v4-flash-free` gateway **accepts 556 tool definitions** (~40k schema tokens, ~62k measured input tokens/run) with no `toolset_rejected` and no `context_overflow` — direct mode is *operable* at this estate size. In the live weather smoke (`MODE=both`, 2 runs) direct graded 1/2: the one failure was a tool-*selection* miss (the model reached for OpenCode's built-in `websearch` instead of the `duckduckgo` fixture, so the required `duckduckgo/search` call never happened), not an infrastructure rejection. So D8's findings present as a **retrieval-quality** gap (canonical-tool hits, wasted selection) rather than a hard `toolset_rejected`/`context_overflow` on this stack; the named failure-reason classifiers stay in place for stacks/models that do cap.
- **Sidecar bake feasibility** inside the bench image (D9) — RESOLVED (2026-07-15): the semantic sidecar needs a Python + FastEmbed venv provisioned via uv/python3; the runtime image is node:alpine (musl) with no Python toolchain, and provisioning would need a runtime fetch the hermetic contract forbids. `ozy index` falls back to lexical-only (verified: index summary reports no vector count; live findTool ranks by lexical term overlap). Baking Python + onnxruntime on musl + an embedding model was declined (build fragility + image bloat). Pinned `OZY_BENCH_RETRIEVAL=lexical` in the Dockerfile and recorded in provenance — the bench measures ozy's lexical retrieval at scale.
- **Per-run index cost at 500 tools** — measured; falls back to index-once-copy-per-run (D9) if material. RESOLVED (2026-07-15): per-run `ozy index` + `ozy mcp` broker-ready over the 523-tool estate takes **2.7s** (lexical, from `logs/ozy.log` in run `20260715-192101`) — immaterial vs the ~75s/run total, so per-run indexing is kept as-is (no index-once-copy switch needed).
