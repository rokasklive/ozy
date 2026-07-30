# Ozy Scenario Benchmark

A controlled, reproducible benchmark that compares direct-MCP vs Ozy-brokered
agent performance against a frozen incident scenario. It answers one question:
**in the same environment, does Ozy shrink the agent-facing tool/context surface
while preserving task success?**

## Quick Start

```bash
make bench        # 5 runs per mode (default)
make bench 10     # N runs per mode — just append the number
```

That's it — zero config on a clean checkout. `make bench` builds the image,
generates the fixture, runs all three modes (`direct`, `direct-lean`, `ozy`) on
semantic retrieval against the default free OpenCode model with a 900 s per-run
timeout, and writes the reports. The only knob you normally touch is the run
count, passed positionally. Artifacts land on the host at
`bench/runs/<timestamp>-<scenario>/`, and the headline is in `comparison.md`:

```bash
cat bench/runs/*/comparison.md
```

New to the output? **[docs/reading-results.md](docs/reading-results.md)** is the
glossary: what `direct` / `direct-lean` / `ozy` mean, every metric in
`comparison.md`, and how to draw a conclusion.

Everything else (model, scenario, mode, timeout) has a baked default and is an
optional override — copy `bench/.env.example` to `.env` and edit if you need to.

### Surface tier only (no Docker, no model)

```bash
make bench-surface
```

The static surface tier enumerates each mode's startup tool/schema surface
in-process — no model, no container. It is deterministic and runs in a second.

### Clean up

```bash
make bench-clean   # prune bench/runs/
```

## What It Measures

Each scenario runs in three modes over one identical fixture, prompt, model, and
environment — only the tool wiring differs:

- **`direct`** — the agent is wired to every scenario server directly: its
  functional fixture toolsets **plus the ≥500-tool corpus** (`bench/corpus/`).
  The worst-case, everything-installed baseline.
- **`direct-lean`** — the agent is wired to **only the functional toolset MCPs**
  the scenario needs — all of each server's tools (task-critical and sibling
  alike), but **none of the corpus**. This is the honest real-world baseline: a
  careful operator installs the MCPs the task needs, not 500 tools they don't.
  The **`ozy − direct-lean`** delta is the primary product comparison.
- **`ozy`** — the agent sees only Ozy's broker interface
  (`findTool`/`describeTool`/`callTool`); Ozy brokers the identical estate.

`--mode`/`MODE` accepts `direct`, `direct-lean`, `ozy`, `all` (the default —
all three in one pass), or `both` (back-compat alias for `direct` + `ozy`).

### The estate (retrieval at scale)

Both modes face the same ~500+ tool estate, so retrieval reliability is a
*measured* quantity, not an assumption. The corpus mirrors real MCP servers
(github, slack, jira, stripe, …) and includes deliberate near-miss distractor
families (rival search, weather/climate lookalikes, document/PDF converters) so
the agent must resolve *the* right tool out of hundreds. Under outcome grading,
each same-capability rival **behaves like the real service it mirrors** (a
data-driven `behavior` on each corpus tool):

- **Functional** (no-auth, same capability) — returns real data via a shared
  backend, so the pick genuinely completes the task (e.g. `doc-converter`'s
  `convert_html_to_pdf` and `office-export`'s `export_to_pdf` actually write the
  PDF; a no-auth search rival would rank the same baked corpus).
- **Auth-error** (auth-gated) — returns a realistic `authentication required`
  error, exactly as it would for an unprovisioned agent, so the pick fails legibly
  and the agent routes to a working tool (`brave-search`/`bing-search`/
  `kagi-search`, `climate-analytics`).
- **Stub** (unrelated capability) — a generic schema-plausible response carrying
  no scenario fact, so a genuinely-wrong-capability path cannot fake success.

Four functional toolsets mirror real public servers with their task-critical
tools working:

