# ozy

My name is Ozy, King of Context: Look on my MCP registry, ye Scrub, and despair!

Ozy is a local **agent tool broker**. You configure many downstream MCP servers
once; Ozy discovers and indexes their tools into a persistent, searchable
capability catalog. Agents connect only to Ozy and use a small, stable interface
to discover, understand, and invoke the right downstream tool — without loading
the entire downstream tool universe into context. See [SPEC.md](SPEC.md) for the
full product specification.

> **Status:** early implementation. Ozy can load `ozy.jsonc`, connect to
> configured downstream MCP servers, run `ozy index`, and persist discovered
> tool metadata for offline `list` and `describe`. Search ranking and brokered
> invocation are still pending; `search` returns `no_good_match` once the catalog
> is non-empty and `call` remains live-gated.

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
ozy doctor                     # diagnose config, env, catalog, and server health
ozy index                      # connect to configured MCP servers and persist tools
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

Ozy reads a single JSONC/JSON config using the opencode `mcp` shape. Path
precedence is:

1. `--config <path>` or `OZY_CONFIG`
2. `$XDG_CONFIG_HOME/ozy/ozy.jsonc`, or `~/.config/ozy/ozy.jsonc` when
   `XDG_CONFIG_HOME` is unset
3. the OS user-config equivalent on Windows, for example
   `%AppData%\ozy\ozy.jsonc`

Run `ozy init` to scaffold a commented `ozy.jsonc` at the default user config
path. Project-local configs are still supported explicitly, e.g.
`ozy --config ./ozy.jsonc index`.

Downstream servers are declared under `mcp`; Ozy supports opencode-compatible
`mcp` entries only, so common MCP snippets can be copied into Ozy config without
reshaping. Local servers support `type`, `command`, `cwd`, `environment`,
`enabled`, and `timeout`. Remote servers support `type`, `url`, `headers`,
`oauth`, `enabled`, and `timeout`. Omitted `enabled` means enabled; `timeout` is
milliseconds and defaults to `5000`. Ozy's own `search`, `embedding`, and
`budgets` sections are top-level siblings:

```jsonc
{
  "mcp": {
    "filesystem": {
      "type": "local",
      "command": ["filesystem-mcp", "--root", "."],
      "cwd": "/path/to/workspace",
      "environment": {
        "OZY_ROOT": "{env:OZY_ROOT}"
      },
      "enabled": true,
      "timeout": 5000
    },
    "atlassian": {
      "type": "remote",
      "url": "https://mcp.example.com/v1/mcp",
      "headers": {
        "Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}"
      },
      "oauth": false,
      "enabled": true
    }
  },
  "search": {
    "lexical": {"enabled": true},
    "semantic": {"enabled": false, "required": false}
  },
  "embedding": {"provider": "python-local", "required": false},
  "budgets": {
    "findTool": {"maxResults": 5, "includeFullSchemas": false},
    "describeTool": {"includeExamples": true},
    "callTool": {"maxResultBytes": 65536}
  }
}
```

Secrets should be supplied through `{env:NAME}` references rather than literals.
`ozy doctor` reports unresolved references by variable name and never prints
resolved secret values.

The discovered catalog is stored at `$XDG_STATE_HOME/ozy/catalog.json` by
default, falling back to `~/.local/state/ozy/catalog.json`. Override it with
`OZY_CATALOG` for tests or isolated runs.

To verify the checked-in real MCP examples against your local environment,
ensure the commands in `examples/test_mcp_examples.jsonc` are available and run:

```bash
OZY_RUN_REAL_MCP_EXAMPLES=1 make check-real-mcp-examples
```

## Agent interface

Ozy exposes exactly three stable MCP tools (SPEC.md §9):

- `findTool` — find the best known tool for a capability query (returns a
  decision, not just a list);
- `describeTool` — return the exact schema and usage guidance for one tool;
- `callTool` — invoke a downstream tool through Ozy (live-gated).

The CLI mirrors these operations through the same in-process broker, so the CLI
and MCP paths cannot drift.
