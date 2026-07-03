## Context

Ozy's runtime (`internal/daemon`) is not a process — it is an in-process object every command rebuilds via `cli.app.load()`. There is no socket, no persistent daemon, no idle logic. `ozy daemon` calls `daemon.Run()` (provision sidecar + conditional index, then block on `<-ctx.Done()`) but serves no transport. `ozy mcp` builds its own runtime with a **nil semantic adapter** and serves lexical-only — it never calls `provisionSidecar` or `Run`, so semantic search is unreachable through the agent's primary surface. `ozy index` already provisions and embeds (since `cf52351`).

The agent harness already owns `ozy mcp` as a long-lived child process for the whole session. That process lifecycle is the lifecycle we want — we just are not using it. This change binds provisioning, indexing, and the sidecar to the MCP connection so configuring `ozy mcp` is the only step a user ever takes.

Constraints: stdout carries the JSON-RPC stream (logs/diagnostics must never go there); the MCP `initialize` handshake can time out on a slow startup; a first-ever run pays a cold embedding-model download (seconds to minutes).

## Goals / Non-Goals

**Goals:**
- Configuring `ozy mcp` is the only action a user takes; indexing, embedding, and the sidecar happen automatically and are invisible.
- Semantic works through **every** entry point with no setup: `ozy mcp` and broker-backed CLI commands (`ozy search`) both provision and index on their own — no prior `ozy index`, no daemon, no flag.
- `ozy mcp` serves hybrid semantic `findTool` once the sidecar is ready, degrading to lexical otherwise.
- The sidecar starts with the MCP connection and dies with it — no orphan, no idle timer, no socket.
- Logs land in `<config-dir>/logs/`, structured and actionable.
- Partial/stale embeds are loud, in `doctor` and at index time.
- Remove `ozy daemon`.

**Non-Goals:**
- Keeping a sidecar **warm across separate CLI invocations** via a socket-backed daemon. This is a latency optimization, not correctness — CLI semantic search itself is a Goal (provision-on-demand). Deferred because the model is disk-cached, so on-demand startup is a few-second in-memory load; the `Broker` interface remains the seam if warm-sharing is ever wanted.
- Changing the sidecar protocol, embedding model, or search ranking.
- Log rotation/retention policy (append-only file now; revisit if size bites).

## Decisions

### 1. Bind the sidecar to the MCP connection, not a daemon

`ozy mcp` runs the existing startup sequence (`provisionSidecar` + conditional index from `daemon.Run`), keeps the sidecar warm for the connection, and tears it down on EOF/signal. The agent process owns the lifecycle, so "start with the agent, stop after" is free. Refactor `daemon.Run`'s startup body into a reusable `Daemon.Start(ctx, log)` that both the (now internal) run path and `mcp` call; `mcp` then serves instead of blocking on `<-ctx.Done()`.

*Alternative rejected — real socket daemon + idle timer:* more code (listener, client, single-instance lock, idle shutdown) for a workflow (CLI semantic search with no agent) that does not exist yet.

### 2. Background provisioning, not blocking the handshake — RESOLVED

At MCP startup, serve immediately and run provision+index in a background goroutine; `findTool` serves lexical until the sidecar is ready, then upgrades. **Chosen over blocking** because the zero-touch requirement includes the first-ever run, where a cold model download would otherwise stall the `initialize` handshake and break "it just works." A warm start completes in seconds regardless.

*Alternative rejected — block before serving (~5 lines):* simplest, and fine on warm starts, but fails the cold first-run case, which is exactly the new-user path this change targets.

### 3. Swappable broker so the background swap is visible

The adapter captures `d.Broker()` once at construction (`commands.go:64`), so a background `reWireBroker()` would not be seen. Guard the daemon's broker field with an `atomic.Pointer[broker.Broker]`; `Daemon.Broker()` loads it; the adapter calls `Broker()` per request through a tiny `BrokerProvider` interface (`Broker() broker.Broker`). `reWireBroker` stores the semantic-wired broker atomically when the sidecar becomes ready. No locks on the hot path, swap is race-free and visible.

*Alternative rejected — pass a `func() broker.Broker` accessor:* equivalent, but a named provider interface reads better and matches the existing `Broker()` method.

