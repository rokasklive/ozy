## 1. Configuration (JSONC + opencode shape)

- [x] 1.1 Add the JSONC parser dependency (`tailscale/hujson`)
- [x] 1.2 Replace the YAML config model with the JSONC model: `mcp` map (opencode shape: `type`, `command`/`environment`, `url`/`headers`, `enabled`) plus `search`/`embedding`/`budgets`
- [x] 1.3 Implement loading via hujson → `encoding/json`; accept comments and trailing commas; default-path precedence (`--config`/`OZY_CONFIG`, then `./ozy.jsonc`/`./ozy.json`, then user config dir)
- [x] 1.4 Implement `{env:NAME}` reference resolution in `environment` and `headers`; record unresolved references as structured diagnostics naming the variable and server
- [x] 1.5 Implement validation: `type` ∈ {local, remote}; `local` requires `command`; `remote` requires `url`; structured `CONFIG_ERROR` naming server and field
- [x] 1.6 Update redaction to mask resolved secrets in `headers`/`environment`, showing the `{env:NAME}` form
- [x] 1.7 Update `ozy init` to scaffold a commented `ozy.jsonc` with opencode-shaped examples
- [x] 1.8 Tests: valid JSONC load, comments/trailing commas, `{env:}` resolution, missing env diagnostic, local-without-command, remote-without-url, unknown type, redaction, init round-trip

## 2. Persistent catalog store

- [x] 2.1 Implement a durable `catalog.Store` as an atomic JSON document store (temp file + rename) at a state path (overridable); load on construction
- [x] 2.2 Ensure only tool metadata is persisted (no resolved secrets)
- [x] 2.3 Wire `daemon.New` to construct the persistent store (keeping `Memory` for tests)
- [x] 2.4 Tests: write-then-read in a fresh store instance (survives restart), empty-store reads, atomic write does not corrupt on overwrite

## 3. Downstream connection layer

- [x] 3.1 Add `internal/downstream` with a `Connector` over the MCP SDK client
- [x] 3.2 Implement `local` connections via `mcp.CommandTransport` (command + args + `environment` on the `exec.Cmd`)
- [x] 3.3 Implement `remote` connections via `mcp.StreamableClientTransport` with header injection through a custom `http.Client` RoundTripper
- [x] 3.4 Skip disabled servers; connect enabled servers concurrently with a bounded `errgroup`; return a per-server result/error without aborting the set
- [x] 3.5 Map connection failures to structured `contract.Error`s scrubbed of resolved secret values
- [x] 3.6 Tests: in-memory/mock downstream server connects and lists; one unreachable server does not abort others; disabled server skipped; connection error excludes secret header value

## 4. Tool discovery / indexing

- [x] 4.1 Add `internal/index` that calls `ListTools` per connected session
- [x] 4.2 Normalize each tool to `catalog.Tool`: `toolRef = <serverId>.<name>`, copy title/description/input schema, set `schemaHash`, `lastIndexedAt`, freshness `fresh`, server status
- [x] 4.3 Write discovered tools to the catalog store
- [x] 4.4 Implement `ozy index`: connect → discover → persist → report a structured summary (servers reached, tools indexed, per-server errors), exit non-zero only on total failure
- [x] 4.5 Tests: discovery maps a tool to the expected `toolRef` and schema; index summary counts servers/tools; no-reachable-server case is instructional

## 5. CLI, broker, and doctor wiring

- [x] 5.1 Confirm `describeTool`/`ozy describe` returns real schema/status for an indexed `toolRef` (broker already reads the store)
- [x] 5.2 Confirm `ozy list` shows indexed tools with server id and freshness
- [x] 5.3 Confirm `findTool` returns `no_good_match` (not `catalog_empty`) once the catalog is non-empty; keep `callTool` unimplemented/live-gated
- [x] 5.4 Extend `ozy doctor` to report per-server reachability and indexed-tool counts (redacted)
- [x] 5.5 Tests: describe/list against a populated store; doctor server-health output is redacted

## 6. Docs and spec note

- [x] 6.1 Update README usage/config section for `ozy.jsonc` and the opencode `mcp` shape
- [x] 6.2 Add an example `ozy.jsonc` (local + remote servers)
- [x] 6.3 Note in the change that `SPEC.md` §11 should be updated to the JSONC/opencode model on acceptance

## 7. Verification

- [x] 7.1 `make build test lint` clean
- [x] 7.2 End-to-end: configure a local test MCP server in `ozy.jsonc`, run `ozy index`, then `ozy list`/`ozy describe` in a separate process (proves persistence + offline read)
- [x] 7.3 Run `openspec validate downstream-discovery` and confirm the change satisfies its specs
