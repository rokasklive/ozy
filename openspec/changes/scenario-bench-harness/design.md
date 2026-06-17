## Context

Ozy is a local agent **tool broker**: agents connect to one Ozy MCP server and use
`findTool`/`describeTool`/`callTool` instead of loading every downstream tool
schema into context (`README.md`, `SPEC.md` §1). The existing eval suite
(`evals/`, `internal/eval`) measures the *static* token-economy ratio by driving
the broker seams over a synthetic corpus — deterministic, no model, no real MCP
servers. `SPEC.md` §14.3 reserves a complementary measurement it does not yet
build: a **direct-MCP-baseline vs Ozy-broker** comparison with a real agent and
ContextSpy as an optional profiler.

This design specifies that benchmark for one frozen scenario,
`suspended-account-invoice-regression`, local-first and reproducible. Constraints
from the request: keep it cheap, mock what we can, no OpenGrok, no public
internet, no hosted model, no Kubernetes, no giant integration. Fit existing
conventions: Go + cobra, JSONC config via `tailscale/hujson` with `{env:VAR}`,
the `internal/eval.TokenEstimator` seam, and the opt-in-heavy-leg pattern the eval
suite already uses for its semantic tier (`OZY_EVAL_SEMANTIC=1`).

## Goals / Non-Goals

**Goals:**
- Prove, with a rerunnable number, that **in the same environment Ozy reduces the
  agent-facing tool/context surface while preserving task success.**
- Make the cheap, deterministic half (startup tool/schema surface) provable with
  **no model at all**, so it can gate in CI.
- Run a **real agent over a configurable OpenAI-compatible endpoint** for the
  behavioral half, with an **itemized context ledger**, not just totals.
- Keep modes **identical** except tool wiring (scenario, prompt, fixture, model,
  env all byte-identical).
- Be incrementally implementable; fit repo conventions for names/dirs/config.

**Non-Goals (v0):**
- Real OpenGrok instance/index (fixture code-search MCP stands in).
- GitHub MCP, any public network, any hosted/required model provider.
- Kubernetes, a UI beyond the Markdown report, an LLM-as-judge **gate**.
- A large/production fixture; compiling/running the Java fixture (source is read,
  never built).
- Multi-scenario suites, N-run statistics, and CI publishing (future extensions).

## Decisions

### D1 — Architecture: one orchestrator, one parameterized fixture MCP, two Compose services

```
host: OpenAI-compatible model endpoint (Ollama / LM Studio / llama.cpp / vLLM / OpenRouter / …)
        ▲  MODEL_BASE_URL (sanitized in report)
        │
┌───────┴────────── docker compose ──────────────────────────────┐
│  contextspy  (python, pinned ==0.2.0)  ── reverse-proxy ──▶ host │
│     ▲ logs every LLM request → SQLite (ground-truth context)     │
│     │ OpenCode points MODEL_BASE_URL here                        │
│  bench-runner  (ozy + ozy-bench + opencode + fixture)            │
│     └ ozy-bench run --scenario … --mode direct|ozy|both          │
│          mode=direct:  opencode ─▶ [7 fixture MCP servers]       │
│          mode=ozy:     opencode ─▶ ozy mcp ─▶ [same 7 servers]   │
└──────────────────────────────────────────────────────────────────┘
fixture MCP servers are stdio child processes (ozy-bench mcp --toolset X), NOT compose services.
```

- **One Go binary `cmd/ozy-bench`** (consistent with `cmd/ozy-install`) with two
  subcommands: `run` (the `bench-runner` orchestrator) and `mcp --toolset <name>`
  (the fixture MCP server). *Alt: separate binaries — rejected, more build
  targets, no benefit.*
- **Compose has two services**: `bench-runner` and `contextspy`. The model is on
  the host (`host.docker.internal`). The fixture MCP servers are **stdio child
  processes** spawned by OpenCode (direct) or Ozy (ozy), so they are not network
  services — this is what keeps Compose minimal. *Alt: a service per MCP — rejected,
  stdio MCP is co-located by design and 6+ services is the "integration monster"
  the request warns against.*
- Core logic lives in **`internal/bench`** (loader, orchestration, ledger export,
  surface measurement, grading, reporters); `cmd/ozy-bench` is a thin cobra shell.

### D2 — Fixture MCP servers: one binary, many toolset identities

