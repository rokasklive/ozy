## Context

The skeleton (`init-project-skeleton`) established the broker seam, the agent
contracts (SPEC.md §9), and an empty in-memory catalog. Every discovery path is
inert because nothing connects to downstream servers. This change delivers the
north star: configure all MCP servers in `ozy.jsonc` (opencode `mcp` shape),
`ozy index` connects and discovers their tools, and a durable catalog makes
`list`/`describe` real and offline-capable.

The MCP Go SDK (already a dependency) provides the client side: a
`CommandTransport` for local stdio servers and a `StreamableClientTransport` for
remote HTTP servers, then `ClientSession.ListTools`. The work is wiring config →
connect → discover → persist, with per-server isolation and no secret leakage.

## Goals / Non-Goals

**Goals:**
- Replace the YAML config with `ozy.jsonc`/`ozy.json` using opencode's `mcp` shape.
- Connect to enabled `local` and `remote` servers, isolated per server.
- `ozy index`: discover tools, normalize to stable `toolRef`s, persist them.
- A durable, inspectable catalog store that survives restarts and serves
  `list`/`describe` offline.
- `doctor` reports per-server reachability and tool counts.

**Non-Goals:**
- `findTool` ranking (lexical/semantic search) — next change; here `findTool`
  only transitions `catalog_empty` → `no_good_match`.
- Brokered invocation (`callTool` stays unimplemented and live-gated).
- Scheduled/auto refresh, tool-list-change notifications, schema-drift detection.
- Eval harness; semantic embeddings.

## Decisions

### D1: JSONC config via `tailscale/hujson`, replacing YAML
Parse `ozy.jsonc`/`ozy.json` by standardizing with `hujson` (strips comments and
trailing commas) then `encoding/json` into the typed model. Default-path
precedence: `--config`/`OZY_CONFIG`, else project-local `./ozy.jsonc` then
`./ozy.json`, else `$XDG_CONFIG_HOME/ozy/ozy.jsonc`. **Alternatives:** keep YAML
(rejected — user wants opencode-compatible JSONC); `muhammadmuzzammil1998/jsonc`
(hujson is the better-maintained, Tailscale-proven choice). Greenfield, so the
YAML model is removed rather than dual-supported.

### D2: opencode `mcp` shape → transport mapping
Config model:
```jsonc
{
  "$schema": "https://ozy.dev/config.json",
  "mcp": {
    "atlassian": { "type": "remote", "url": "https://...", "headers": { "Authorization": "Bearer {env:ATLASSIAN_MCP_TOKEN}" }, "enabled": true },
    "filesystem": { "type": "local", "command": ["filesystem-mcp", "--root", "."], "environment": { "FOO": "{env:FOO}" }, "enabled": true }
  },
  "search":   { "lexical": { "enabled": true }, "semantic": { "enabled": false } },
  "embedding": { "provider": "python-local", "required": false },
  "budgets":  { "findTool": { "maxResults": 5 }, "callTool": { "maxResultBytes": 65536 } }
}
```
`type: local` → `&mcp.CommandTransport{Command: exec.Command(command[0], command[1:]...)}` with `environment` set on the `exec.Cmd`. `type: remote` → `&mcp.StreamableClientTransport{Endpoint: url, HTTPClient: <client injecting headers>}`. **Alternative:** invent a richer transport enum — rejected; matching opencode keeps configs portable.

### D3: Environment references use opencode `{env:NAME}` syntax
Resolve `{env:NAME}` in `environment` and `headers` values at load (replacing the
skeleton's `${VAR}`), since we are adopting opencode's shape. Missing references
become structured diagnostics; redaction shows the `{env:NAME}` form. **Alternative:**
keep `${VAR}` — rejected for opencode fidelity. (Flagged as a confirmable choice
in Open Questions.)

### D4: `internal/downstream` connector
A `downstream.Connector` opens a session per server via the SDK client. Remote
header injection uses an `http.Client` with a header-adding `RoundTripper`.
Connections run concurrently with a bounded `errgroup`; each server yields a
`(session, error)` independently so one failure never aborts the set. Errors are
mapped to structured `contract.Error`s (`DOWNSTREAM_SERVER_OFFLINE`,
`AUTH_UNAVAILABLE`, `CONFIG_ERROR`) and scrubbed of resolved secrets.

### D5: `internal/index` discovery
For each connected session, call `ListTools`, then map each tool to a
`catalog.Tool`: `toolRef = serverId + "." + tool.Name`, copying title,
description, and input schema; `schemaHash = sha256(canonical(inputSchema))`;
`freshness = fresh`; `lastIndexedAt = now`; `serverStatus = online`. `ozy index`
aggregates a per-server summary (reached, tool count, errors).

### D6: Persistence as an atomic JSON document store
Implement `catalog.Store` as a JSON file at `$XDG_STATE_HOME/ozy/catalog.json`
(overridable), written atomically (temp file + `os.Rename`) and loaded on
construction. **Rationale:** inspectable (SPEC.md §4.11), pure-Go (no cgo,
cross-platform per §11), and ample for a modest catalog; `index` is the sole
writer. **Alternatives:** `bbolt` (pure-Go embedded KV — adopt if concurrent
writers or large catalogs appear) or `modernc.org/sqlite` (pure-Go but heavier).
The in-memory `Memory` store stays for tests. The store persists only tool
metadata — never resolved secrets (D2 values live only in the config model).

### D7: Per-server isolation and live-gating preserved
`index` and `doctor` treat each server independently and report structured
per-server outcomes. Persistence enables offline `list`/`describe`, but
`describeTool` marks cached entries with their freshness and does not assert live
callability from cache alone; `callTool` remains unimplemented and live-gated
(unchanged this change), preserving SPEC.md §4.6.

## Risks / Trade-offs

- **JSON-file store has a single-writer assumption** → `index` is the only writer;
  atomic rename avoids torn reads. Revisit with `bbolt` if concurrent indexing is
  introduced.
- **Spawning local server processes from config is powerful** → only enabled
  entries from the user's own config are launched; environment is scrubbed from
  errors; document that `command` runs arbitrary local executables.
- **opencode shape may drift** → we pin to the documented local/remote shape and
  isolate parsing in `internal/config`; a shape change touches one package.
- **Remote header injection correctness** → cover with a test server asserting the
  resolved header arrives; never log resolved headers.
- **Config format is BREAKING vs. the skeleton YAML** → greenfield, no released
  users; `ozy init` writes the new `ozy.jsonc` and the README/example are updated.

## Migration Plan

Greenfield. Remove the YAML config model and `config.yaml` scaffold; `ozy init`
now writes `ozy.jsonc` with a commented opencode-shaped example. No data
migration. Rollback = revert the change; nothing external depends on the catalog
yet. On acceptance, update `SPEC.md` §11 to show the JSONC/opencode model.

## Open Questions

- Confirm environment-reference syntax: opencode `{env:NAME}` (recommended for
  shape fidelity) vs. the skeleton's `${VAR}`. Also: support opencode `{file:path}`?
  (Proposed: defer `{file:path}`.)
- Persistence backend: JSON document file (recommended) vs. `bbolt` — revisit if
  scale/concurrency demands.
- Exact config-path precedence (project-local vs. user-config-dir ordering).
- Should `ozy daemon` auto-refresh/index on start (SPEC.md §12)? Proposed: defer to
  a dedicated refresh change; `ozy index` is the explicit trigger here.
