# Ozy Scenario Benchmark

A controlled, local-first, reproducible benchmark that compares direct-MCP
vs Ozy-brokered agent performance against a frozen incident scenario.

## Quick Start

1. Copy and edit the environment file:

```bash
cp bench/.env.example .env
# Edit .env — set MODEL_NAME, MODEL_BASE_URL, etc.
```

2. Run with Docker Compose:

```bash
docker compose -f bench/docker-compose.yml up
```

The benchmark runs hands-off — progress streams to stdout, and artifacts
land on the host at `bench/runs/<timestamp>-<scenario>/`.

3. Read the report:

```bash
cat bench/runs/*/comparison.md
```

## What It Measures

The benchmark runs one frozen incident scenario in two modes:

- **`direct`** — the agent is wired to all 7 fixture MCP servers directly
  (4 useful + 3 distractors).
- **`ozy`** — the agent sees only Ozy's broker interface
  (`findTool`/`describeTool`/`callTool`); Ozy brokers the same 7 servers.

The scenario, prompt, fixture, model endpoint, and environment are identical
across modes — only the tool wiring differs.

### Two Tiers

| Tier | Model Required | What It Measures |
|------|---------------|-----------------|
| **Static surface** | No | Startup tool count, schema bytes, estimated tokens — deterministic, CI-gateable |
| **Live agent** | Yes (BYO endpoint) | Token economy, tool-use behavior, task success — from real agent runs |

The static surface tier alone proves the "reduces tool/context surface" claim
with no model at all.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MODEL_NAME` | — | Model name (recorded in provenance) |
| `MODEL_BASE_URL` | `http://host.docker.internal:8080/v1` | OpenAI-compatible endpoint |
| `MODEL_API_KEY` | — | API key (never recorded) |
| `MODEL_TEMPERATURE` | `0.0` | Model temperature |
| `MODEL_MAX_TOKENS` | `4096` | Max output tokens |
| `SCENARIO` | `suspended-account-invoice-regression` | Scenario to run |
| `MODE` | `both` | `direct`, `ozy`, or `both` |
| `BENCH_RUNS` | `5` | Live runs per mode |

## Run Directory Layout

```
bench/runs/<timestamp>-<scenario>/
  scenario.jsonc       environment.json
  surface.json          # deterministic, computed once
  direct/
    run-1/ … run-N/
      transcript.jsonl context-ledger.jsonl tool-calls.jsonl
      metrics.json final-answer.md grading.json
    aggregate.json      # mean/min/max/stdev + success rate k/N
  ozy/   (same shape)
  comparison.json  comparison.md
```

## The Claim

The benchmark proves that **in the same environment, Ozy reduces the
agent-facing tool/context surface while preserving task success**. The static
surface tier carries this claim independent of any model — it needs no model
endpoint and can gate in CI.