`ozy-bench mcp --toolset code-search|git|incident-db|filesystem|time|memory|notes`.
Launching it seven times with different `--toolset` flags yields seven distinct
MCP server surfaces from one implementation. Useful surfaces:
`code-search` (`search_text`, `search_symbol`, `read_file`, `find_references` —
backed by `rg`/file reads over the fixture), `git` (`git_log`, `git_show`,
`git_blame`, `git_diff` — shells `git` in the fixture repo), `incident-db`
(`list_tables`, `describe_table`, `query_readonly` — read-only SQLite),
`filesystem` (`read_file`, `list_dir`). Distractors: `time` (`current_time`,
`convert_timezone`), `memory` (`search_memory`, `store_memory`), `notes`
(`create_plan`, `append_note`). *Alt: off-the-shelf MCP servers — rejected,
non-deterministic and adds external deps; the point is controlled surface
pressure, not feature-completeness.*

### D3 — Direct vs Ozy: minimal-diff wiring

Two OpenCode configs plus one Ozy downstream config, differing only in wiring:
- `bench/configs/opencode.direct.jsonc` — lists all seven fixture servers
  directly.
- `bench/configs/opencode.ozy.jsonc` — lists only `ozy mcp`.
- `bench/configs/ozy.downstream.jsonc` — Ozy's own config; lists the **same
  seven** servers as downstream (`mcp` block, real Ozy format). In ozy mode the
  orchestrator runs `ozy index` against this config, then `ozy mcp`.

Model, prompt, fixture, and env are identical across modes; only the wiring file
differs. This is the controlled experiment. *Alt: programmatic wiring — rejected,
config files are the real-world surface and keep the diff auditable.*

### D4 — Agent runner: OpenCode headless, behind a thin adapter

The repo already ships `.opencode/`. The orchestrator invokes OpenCode in its
non-interactive run mode with the per-mode config and the scenario prompt, and
captures stdout/transcript + tool-call events. A small `Runner` interface
(`Run(ctx, mode, cfg, prompt) (transcript, error)`) isolates the runner so a
later variant can swap agents without touching metrics. *Alt: build a bespoke
agent loop — rejected, OpenCode is the established runner and writing an agent is
out of scope.* ponytail: one implementation now, interface only because the
direct/ozy split already needs a seam.

### D5 — Context capture: ContextSpy reverse-proxy, with a graceful estimator fallback

ContextSpy v0.2.0 is a Python proxy that intercepts OpenAI-compatible LLM
requests and stores each request's **full prompt composition** (system prompt,
tool definitions, tool results, history) to local SQLite — exactly the itemized
ledger the request wants, measured on the wire rather than guessed. For a local
OpenAI-compatible endpoint we use ContextSpy **local mode** (`contextspy
start-local`): a `[[reverse_targets]]` entry in `~/.contextspy/config.toml` sets
`target_url` to the model endpoint (from `MODEL_BASE_URL`) with
`provider = "openai"` and a `listen_port`; OpenCode then points `MODEL_BASE_URL`
at `http://contextspy:<listen_port>/v1`. This is a reverse proxy, so **no CA
certificate is needed** — the MITM CA-cert-in-trust-store dance is only for the
cloud forward-proxy mode (confirmed against the v0.2.0 docs and by the project
owner). The upstream is fully configurable via `target_url`, so any
OpenAI-compatible endpoint works unchanged.

ContextSpy is the **preferred backend, not a hard dependency** (matches `SPEC.md`
§14.3). When ContextSpy or the endpoint is absent, the harness still emits a
ledger built from (a) the startup tool schemas it enumerates directly and (b) the
tool-call/result transcript, tokenized with the existing estimator and labeled
`estimated`. ContextSpy upgrades those items to `measured`. *Alt: write our own
logging proxy — rejected, ContextSpy exists, is pinned, and is the named §14.3
backend. Alt: estimate only — kept as the fallback, not the default.*

### D6 — Token accounting: reuse the documented estimator, label provenance

Reuse `internal/eval.TokenEstimator` (chars/4 heuristic, swappable, named in
provenance). Every ledger item and metric carries `tokenSource: measured |
estimated`. When ContextSpy or the endpoint reports real usage, those numbers are
`measured` and override estimates. *Alt: bundle a BPE tokenizer — deferred; the
comparison is a ratio, robust to the approximation, and the request explicitly
permits a labeled approximation.*

