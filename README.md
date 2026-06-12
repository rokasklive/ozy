# ozy

My name is Ozy, King of Context: Look on my MCP registry, ye Scrub, and despair!

Ozy is a local **agent tool broker**. You configure many downstream MCP servers
once; Ozy discovers and indexes their tools into a persistent, searchable
capability catalog. Agents connect only to Ozy and use a small, stable interface
to discover, understand, and invoke the right downstream tool — without loading
the entire downstream tool universe into context. See [SPEC.md](SPEC.md) for the
full product specification.

> **Status:** project skeleton. The architectural seams (CLI, daemon, MCP
> adapter, config, catalog, broker) are wired and the agent-facing contracts are
> in place, but real search and downstream invocation are not yet implemented.
> Commands whose behavior is pending return a structured `NOT_IMPLEMENTED`
> response.

## Build

```bash
make build      # produces ./ozy
make test       # go test ./...
make lint       # go vet + gofmt check
```

Requires a recent Go toolchain (see `go.mod`).

## Usage

```bash
ozy init                       # scaffold a starter config
ozy doctor                     # diagnose config, env, and adapter readiness
ozy list                       # list indexed tools
ozy search "search confluence wiki"
ozy describe atlassian.confluence_search
ozy call atlassian.confluence_search --json '{"query":"billing migration","limit":5}'
ozy daemon                     # run the daemon
ozy mcp                        # serve the MCP adapter over stdio
```

Every command accepts a global `--format` flag with `human` (default), `json`
(a single machine-readable document for agents and evals), or `concise`.

## Configuration

By default Ozy reads its configuration from `$XDG_CONFIG_HOME/ozy/config.yaml`
(or the OS user-config equivalent). Override the path with `--config <path>` or
the `OZY_CONFIG` environment variable. Run `ozy init` to scaffold a starter
file.

Secrets should be supplied through `${ENV_VAR}` references rather than literals;
`ozy doctor` reports any unresolved references by name and never prints resolved
secret values. See [SPEC.md §11](SPEC.md) for the full configuration model.

## Agent interface

Ozy exposes exactly three stable MCP tools (SPEC.md §9):

- `findTool` — find the best known tool for a capability query (returns a
  decision, not just a list);
- `describeTool` — return the exact schema and usage guidance for one tool;
- `callTool` — invoke a downstream tool through Ozy (live-gated).

The CLI mirrors these operations through the same in-process broker, so the CLI
and MCP paths cannot drift.
