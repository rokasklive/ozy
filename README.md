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
> tool metadata for offline `list` and `describe`. `findTool` ranks the
> **persistent catalog** with a hybrid (lexical + optional semantic) search
> engine and returns the single best tool plus one runner-up with confidence
> and a reason — no live discovery needed. The lexical and semantic
> rankings are fused with **Reciprocal Rank Fusion (RRF)**, so they are
> combined by rank rather than by mixing their incomparable raw scores. The
> semantic leg is produced by an integrated **Python embedding sidecar** that
> the Go daemon launches over stdio (newline-delimited JSON), embeds with
> **FastEmbed** (ONNX, CPU-only), and serves an ANN search over **turbovec**
> by default (4-bit quantization, kernel-level allowlist filtering). **FAISS**
> is an opt-in alternative. The sidecar environment is auto-provisioned on
> demand via `uv` (with a `python -m venv` + `pip` fallback) and the env is
> cached under XDG state. The daemon indexes the catalog **on startup** when
> it is stale (never indexed, or configured servers changed), and serves the
> existing catalog gracefully even when downstream servers are offline.
> `callTool` remains live-gated: invocation connects to the target server at
> call time. `describeTool` returns the exact schema from the catalog.
> Semantic search is **enabled by default**; when Python or the sidecar is
> absent, fails to provision, crashes, or a query times out, the daemon
> marks semantic unavailable, continues serving `findTool` from the lexical
> baseline, and surfaces the degraded mode rather than failing.

## Build

```bash
make build          # produces ./ozy
make test           # go test ./...
make tools          # install the pinned golangci-lint
make lint           # go vet + gofmt check
make install-hooks  # run make lint before git push
```

Requires a recent Go toolchain (see `go.mod`). Run `make install-hooks` once per
clone to enable the tracked pre-push hook.

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
    "semantic": {"enabled": true, "required": false}
  },
  "embedding": {
    "provider": "python-local",
    "vectorBackend": "turbovec",
    "model": "BAAI/bge-small-en-v1.5",
    "required": false
  },
  "budgets": {
    "findTool": {"maxResults": 5, "includeFullSchemas": false},
    "describeTool": {"includeExamples": true},
    "callTool": {"maxResultBytes": 65536}
  }
}
```

Semantic search is **on by default** (omit `search.semantic.enabled` for the
out-of-the-box hybrid experience, or set it `false` to opt back into
lexical-only). The vector backend is `turbovec` by default; set
`embedding.vectorBackend` to `faiss` **before the first index is built** to
opt into FAISS (`faiss-cpu` is installed on that path only). The embedding
model is the FastEmbed `BAAI/bge-small-en-v1.5` by default; override via
`embedding.model` to use a different model (the sidecar rebuilds the index
when the model changes). The vector dimension is derived from the model at
runtime — it is not configured. The Python sidecar is **auto-provisioned on
demand** (the daemon resolves a Python interpreter and creates a pinned
isolated environment under XDG state via `uv` with a `python -m venv` +
`pip` fallback, then launches the sidecar over stdio). If Python or the
sidecar is unavailable, the daemon continues to serve lexical-only `findTool`
results and surfaces the degraded mode rather than failing.

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

## Using Ozy as an MCP server

Ozy can be configured as an MCP server in opencode or any MCP-compatible agent
harness. An agent that connects to Ozy sees exactly three tools (`findTool`,
`describeTool`, `callTool`) and can discover all downstream tools by calling
`findTool` — no `ozy index` required beforehand.

### Minimal opencode configuration

Add Ozy to your opencode `mcp.json` (or equivalent MCP client config):

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

Ozy reads its own downstream server list from `~/.config/ozy/ozy.jsonc` (or
`$OZY_CONFIG`). On startup `ozy mcp` loads this config once; when an agent
calls `findTool`, Ozy connects to every enabled downstream server, calls
`tools/list`, and returns the complete live-discovered tool list as
`choose_from_candidates` with stable `toolRef`s (e.g.
`atlassian.confluence_search`), each carrying `title`, `description`, and
`inputSchema`. The end-to-end flow is: the agent calls `findTool` first,
picks a candidate's `toolRef`, and then calls `callTool` with that
`toolRef` and the arguments described in the candidate's `inputSchema`.
`callTool` resolves the `toolRef` against `ozy.jsonc`, connects to that
single downstream server, and runs `tools/call` — no `ozy index` required.

`describeTool` remains catalog-backed by design and is **not** part of the
live flow: it serves indexed tools from the persisted catalog and returns
`TOOL_NOT_FOUND` for tools that have only been discovered live. The agent
should use the `inputSchema` returned in the `findTool` candidate as the
authoritative schema when planning a call.

### Quick start

```bash
# 1. Scaffold a config (one-time)
ozy init

# 2. Edit ~/.config/ozy/ozy.jsonc to declare your downstream MCP servers
#    (copy-paste from your existing opencode mcp.json entries)

# 3. Verify configuration
ozy doctor

# 4. Run the MCP adapter (open-code or other harness connects to this)
ozy mcp
```

All three Ozy tools are advertised immediately; `findTool` discovers downstream
tools live without requiring `ozy index`.

## Agent interface

Ozy exposes exactly three stable MCP tools (SPEC.md §9):

- `findTool` — find the best known tool for a capability query (returns a
  decision, not just a list);
- `describeTool` — return the exact schema and usage guidance for one tool;
- `callTool` — invoke a downstream tool through Ozy. It performs live
  brokered invocation: one configured downstream server is contacted per
  call, the result is normalized to the `SPEC.md` §9.3 envelope, and
  `budgets.callTool.maxResultBytes` bounds the returned result.

The CLI mirrors these operations through the same in-process broker, so the CLI
and MCP paths cannot drift.
