<div align="center">

<img src="assets/ozy.png" alt="Ozy" width="400" />

# Ozy

**The local agent tool broker.**

*My name is Ozy, Thing of Things. Look on my MCP setup, ye Agent, and vibe!* — Ozymandias, probably.

<br/>

[![License](https://img.shields.io/badge/License-Apache_2.0-blue?style=for-the-badge&logo=apache&logoColor=white)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?style=for-the-badge&logo=go&logoColor=white)](go.mod)
[![CI](https://img.shields.io/github/actions/workflow/status/rokasklive/ozy/ci.yml?style=for-the-badge&label=CI)](.github/workflows/ci.yml)
[![MCP](https://img.shields.io/badge/MCP-Server-1f6feb?style=for-the-badge)](SPEC.md)
[![Evals](https://img.shields.io/badge/Evals-Tracked-success?style=for-the-badge)](evals/BENCHMARKS.md)

</div>

---

## What is Ozy?

Ozy is a local **agent tool broker**. Configure your downstream MCP servers
once; Ozy discovers and indexes their tools into a persistent, searchable
catalog. Agents connect to Ozy and use a small, stable interface — `findTool`,
`describeTool`, `callTool` — to discover and invoke the right downstream tool
**without loading the entire downstream tool universe into context**.

## Why?

Connecting an agent to many MCP servers usually means stuffing every tool
description into the model's context. That does not scale, and it does not
degrade gracefully when a server is offline.

Ozy fixes that by acting as a single, brokered entry point:

- **One stable agent interface** — three tools, always, regardless of how
  many downstream servers you wire up.
- **Persistent catalog** — tools are indexed once and queried offline; no live
  re-discovery on every agent turn.
- **Hybrid search** — lexical + optional semantic ranking fused with
  Reciprocal Rank Fusion. Semantic falls back to lexical gracefully if the
  embedding sidecar is unavailable.
- **Live-gated invocation** — `callTool` connects to a single downstream
  server at call time; the catalog is served even when servers are down.

## Quick start

The one-line bootstrap inspects your machine, prints a plan (nothing changes
until you confirm), then installs the `ozy` binary, writes a starter config,
provisions the embedding sidecar, and puts `ozy` on your `PATH`:

```bash
go run github.com/rokasklive/ozy/cmd/ozy-install@latest
```

It is dry-run-first, consent-based, and safe to rerun. Then:

```bash
# 1. Declare your downstream MCP servers in your ozy.jsonc
#    (copy-paste your existing mcp.json entries — the shape is opencode-compatible)

# 2. Index, then run
ozy index                # connect, discover, persist the catalog
ozy search "confluence"  # find the right tool
ozy mcp                  # expose Ozy to your agent
```

Prefer to manage the binary yourself? Install it directly and scaffold a config:

```bash
go install github.com/rokasklive/ozy/cmd/ozy@latest
ozy init
```

To remove Ozy, run `ozy uninstall` (or
`go run github.com/rokasklive/ozy/cmd/ozy-install@latest uninstall`). It is
plan-first and conservative — your config and downstream MCP definitions are
kept unless you pass `--purge`.

## Usage

```bash
ozy init                       # scaffold a starter config
ozy doctor                     # diagnose config, env, catalog, and server health
ozy index                      # connect to MCP servers and persist tools
ozy list                       # list indexed tools
ozy search "search confluence wiki"
ozy describe atlassian.confluence_search
ozy call  atlassian.confluence_search --json '{"query":"billing migration","limit":5}'
ozy daemon                     # run the daemon
ozy mcp                        # serve the MCP adapter over stdio
ozy eval run                   # run the eval suite over the committed corpus
ozy uninstall                  # remove Ozy (plan-first; keeps config unless --purge)
```

Install and removal run through a separate bootstrap so they work before `ozy`
exists: `go run github.com/rokasklive/ozy/cmd/ozy-install@latest` to set up, and
`… /cmd/ozy-install@latest uninstall` (or `ozy uninstall`) to remove.

Every command accepts a global `--format` flag: `human` (default), `json`
(single machine-readable document for agents and evals), or `concise`.

## Configuration

Ozy reads a single JSONC config at one of:

1. `--config <path>` or `$OZY_CONFIG`
2. `$XDG_CONFIG_HOME/ozy/ozy.jsonc` (or `~/.config/ozy/ozy.jsonc`)
3. `%AppData%\ozy\ozy.jsonc` on Windows

Run `ozy init` to scaffold a fully commented starter at the default path. The
shape is **opencode-compatible**, so existing `mcp.json` entries can be copied
in unchanged.

```jsonc
{
  "mcp": {
    "filesystem": {
      "type": "local",
      "command": ["filesystem-mcp", "--root", "."],
      "cwd": "/path/to/workspace",
      "environment": { "OZY_ROOT": "{env:OZY_ROOT}" },
      "enabled": true,
      "timeout": 5000
    },
    "atlassian": {
      "type": "remote",
      "url": "https://mcp.example.com/v1/mcp",
      "headers": { "Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}" },
      "oauth": false,
      "enabled": true
    }
  },
  "search":     { "lexical": { "enabled": true }, "semantic": { "enabled": true } },
  "embedding":  { "provider": "python-local", "vectorBackend": "turbovec" },
  "budgets":    { "findTool": { "maxResults": 5 }, "callTool": { "maxResultBytes": 65536 } }
}
```

**Secrets** belong in `{env:NAME}` references — `ozy doctor` reports
unresolved variables by name and never prints resolved secret values. The
catalog is stored at `$XDG_STATE_HOME/ozy/catalog.json` (override with
`$OZY_CATALOG` for tests).

Semantic search is on by default. The Python embedding sidecar is
**auto-provisioned on demand** via `uv` (with a `python -m venv` + `pip`
fallback); if it is unavailable, Ozy falls back to lexical-only `findTool`
and surfaces the degraded mode rather than failing.

## Use Ozy as an MCP server

Add Ozy to your agent's MCP config:

```jsonc
{
  "mcp": {
    "ozy": {
      "type": "local",
      "command": ["ozy", "mcp"],
      "cwd": "/path/to/your/project"
    }
  }
}
```

On startup `ozy mcp` loads `~/.config/ozy/ozy.jsonc`. When the agent calls
`findTool`, Ozy connects to every enabled downstream server, runs `tools/list`,
and returns stable `toolRef`s (e.g. `atlassian.confluence_search`) with
`title`, `description`, and `inputSchema`. The agent then calls `callTool`
with that `toolRef` and the arguments from the candidate's `inputSchema` —
no `ozy index` required beforehand.

## The three tools

Ozy exposes exactly three stable MCP tools (see [SPEC.md](SPEC.md) §9):

| Tool           | Purpose                                                                            |
| -------------- | ---------------------------------------------------------------------------------- |
| `findTool`     | Find the best known tool for a capability query — a decision, not just a list.     |
| `describeTool` | Return the exact schema and usage guidance for one tool (catalog-backed).          |
| `callTool`     | Invoke a downstream tool through Ozy with budget-bounded results.                 |

The CLI mirrors these operations through the same in-process broker, so the
CLI and MCP paths cannot drift.

## Documentation

- [**SPEC.md**](SPEC.md) — the living product specification. Start here for
  the full architecture, contract, and design.
- [**evals/BENCHMARKS.md**](evals/BENCHMARKS.md) — public discovery / invocation
  scoreboard over the committed corpus.
- [**examples/ozy.jsonc**](examples/ozy.jsonc) — annotated starter config.
- [**CONTRIBUTING.md**](CONTRIBUTING.md) — build, test, lint, and how to
  contribute.

## License

[Apache License 2.0](LICENSE).