### D7 — Incident DB: real read-only SQLite

The generator builds `db/incident.sqlite` with the `sqlite3` CLI (present on dev
machines; a one-line apt install in the image). The `incident-db` toolset opens it
**read-only** and rejects any statement that is not `SELECT`/`PRAGMA`/`EXPLAIN`.
Reader driver: pure-Go `modernc.org/sqlite` (no cgo, clean single binary) — also
used to read ContextSpy's export. *Alt: shell `sqlite3 -readonly` and parse output
— rejected, fragile row parsing; the one pure-Go dep is worth structured rows.*

### D8 — Grading: deterministic rubric, no model

`internal/bench` scores the final answer + tool-call log against
`expected/ground_truth.json`, emitting `grading.json` (per-criterion pass/fail +
overall): root-cause file path present, root-cause function name present, culprit
commit matched by **subject** (stable) or recorded **hash**, expected test name
present, patch (if any) targets the expected file, and forbidden checks — no
distractor/`web`-class tool was called (from the tool-call log), no broad refactor
(patch touches only the expected file / no multi-file sprawl), no public-internet
use. *Alt: LLM-as-judge — explicitly optional/future; the rubric is objective and
reproducible.*

### D9 — Determinism

Fixture: the generator pins author/committer identity and `GIT_*_DATE`, fixed
content and commit order, so the tree (and thus the culprit SHA, per generator
version) is reproducible; the resolved culprit SHA is written into
`environment.json` post-generation so grading can match hash or subject. Surface
tier: fully deterministic (schema enumeration), so it is computed once regardless
of run count. Live tier: `MODEL_TEMPERATURE=0` and any provider seed are set and
recorded, but the model is still probabilistic, so the live tier runs **N times
per mode** (see D12) and reports per-run results plus aggregates rather than
trusting a single sample.

### D10 — Config & artifacts: JSONC + the requested run layout

Scenario config is **JSONC** (`bench/scenarios/<name>/scenario.jsonc`), parsed
with `tailscale/hujson`, model endpoint via `{env:VAR}` / `*_env` keys, fitting
`ozy.jsonc`. Base URL is sanitized for the report with `internal/config`
redaction. Run output matches the requested shape:

```
bench/runs/<ts>-<scenario>/
  scenario.jsonc  environment.json
  surface.json                         # deterministic, computed once
  direct/
    run-1/ … run-N/  transcript.jsonl  context-ledger.jsonl  tool-calls.jsonl  metrics.json  final-answer.md  grading.json
    aggregate.json                     # mean / min / max / stdev + success rate k/N
  ozy/   (same shape)
  comparison.json  comparison.md
```

`bench/runs/` is git-ignored and **bind-mounted to the host** in Compose, so the
report lands on the operator's disk without entering the container.
`comparison.md` is the publishable headline (startup tools, startup schema tokens,
total tokens, irrelevant tool calls, success rate per mode, deltas), showing both
per-run values and aggregates.

### D11 — Hermeticity boundary: everything is sealed except the model endpoint

The benchmark is **hermetic in its environment, fixture, and tool surface**. The
fixture repo and incident DB are generated deterministically and vendored into the
run; every MCP server is a local stdio child process over that fixture (no
network); the agent's tool surface contains **no internet-reaching capability**,
so the "no public internet" rule is enforced by *absence*, not policy; and Compose
plus pinned `ozy`/OpenCode/ContextSpy versions seal the rest. The **static surface
tier is therefore both hermetic and bit-reproducible.**

The **one deliberate non-hermetic seam is the model endpoint.** It is external by
design (BYO OpenAI-compatible endpoint, not baked into the image per the request)
and may even be remote (OpenRouter/DeepSeek cross the public internet — permitted
but no longer hermetic). LLM inference is **not bit-deterministic even at
temperature 0 and even locally** (kernel/batching/version variance). We handle
this rather than pretend it away: (a) each mode is run **N times against the same
endpoint** and compared as matched aggregates (D12), so model variance is averaged
rather than sampled once; (b) the model name/version and sanitized base URL are
recorded in provenance so runs are compared only like-for-like; (c) per-run
success is pass/fail and the headline is a **success rate k/N** per mode. For a **fully hermetic
run**, pin a specific local model (fixed server version + weights) and treat it as
part of the environment — v0 does not vendor a model because the request asks not
to bake one in.