| Toolset | Mirrors | Functional tools |
|---------|---------|------------------|
| `weather` | [weather-mcp](https://github.com/weather-mcp/weather-mcp) (17) | `get_historical_weather`, `search_location` |
| `duckduckgo` | [nickclyde/duckduckgo-mcp-server](https://github.com/nickclyde/duckduckgo-mcp-server) (2) | `search`, `fetch_content` |
| `wikipedia` | [Rudra-ravi/wikipedia-mcp](https://github.com/Rudra-ravi/wikipedia-mcp) (11) | `search_wikipedia`, `get_article`, `get_summary` |
| `pdf-toolkit` | [AryanBV/pdf-toolkit-mcp](https://github.com/AryanBV/pdf-toolkit-mcp) (22) | `pdf_create`, `pdf_create_from_markdown`, `pdf_extract_text`, `pdf_get_metadata`, `pdf_search` |

Everything stays hermetic: weather comes from a baked dataset, search ranks a
baked corpus, wikipedia serves baked articles, and PDFs are genuinely written to
disk by a small pure-Go writer and read back for grading. See
[`corpus/README.md`](corpus/README.md).

Baked fixtures can be **captured from the real no-auth MCP servers** at
generation time (opt-in, network-allowed, outside the hermetic runtime):

```bash
# only if the scenario ships a fixture/capture.json (hand-authored fixtures
# need no capture) — contacts the real MCPs, bakes their schemas + representative
# responses into the fixture, and is cached in-repo:
ozy-bench capture --scenario historical-weather-report
```

Capture is **generation-time only** — a benchmark run never contacts a real MCP;
it replays the baked files byte-for-byte with the network restricted to the model
gateway. Re-capture is explicit (the baked data is pinned), so runs stay
deterministic.

ozy exercises its **semantic** (hybrid semantic+lexical) retrieval here, not a
lexical fallback: the FastEmbed venv and embedding model are baked into the image
at build time, so `ozy index` builds a real vector index while the runtime stays
egress-restricted to the model gateway (the glibc base + Python + onnxruntime +
bundled model roughly double the image, ~1.2 GB larger). A build-time smoke runs
`ozy doctor` with the network cut (`RUN --network=none`) and fails the build if
semantic isn't available offline. `environment.json` records `retrieval: semantic`.

### Scenarios

- **`historical-weather-report`** (default) — a 4-server chain (weather → search →
  wikipedia → PDF): find Vilnius's historical weather for a fixed date, research
  the city via **a privacy-focused web search engine** and Wikipedia, and produce a
  real `output/report.pdf`. The task names a *capability*, not a tool: success is
  judged on the outcome (the weather figures + city facts in the answer and PDF),
  so an agent that reaches the facts via any working search engine passes. The
  corpus still ships rival privacy-search distractors (`brave-search`/`kagi-search`/
  `bing-search`) — but they now behave like the real services (see below), so a
  rival pick is either usable or legibly unusable, never a silent trap. Repeatable
  by construction (fixed date, baked data).

### Two tiers

| Tier | Model required | What it measures |
|------|----------------|------------------|
| **Static surface** | No | Startup tool count, schema bytes, estimated tokens per mode — deterministic, runs natively |
| **Live agent** | Yes (free by default) | Task success, retrieval quality, tool-use behavior, token economy — from real OpenCode runs |

Token metrics come from OpenCode's own usage events (`token_source: measured`);
when a transcript carries none, they are estimated from transcript bytes plus
the enumerated startup schemas (`token_source: estimated`). Every artifact
labels which.

### Outcome-based grading (BREAKING vs pre-change numbers)

Run **success** is judged on the **outcome**, not on which tool the agent picked.
The grader gates `overall` on the *result* criteria only — `answer_must_contain`,
`commit_check`, artifact `must_contain`, and `forbidden_answer_patterns`. The
tool-usage fields (`required_tools`, `forbidden_tool_patterns`) are recorded as
**informational** telemetry in `grading.json` and never flip the verdict. This is
what a broker sells: the same or better outcome for a fraction of the
schema/token/latency cost. **This changes the pass/fail contract** — pre-change
pass/fail numbers (which folded tool attribution into success) are not comparable
to post-change numbers; don't mix them.

The `comparison.md` verdict leads with the efficiency + outcome metrics — startup
schema tokens → total tokens/run → tool calls/run → duration → success k/N — and
demotes canonical-hit / distractor-call figures to an informational footer.

### Retrieval quality and estate-scale findings

Every fixture server writes a **server-side invocation log** (`calls.jsonl`) —
the mode-symmetric ground truth for which downstream tool actually ran (in ozy
mode the transcript shows only broker calls). From it, `metrics.json` reports
`canonical_tools_hit` (k of N required tools), `distractor_call_count`, and
`downstream_call_count`, and `comparison.md` shows them per mode with deltas as
**informational** telemetry (they do not gate success).

Direct mode at 500+ tools may exceed a gateway's tool-definition cap or a model's
context. That is **recorded as a finding, never worked around**: the run records
a named failure reason (`toolset_rejected`, `context_overflow`), aggregates count
them, and the comparison states plainly that direct mode could not operate at
this estate size. The estate is never shrunk for one mode — identical
environments is the bench's core contract.

## Configuration

All variables are optional (defined once, in `docker-compose.yml`):

| Variable | Default | Description |
|----------|---------|-------------|
| `BENCH_MODEL` | `opencode/big-pickle` | Any model ID the pinned OpenCode resolves |
| `MODE` | `all` | `direct`, `direct-lean`, `ozy`, `all`, or `both` |
| `BENCH_RUNS` | `5` | Live runs per mode — usually set positionally: `make bench 10` |
| `SCENARIO` | `historical-weather-report` | Scenario to run (the only scenario) |
| `BENCH_TIMEOUT` | `900` | Hard per-run timeout (seconds) |

Effective values resolve as: **positional run count (`make bench N`) > environment
variable > built-in default.** None are required — `make bench` needs no env at all.

The pinned OpenCode's free tier (from `opencode models`):
`opencode/big-pickle` (default), `opencode/deepseek-v4-flash-free`,
`opencode/nemotron-3-ultra-free`, `opencode/hy3-free`, `opencode/mimo-v2.5-free`,
`opencode/north-mini-code-free`. An unresolvable `BENCH_MODEL` fails fast before
any live run. `big-pickle` is the default because `deepseek-v4-flash-free` stalls
mid-stream often enough to wedge runs.

## Run Directory Layout

Every path below is written by the harness:

```
bench/runs/<timestamp>-<scenario>/
  environment.json        # provenance: model, versions, estimator, usage source, retrieval stack
  surface.json            # deterministic startup surface, all three modes, computed once
  comparison.json         # cross-mode comparison (machine-readable)
  comparison.md           # cross-mode comparison + verdict, live-tier deltas (human-readable)
  direct/
    run-1/ … run-N/
      task.md  transcript.jsonl  tool-calls.jsonl
      calls.jsonl         # server-side invocation log (downstream tool ground truth)
      workspace/          # the agent's scratch dir; deliverables land here (e.g. output/report.pdf)
      final-answer.md  grading.json  metrics.json
    aggregate.json        # success k/N, retrieval quality, failure reasons, duration + token stats
  direct-lean/            # same shape (functional MCPs only, no corpus)
  ozy/                    # same shape
```

`environment.json` records `retrieval: semantic|lexical` — which retrieval stack
ozy actually exercised (verified at image build time, never assumed). The live
table in `comparison.md` carries a **Δ (ozy − direct-lean)** column (the primary
real-world comparison) and a **Δ (ozy − direct)** column for every metric when
the compared modes ran; a mode that wasn't run is labelled `not run (MODE=…)`,
never a bare "—".

A surface-only invocation writes `surface.json`, `environment.json`, and a
surface-only `comparison.{json,md}` (live sections marked skipped), and exits 0.

## Exit Codes

The harness exits **0** for any completed invocation — including one where the
agent loses the benchmark (that's in the report, not the exit code). It exits
**non-zero only for harness errors**: fixture generation failure, config error,
or an unresolvable model.

## Hermeticity

After `docker compose build`, a run performs **no network access other than
OpenCode's model gateway** for the selected model. There is no provider package
to install and no auth file to write — the live tier drives OpenCode's built-in
models. A build-time smoke test fails the image build if the agent cannot reach
its ready-to-run state with the npm registry unroutable, so a runtime package
fetch can never sneak in. Provenance (`environment.json`) records the model ID
as the one deliberately unpinned variable, and carries no credential of any kind.

## CI

The benchmark does **not** run in CI — it is run manually (`make bench`). The
surface tier can still be produced locally at any time with `make bench-surface`
(deterministic, no Docker or model). Whether and how to wire any of this into CI
is deliberately left open.

## Extending

Extension is data, not code:

- **New scenario** — add `bench/scenarios/<name>/`: `scenario.jsonc` (declaring
  its `toolsets` and whether the `corpus` attaches, default true), `task.md` (name
  the *capability* a step needs, not a tool), and `expected/ground_truth.json`.
  Ground truth is declarative — success gates on `answer_must_contain`,
  `commit_check`, `forbidden_answer_patterns`, and `artifacts` (`{path, type,
  must_contain}`, with PDF parse+extract for `type: pdf`); `required_tools` and
  `forbidden_tool_patterns` (matched against `calls.jsonl`) are recorded as
  informational only. No grader code changes.
- **New corpus server** — add a `bench/corpus/<server>.json` data file (or edit
  `gen_data.go` and regenerate). Each tool may carry a `behavior`
  (`functional:<backend>` | `auth_error` | `stub`, default `stub`) so a
  same-capability rival behaves like the real service. It is served, indexed, and
  counted with no code change.
- **New mode** — add `bench/configs/opencode.<mode>.jsonc`; the orchestrator
  resolves mode names to templates.
- **New model** — set `BENCH_MODEL`.
