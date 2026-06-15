## Why

Ozy's whole premise (`README.md`, `SPEC.md` §1, §13) is that brokering many
downstream MCP servers behind one small interface shrinks the agent-facing
tool/context surface **without** hurting task success. The existing eval suite
(`evals/`, `internal/eval`) proves the *static* token-economy ratio by driving
the broker seams directly over a synthetic corpus — but it never launches a real
agent, a real model, or real MCP servers, so it cannot answer the question that
actually sells Ozy: **given the same agent, model, task, fixture repo, and MCP
capabilities, does putting Ozy in front change how the agent behaves and what it
costs?** `SPEC.md` §14.3 already reserves a slot for exactly this — a
direct-MCP-baseline-vs-Ozy-broker comparison with ContextSpy as an optional
measurement backend — but nothing implements it. This change builds that
controlled, local-first, reproducible scenario benchmark so the headline claim
becomes a measured number anyone can rerun, not a story.

## What Changes

- Add a **scenario benchmark harness** that runs one frozen incident scenario in
  two modes against an identical environment and emits a side-by-side report:
  - **`direct`** — the agent runner (OpenCode) is wired to all fixture MCP
    servers directly (useful + distractor surfaces exposed at startup).
  - **`ozy`** — the agent runner sees **only Ozy**; Ozy brokers the same fixture
    servers as its downstream (`findTool`/`describeTool`/`callTool`).
  - **`both`** — run direct then ozy and produce the comparison.
  - Scenario, prompt, fixture repo, model endpoint, and expected answer are
    byte-identical across modes; only the tool wiring differs.
- Add a **two-tier measurement** so the core claim is provable cheaply and the
  expensive leg stays optional (mirrors the suite's opt-in semantic leg):
  - **Static surface tier (no model required)** — start each mode's MCP wiring,
    enumerate tools/schemas advertised *at startup*, and report the surface
    delta (tool count, schema bytes, estimated schema tokens) deterministically.
    This alone proves the "reduces tool/context surface" half and can gate in CI.
  - **Live agent tier (opt-in, needs a model endpoint)** — drive a real agent
    over a configurable **OpenAI-compatible** endpoint, capture the real
    on-the-wire context via ContextSpy, and report tool-use behavior, token
    economics, and task success for each mode.
- Add a **deterministic fixture generator** that builds the
  `suspended-account-invoice-regression` fixture: a small `acme-billing` repo
  with real git history (a culprit commit that maps `SUSPENDED → ACTIVE`), local
  docs, a read-only incident SQLite DB, and a machine-checkable
  `expected/ground_truth.json`.
- Add **lightweight fixture MCP servers** (one parameterized binary, many
  toolset identities): code-search, git, incident-db, filesystem, plus
  time/memory/notes **distractors** — enough surface pressure to make the broker
  delta meaningful, with no OpenGrok, no network, and no hosted services.
- Add a **context ledger + grading layer**: a JSONL itemized
  context/content ledger (per `kind`: `tool_schema`, `tool_call`, `tool_result`,
  …), a deterministic success rubric scored against `ground_truth.json`, and
  machine-readable + human-readable comparison artifacts per run.
- Add a **Docker Compose** environment (minimal: a `bench-runner` and the
  ContextSpy proxy; the model runs on the host, fixture MCP servers are stdio
  child processes) and the `bench-runner` CLI:
  `--scenario <name> --mode direct|ozy|both`.
- Configuration uses the project's existing **JSONC + `{env:VAR}`** convention
  (`tailscale/hujson`); the model endpoint is supplied entirely through env
  (`MODEL_BASE_URL`, `MODEL_API_KEY`, `MODEL_NAME`, …) and recorded — sanitized —
  in run provenance.

## Capabilities

### New Capabilities
- `scenario-bench`: The benchmark harness — the scenario config schema (JSONC),
  the `bench-runner` entrypoint and `direct|ozy|both` orchestration, the
  identical-environment guarantee across modes, the OpenAI-compatible model
  endpoint configuration, run-directory lifecycle, and the recorded run/runtime
  provenance (model, sanitized base URL, temperature, max tokens, timestamp, Ozy
  git SHA, scenario hash, mode, run count).
- `bench-fixtures`: The generated scenario fixture — the deterministic
  `acme-billing` generator, its real multi-commit git history with a
  deterministic culprit commit, the local docs and read-only incident SQLite DB,
  the rigid task prompt, and the machine-checkable `ground_truth.json` /
  forbidden-behavior contract.
- `bench-mcp-fixtures`: The mock/lightweight MCP capability surfaces — the
  parameterized fixture server (code-search, git, incident-db, filesystem) and
  the time/memory/notes distractors, their tool contracts, read-only
  enforcement, and deterministic responses, exposed directly in `direct` mode and
  as Ozy downstream in `ozy` mode.
- `context-ledger`: The measurement and reporting surface — the ContextSpy
  capture integration (optional backend, not a runtime dependency), the JSONL
  context-ledger item schema, the static-surface and live token/tool-behavior
  metrics, the deterministic grading rubric, and the per-run `comparison.json` /
  `comparison.md` artifacts.

### Modified Capabilities
<!-- None. The benchmark is additive: it *uses* the existing `ozy mcp` / `ozy index`
     surface unchanged and lives in a separate binary and directory, so no existing
     spec's requirements change. -->

## Impact

- Affected code:
  - `bench/` (**new**): scenario configs, the fixture generator, the
    direct/ozy/Ozy-downstream agent configs, `docker-compose.yml`, `Dockerfile`,
    a `README.md`, and a git-ignored `runs/` output tree.
  - `cmd/ozy-bench` (**new** Go binary, consistent with `cmd/ozy-install`): a
    `run` subcommand (the orchestrator/`bench-runner`) and an `mcp --toolset
    <name>` subcommand (the fixture MCP server).
  - `internal/bench` (**new** Go package): scenario loader, mode orchestration,
    ContextSpy-SQLite → JSONL ledger export, static surface measurement,
    deterministic grading, and the JSON + Markdown comparison reporters.
- Reused, unchanged: `internal/eval.TokenEstimator` (the documented swappable
  estimator, labeled "estimated"), `internal/config` base-URL redaction,
  `github.com/modelcontextprotocol/go-sdk` (fixture servers), `tailscale/hujson`
  (scenario JSONC), and the `ozy mcp` / `ozy index` CLI surface.
- Dependencies: the live tier needs an OpenAI-compatible model endpoint (env),
  an agent runner (OpenCode), and ContextSpy (Python, pinned `==0.2.0`, a Compose
  service only — never linked into the Go binary). The static surface tier needs
  none of these. A pure-Go SQLite reader may be added for the incident DB and the
  ContextSpy export (design picks driver vs `sqlite3` CLI).
- Non-goals (v0): a real OpenGrok instance/index; GitHub MCP; any public-internet
  or hosted-model requirement; Kubernetes; a UI beyond the Markdown report; an
  LLM-as-judge gate (deterministic rubric only); a large/production fixture; a
  Java build/run system (fixture source is read, never compiled); multi-scenario,
  N-run statistics, and a published CI scoreboard (all listed as future
  extensions in `design.md`).
