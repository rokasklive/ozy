## Why

The skeleton wired every seam but nothing populates the catalog, so `findTool`,
`describeTool`, `list`, and `index` all bottom out in empty/`NOT_IMPLEMENTED`
stubs. Ozy cannot yet do the one thing it exists for: know what downstream tools
exist. This change makes the catalog real.

**North star:** a user configures all of their downstream MCP servers in a single
`ozy.jsonc` / `ozy.json` file (using the same `mcp` shape as opencode), runs
`ozy index`, and Ozy connects to each server, discovers its tools, and persists
them into a durable catalog — so `ozy list` and `ozy describe <toolRef>` return
real results, even after a restart or while a server is offline.

## What Changes

- **Config format → `ozy.jsonc` / `ozy.json`.** Replace the skeleton's YAML
  config with a JSONC/JSON file. Downstream servers are declared under an `mcp`
  key following opencode's shape: each entry has `type` (`local` | `remote`),
  and for `local` a `command` array + `environment` map, for `remote` a `url` +
  `headers` map, plus `enabled`. Ozy's own sections (`search`, `embedding`,
  `budgets`) remain as sibling keys. `ozy init` scaffolds `ozy.jsonc`.
  **BREAKING** vs. the skeleton's YAML model (greenfield — no released users).
- **Downstream connection layer.** Connect to each enabled server through the
  MCP Go SDK client: `local` via a command/stdio transport, `remote` via a
  streamable-HTTP transport with configured headers. Per-server connect/health
  with structured, non-leaky errors; one unreachable server never aborts the
  whole index.
- **Tool discovery / indexing.** Implement `ozy index`: for each reachable
  server call `tools/list`, normalize each tool into a stable
  `<serverId>.<toolName>` `toolRef` with title, description, input schema, and
  freshness (SPEC.md §7, §8), and write it to the catalog.
- **Persistent catalog.** Replace the in-memory `catalog.Store` placeholder with
  a durable local store so discovered tools survive process restarts and support
  offline `list`/`describe` (SPEC.md §4.4, §4.6, §5.1). `describeTool` and `list`
  become real for free once the store is populated.
- **Richer `doctor`.** Report per-server reachability and indexed-tool counts
  (SPEC.md §17).

Out of scope (explicit non-goals, deferred to later changes): lexical/semantic
search **ranking** for `findTool` (it flips from `catalog_empty` →
`no_good_match` here and gets real ranking next); brokered **invocation**
(`callTool` stays live-gated and unimplemented); scheduled/auto refresh and
schema-drift detection; the eval harness; tool-list-change notifications.

Note: this amends the configuration model in `SPEC.md` §11 (illustrative YAML →
JSONC with the opencode `mcp` shape). `SPEC.md` §11 should be updated when this
change is accepted, per the governance rules in §18.

## Capabilities

### New Capabilities
- `downstream-connection`: connect to configured downstream MCP servers (local
  stdio and remote HTTP), manage per-server lifecycle/health, and surface
  structured connection errors.
- `tool-discovery`: discover downstream tools via `tools/list`, normalize them to
  stable `toolRef`s with schema and freshness, and drive `ozy index` to populate
  the catalog.
- `catalog-persistence`: a durable local catalog store that survives restarts and
  serves `list`/`describe` offline when servers are unreachable.

### Modified Capabilities
- `configuration`: consume `ozy.jsonc` / `ozy.json` using opencode's `mcp` shape
  (`type` local/remote, `command`/`environment`, `url`/`headers`, `enabled`);
  update discovery, validation, environment-reference resolution, and redaction
  for the new model.

## Impact

- New code: `internal/downstream/` (MCP client connector), `internal/index/`
  (discovery/indexing), a durable `catalog.Store` implementation.
- Changed code: `internal/config/` (JSONC + opencode shape, replaces YAML model),
  `internal/cli/` (`index` becomes real; `doctor` gains server health), `internal/daemon/`
  (constructs the persistent store), `internal/broker/` (`FindTool` empty→no-match
  transition; `List`/`DescribeTool` now return real data).
- New dependencies: a JSONC parser (e.g. `tailscale/hujson`) and a durable store
  backend (selected in design).
- Affected contracts: `findTool`, `describeTool`, `callTool` response shapes are
  preserved (SPEC.md §9); their *content* becomes real. The configuration model
  (SPEC.md §11) changes.
- New main specs: `downstream-connection`, `tool-discovery`, `catalog-persistence`;
  modified main spec: `configuration`.

## Acceptance Note

When this change is accepted, update `SPEC.md` §11 from the illustrative YAML
model to the JSONC/opencode `mcp` model implemented here.