### 4. Logging: stdlib `slog`, JSON handler, file beside the config

Use `log/slog` (stdlib, no dependency) with a JSON handler writing to `<dir(configPath)>/logs/ozy.log`. The config dir is already known at `load()` time. Logs never go to stdout (reserved for JSON-RPC); a concise human line may still go to stderr. Reuse the existing secret `scrub` for any field carrying downstream detail. If the log dir is not writable, fall back to stderr-only and continue. Lifecycle/degradation events (provision, index, ready, degrade, partial-embed, shutdown) become `slog` calls with structured fields and an `action` field for remediation.

*Alternative rejected — a logging library (zerolog/zap) or lumberjack rotation:* new dependencies for what `slog` covers; rotation is a non-goal for now.

### 5. Coverage honesty: undercount is failure

Change the index loud-fail guard (`index.go:224`) from `VectorCount == 0` to `VectorCount < ToolsIndexed` (when semantic enabled and sink available), with a count-naming message. `doctor` cross-checks the same: compare the embedding probe's vector/tool counts to the catalog count and emit a WARN when vectors are short, instead of two independent OK checks.

### 6. Remove `ozy daemon` outright

Drop `daemonCmd` and its registration. It is dev-stage (`0.1.0-dev`) with no stability promise; a deprecation alias is not worth the surface. Update README/quickstart to a single `ozy mcp` step.

### 7. CLI semantic via provision-on-demand, same path as the adapter

`ozy search` runs the same `Daemon.Start` sequence (provision sidecar when semantic is enabled + conditional index) before querying, then releases the sidecar on exit — exactly what `ozy index` already does, reused. The shared `Start` path means MCP and CLI cannot diverge: both get auto-index + auto-embed + hybrid ranking with no setup. Catalog-only commands (`list`, `describe`, `call`) keep using plain `load()` and do **not** provision, so they pay no sidecar cost.

Trade-off: each `ozy search` pays sidecar spawn + in-memory model load (seconds; the model is disk-cached after first run) and a conditional index when the catalog is stale. Accepted: correctness-out-of-the-box over per-call latency, matching the user's stated priority. The warm-sharing daemon (Non-Goal) is the latency fix if this ever bites.

*Alternative rejected — leave CLI `search` lexical-only:* the original framing; it breaks "semantic works everywhere with no setup" and surprises a CLI user with silently worse results than the agent gets.

## Risks / Trade-offs

- **Background swap races with in-flight `findTool`.** → `atomic.Pointer` swap; a call either sees the old (lexical) or new (semantic) broker, both valid. No partial state.
- **Cold model download still fails / never readies.** → Session stays lexical and logs the cause with an action; no failure surfaced to the agent. Same graceful-degradation contract as today.
- **First `findTool` calls return lexical before warm-up finishes.** → Acceptable and surfaced; the alternative (blocking) is worse. Warm starts make this a non-issue.
- **Log file unbounded growth.** → Append-only for now (non-goal); revisit with rotation if it matters. `// ponytail: append-only log, add rotation if size bites`.
- **Config dir not writable (read-only mount, odd `--config`).** → Fall back to stderr-only logging; never block startup on logging.
- **Two callers of the startup sequence** (internal run path + mcp). → Extract one `Start` method so they cannot drift; covered by existing daemon startup tests plus an mcp self-provision test.

## Migration Plan

1. Extract `Daemon.Start`; make `mcp` call it, serve, and shut down the sidecar on exit.
2. Add the atomic broker pointer + `BrokerProvider`; adapter reads per request.
3. Move provision+index into a background goroutine in the mcp path; keep the warm path fast.
4. Add `slog` wiring to `<config-dir>/logs/ozy.log`; convert lifecycle/degradation notices to structured logs.
5. Tighten the index guard and `doctor` cross-check.
6. Remove `daemonCmd`; update README/quickstart.

Rollback: re-register `daemonCmd` and revert the mcp path to lexical-only serving; the runtime, sidecar, and index code are unchanged underneath.

## Open Questions

- Should `findTool`'s existing "semantic unavailable — run `ozy index`" note be reworded for the warming case (provisioning in progress) versus the truly-degraded case (provisioning failed)? Leaning yes — distinguish "warming up" from "unavailable" so the agent does not chase a remediation that is already happening.
