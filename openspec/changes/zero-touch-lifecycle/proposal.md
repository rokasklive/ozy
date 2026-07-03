## Why

Ozy's primary interface — the MCP server an agent harness launches — serves **lexical-only** today: `ozy mcp` never provisions the embedding sidecar and never indexes, so semantic search is unreachable through the very surface agents use. Making it work currently requires a user to discover and run extra CLI commands (`ozy daemon`, `ozy index`) and to understand the Python embedding sidecar — operational burden that should not exist. A user should configure ozy once in their harness, start the agent, and have indexing, embedding, and serving happen automatically, with no CLI babysitting and no awareness of a sidecar.

## What Changes

- **`ozy mcp` self-bootstraps.** On startup it provisions the embedding sidecar and runs a conditional index (the sequence that already exists in `daemon.Run`), wires the hybrid semantic broker, keeps the sidecar warm for the life of the MCP stdio connection, and tears it down on disconnect. Semantic search works through the MCP surface with zero manual steps.
- **Lifecycle is bound to the agent's own process.** Because the harness owns `ozy mcp` as a child process, "starts with the agent, stops when the agent stops" comes for free — no idle timer, no socket, no background daemon to manage.
- **BREAKING: remove the user-facing `ozy daemon` command.** Its only job (warm sidecar + startup index) now belongs to `mcp`. The README, which currently tells users to run `ozy daemon` and `ozy mcp` as two separate processes, is corrected.
- **`ozy index` and `ozy doctor` become optional diagnostics, not setup steps.** Normal operation never requires them. `index` remains a manual reindex escape hatch; `doctor` remains for troubleshooting.
- **CLI semantic works out of the box too.** Broker-backed CLI commands that rank tools — notably `ozy search` — provision the sidecar on demand and run a conditional index automatically, returning hybrid semantic results with no prior `ozy index` and no daemon, identical to the MCP surface. (`list`, `describe`, and `call` are catalog/live-only and need no sidecar.)
- **The embedding sidecar becomes invisible to users.** Auto-provisioned, auto-managed, lifetime-bound to the serving process. No command, flag, or concept the user must learn.
- **Logs move next to the config.** Ozy writes descriptive, agent-ergonomic logs to a `logs/` directory in the same folder as `ozy.jsonc` (e.g. `~/.config/ozy/logs/`). Lines name the cause and the next action, not bare errors.
- **Honest readiness signals.** Startup and `doctor` warn when the queryable vector count is below the catalog tool count (a partial or stale embed), closing the silent gap where the loud-fail guard fires only on exactly zero vectors.

## Capabilities

### New Capabilities
- `runtime-logging`: where ozy writes operational logs (a `logs/` directory beside `ozy.jsonc`), the structured agent-ergonomic line format, and which lifecycle/degradation events must be logged.

### Modified Capabilities
- `mcp-adapter`: the MCP server must provision the sidecar and run a conditional index on startup, serve hybrid semantic search (not lexical-only), keep the sidecar warm for the connection lifetime and shut it down on disconnect, and announce readiness without blocking the MCP initialize handshake.
- `cli-interface`: remove the `daemon` command; `index` and `doctor` are optional rather than required for normal use; `doctor` cross-checks the vector count against the catalog tool count and warns on drift.
- `daemon-runtime`: the runtime startup sequence (provision + conditional index) is no longer exposed as a standalone user-run command; it is invoked by the serving adapter and bound to that process's lifetime.
- `embedding-sidecar`: the sidecar is auto-provisioned by the serving process with its lifetime bound to that process, never surfaces as a user-facing concern, and reports a partial embed (vectors < catalog tools) rather than silently accepting it.

## Impact

- **Code**: `internal/cli/commands.go` (mcp self-bootstrap; remove `daemonCmd`), `internal/cli/cli.go` (command registration), `internal/daemon/daemon.go` (expose the startup sequence to the adapter; broker handle so a background re-wire is visible), `internal/mcp/adapter.go` (read the broker per request instead of capturing it at construction), `internal/index/index.go` (partial-embed guard), `internal/cli/doctor.go` (vector-vs-catalog cross-check), and new logging wiring writing to the config-dir `logs/` path.
- **Surface / BREAKING**: `ozy daemon` removed. README and quickstart updated to a single configure-and-go step.
- **Behavior**: an agent that has `ozy mcp` configured gets semantic search automatically on next start; the first-ever run pays a one-time cold embedding-model download.
- **Dependencies**: none added.
- **Deferred (performance only, not correctness)**: a socket-backed daemon that keeps one sidecar *warm* across separate CLI invocations. CLI semantic search itself is in scope via provision-on-demand above; only the cross-invocation speedup is deferred — the embedding model is cached on disk, so on-demand startup is a few-second in-memory load, not a re-download. The `Broker` interface remains the seam if warm-sharing is ever wanted.