### D12 — N runs, isolation between runs, and hands-off operation

The live tier runs `BENCH_RUNS` (default 5; CLI `--runs`/scenario config) times
per mode — all N direct, then all N ozy, against the same endpoint — and reports
**per-run breakdown plus aggregates** (mean/min/max/stdev for numeric metrics, a
success rate **k/N** per mode). Aggregation is intentionally plain arithmetic:
**no confidence intervals, significance tests, or charts** in v0 (deferred). The
deterministic surface tier is computed once, not N times.

**Each run is an independent sample**, which requires a clean wipe between runs —
otherwise a persisted agent would carry the prior answer forward and contaminate
the result. This is achieved by *construction* rather than a teardown framework:
every run gets fresh ephemeral state dirs (a throwaway agent home/session and a
throwaway Ozy state dir, then `ozy index` from scratch), the fixture is mounted
**read-only** (our MCP servers are read-only anyway, so nothing to reset), and the
`memory`/`notes` distractors keep state in-process so it dies with the run. *Alt:
reuse one agent/catalog across runs — rejected, the runs would not be independent.*

**Hands-off operation** — the operator never enters the container. They edit a
`.env` (model URL, scenario, mode, `BENCH_RUNS`), run `docker compose up`, and:
- the runner **streams progress + a heartbeat** to stdout (`mode=ozy run 3/5:
  turn 4, tool_call git_show… 31s elapsed`) so liveness is visible and a stall is
  obvious;
- each run has a **hard `timeout_seconds`**; a hung agent is killed, recorded as
  `timed_out`, and the batch continues (one hang never freezes the suite), and an
  unreachable endpoint fails fast with a clear message;
- artifacts land on the host via the bind-mounted `bench/runs/` (D10), and the
  final `comparison.md` path is printed at the end.

## Risks / Trade-offs

- **Local-model nondeterminism** → temperature 0 + recorded seed; **N runs per
  mode with aggregates + success rate k/N** (D12) so the delta is not a single
  sample; the deterministic surface tier carries the core claim independent of the
  model.
- **Weak local model fails the task in both modes** → acceptable: surface +
  behavior deltas still report, and "without hurting success" is a *relative*
  direct-vs-ozy comparison, not an absolute bar.
- **ContextSpy capture unavailable in a given environment** (not installed, or a
  run without the proxy) → the estimator-ledger fallback keeps v0 working; the
  local-mode wiring itself is settled (reverse proxy, configurable `target_url`,
  no CA cert).
- **OpenCode non-interactive flags / transcript shape drift** → isolate behind the
  `Runner` adapter; pin the OpenCode version in the image.
- **Fixture too easy or leaky** → distractors + multi-commit history + a single
  deterministic `SUSPENDED→ACTIVE` bug provide realistic pressure without bloat.
- **New SQLite dep** → pure-Go `modernc.org/sqlite`, read-only use only; isolated
  to `internal/bench` and the fixture server.
- **Scope creep** → staged tasks; static tier ships and proves the claim before
  any live/Docker/ContextSpy work.

## Migration Plan

Additive only; no existing behavior changes, nothing to roll back beyond deleting
`bench/`, `cmd/ozy-bench`, and `internal/bench`. Staged so value lands early:

1. Fixture generator + ground truth (testable standalone).
2. Fixture MCP servers (unit tests: read-only enforcement, deterministic output).
3. Scenario loader + provenance + **static surface tier** + surface-only
   comparison — **proves the core claim with no model**.
4. Context ledger schema + deterministic grading rubric.
5. Docker Compose (bind-mounted `runs/`, `.env` config, streamed logs) +
   ContextSpy + OpenCode direct/ozy configs.
6. **Live agent tier**: single-run path first, then the N-run loop with per-run
   isolation, aggregation, per-run timeout, and the full comparison (opt-in; needs
   an endpoint).

## Open Questions

- OpenCode non-interactive run flag and transcript/tool-event JSON shape — confirm
  at apply and pin the version.

## Future Extensions

OpenGrok-backed code-search variant; a GitHub MCP scenario; richer N-run
statistics (confidence intervals, significance tests, distribution charts) on top
of the v0 aggregates; a local-model comparison matrix; CI artifact publishing of
`comparison.md`; additional scenarios; optional calibrated LLM-as-judge grading.
