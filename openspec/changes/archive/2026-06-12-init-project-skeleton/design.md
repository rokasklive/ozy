## Context

Ozy is a greenfield project defined only by `SPEC.md`. This change establishes the buildable skeleton so later OpenSpec changes can implement real broker behavior. The accepted architectural baseline (`SPEC.md` §20) fixes several constraints up front: Go owns the trusted runtime, daemon, MCP adapter, CLI, broker, and catalog authority; lexical search is the mandatory baseline; semantic search and the Python embedding worker are optional; behavior must be local-first and degrade gracefully when optional subsystems are absent.

The skeleton must do three things well: (1) compile to a single `ozy` binary, (2) make the architectural seams real (config, broker, catalog, CLI adapter, MCP adapter) so they can be filled in independently, and (3) make the agent-facing surface (`findTool`/`describeTool`/`callTool`) already conform to the §9 response contracts even though behavior is stubbed. Everything else (real search/index, downstream connection, eval harness, drift detection, ContextSpy) is explicitly deferred.

## Goals / Non-Goals

**Goals:**
- A single `ozy` binary with the full MVP command surface wired (`SPEC.md` §15).
- One in-process **broker interface** that both the CLI and MCP adapter depend on, guaranteeing adapter parity (§4.9, §14.1) by construction.
- Typed result/error models that serialize to the §9 response shapes, including the `agentInstruction`, `decision`, `nextAction`, and structured-error fields.
- Configuration loading/validation/redaction per §11 with `${ENV}` resolution.
- A catalog store **interface** with a working in-memory placeholder that handles the empty-catalog state instructionally.
- Build/test/lint tooling and CI so the quality gate exists from day one.

**Non-Goals:**
- Real lexical or semantic search ranking (stub returns `catalog_empty` / `no_good_match`).
- Real downstream MCP connection and brokered invocation (`callTool` returns a structured not-yet-callable failure).
- A separate daemon process with cross-process IPC — the skeleton runs the broker in-process.
- Persistent catalog storage engine, embedding worker, eval harness, schema-drift detection, ContextSpy.
- Defining concrete token-economy targets (deferred until baselines are measured, per §13).

## Decisions

### D1: CLI framework — `spf13/cobra`
Cobra is the de-facto Go CLI standard, gives a clean command tree, per-command help, and flag inheritance for a global `--format` and `--config`. **Alternatives:** `urfave/cli` (lighter but less idiomatic for large command trees); stdlib `flag` (too low-level for ten subcommands with shared flags). Cobra's structure maps cleanly to the §15 command list.

### D2: MCP server library — official `modelcontextprotocol/go-sdk`, hidden behind an internal adapter
We use the official Go MCP SDK to register the three tools and serve the protocol, but wrap it behind `internal/mcp` so the SDK is swappable. **Alternatives:** `mark3labs/mcp-go` (popular community SDK — viable fallback if the official SDK is immature); hand-rolled JSON-RPC (unjustified cost). The wrapper keeps tool definitions and response marshalling in our code so a library swap does not touch the broker.

### D3: One broker interface shared by both adapters
`internal/broker.Broker` exposes `FindTool`, `DescribeTool`, `CallTool` returning typed result structs (`FindResult`, `DescribeResult`, `CallResult`). The CLI and MCP adapter both depend only on this interface; neither contains broker logic. This makes adapter drift (an anti-pattern in §22) structurally impossible and gives later changes a single place to implement behavior. **Alternative:** separate CLI and MCP implementations sharing helpers — rejected because it invites divergence.

### D4: In-process broker now, client/server split deferred
`SPEC.md` §6.2 shows `CLI -> daemon -> downstream` and `MCP -> daemon -> downstream`. For the skeleton we construct the broker in-process for every entrypoint (including `ozy daemon`, which simply hosts a long-running instance), avoiding premature IPC. The `Broker` interface is the seam that lets a future change introduce a daemon client/server transport without changing callers. **Alternative:** build the daemon IPC now — rejected as scope creep with no behavior to serve yet.

### D5: Typed results that marshal to §9 contracts
Result and error structs carry the contract fields (`decision`, `confidence`, `nextAction`, `agentInstruction`, `ok`, `error.type`, `error.retryable`). JSON tags produce the §9 shapes directly; the human/concise renderers format the same structs. Error types are constants from §9.3 (`TOOL_NOT_FOUND`, `DOWNSTREAM_SERVER_OFFLINE`, …) plus a skeleton-only `NOT_IMPLEMENTED` marker. This freezes the contract surface now so later behavior changes are refinements, not breaking changes.

### D6: Catalog store as an interface with an in-memory placeholder
`internal/catalog.Store` defines the operations over servers/tools/freshness/runtime status (§8). The skeleton ships an in-memory implementation; a durable local store (e.g. SQLite or bbolt) is a later decision. The empty store drives the `catalog_empty` decision path so the instructional empty-state is exercised from the start. **Alternative:** pick the storage engine now — deferred to keep this change about seams, not persistence.

### D7: Config via `gopkg.in/yaml.v3` with a typed model + redaction
A typed config struct mirrors §11. `${ENV}` references resolve at load; unresolved references become structured diagnostics naming the variable and owning server. A `Redacted()` view replaces secret-sourced values with the env-reference name or a mask for `doctor`/logging. **Alternative:** `goccy/go-yaml` (faster, richer errors) — `yaml.v3` chosen for ubiquity; can swap later if error quality matters.

### D8: Output rendering layer
A small `render` package takes a typed result and a `--format` value (`human` default, `json`, `concise`) and writes to stdout. JSON mode emits a single document for agents/evals (§15). This keeps formatting out of the broker and adapters.

## Risks / Trade-offs

- **Official MCP Go SDK maturity is uncertain** → isolate it behind `internal/mcp`; the broker never imports the SDK, so swapping to `mark3labs/mcp-go` touches one package.
- **In-process broker diverges from the §6.2 daemon picture** → the `Broker` interface is the explicit seam for a later client/server split; document that `ozy daemon` currently hosts an in-process instance.
- **Stub responses could mislead agents into thinking capability exists** → every stub carries an explicit `NOT_IMPLEMENTED`/`catalog_empty` marker and an `agentInstruction` saying the capability is pending, satisfying the grounded-instruction criterion (§4.5).
- **Over-scoping the skeleton** → requirements and tasks are deliberately limited to wiring + contract-shaped stubs; real search/invocation are out of scope and called out as non-goals.
- **Freezing §9 shapes too early** → these shapes come straight from `SPEC.md`; treating them as contracts now is intended, and contract changes already require review per §9.

## Migration Plan

Greenfield — no migration. Rollout is a single PR that adds the module, layout, tooling, and stubs. Rollback is reverting the PR; nothing depends on Ozy yet. The change introduces the durable contracts (§9 shapes, §11 config, §15 CLI) that subsequent changes must preserve or deliberately amend through OpenSpec.

## Open Questions

- Which MCP Go SDK is the long-term choice (official vs `mark3labs/mcp-go`)? Resolve during implementation based on SDK ergonomics; the `internal/mcp` wrapper limits the blast radius either way.
- Default config file location and name (e.g. `~/.config/ozy/config.yaml` vs `./ozy.yaml`) — pick a documented default in implementation; the path-override requirement already covers overrides.
- Minimum Go version to pin in `go.mod` (target a recent stable release).
