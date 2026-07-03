# Proposal: audit-remediation

## Why

The 2026-07-03 agent-ergonomics audit found that Ozy's agent surface repeatedly promises things the serving path does not do: catalog state is presented as live truth but never updated (`callableNow: true` forever, ghost tools that can never be deleted), every `callTool` is budgeted by the 5-second *discovery* timeout so real workloads fail with `retryable: true`, the guidance channel (result summaries, truncation recovery, next actions) rides in MCP `_meta` which major clients never show the model, and the golden path costs three round-trips even for one-string-argument tools while surfacing at most two candidates justified by stopword matches ("Matched terms [for, the]"). Each gap converts directly into misplaced agent trust and then abandonment — the exact adoption problem Ozy exists to solve. The audit equally confirmed what must not regress: the typed error contract with `agentInstruction`, pre-flight argument validation, decision-shaped `findTool`, the breadcrumb, and compact single-representation responses.

## What Changes

- **Invocation gets its own clock.** `callTool` runs under a per-server `callTimeout` (default 60s) instead of `timeout` (discovery, 5s). A deadline Ozy itself imposed is reported as non-retryable with "raise callTimeout / narrow the call" guidance, so agents stop retrying deterministic failures.
- **The catalog stops lying.** Indexing reconciles: tools no longer discovered on their server are deleted; tools on unreachable or config-removed servers are marked `stale` and not callable. `callableNow`/`serverStatus`/`freshness` are derived from reconciled state, and responses carry the catalog's age so an agent can weigh trust.
- **Actionable guidance moves in-band.** Truncation recovery, cache-hit staleness stamps, and any instruction the agent must act on are appended to `content` (single short trailer); `_meta` remains a mirror for clients that read it. Truncation cuts at a structural boundary (whole array elements / line boundary) instead of mid-JSON bytes.
- **findTool gets cheaper and more credible.** When the selected tool's schema is small, the full `inputSchema` and a `recommendedCall` are inlined with `nextAction: callTool` — the describe hop becomes the exception, not the ritual. `budgets.findTool.maxResults` (scaffolded today, read by nothing) is honored as top-N alternatives. Match reasons drop zero-signal stopwords. The ambiguous branch stops instructing `describeTool` for schemas it already inlined.
- **The adapter stops swallowing and overpromising.** A `findTool` broker error returns a §9.3 error envelope instead of literal `null` with `isError: false`. The MCP `initialize` response carries server `instructions` (workflow guidance + capability summary — the one always-loaded client-blessed channel, unused today). `describeTool`'s description is trimmed to what it returns, and `recommendedCall` is actually populated.
- **Dead advertised surface is removed.** Never-emitted contract decisions (`choose_from_candidates`, `known_but_unavailable`) and error types (`TOOL_SCHEMA_CHANGED`, `RESULT_TRUNCATED`) are deleted from contract and SPEC; the README's false "findTool live-connects, no `ozy index` required" claim and the stale `ConnectAll powers FindTool` comment are corrected.
- **`ozy doctor` flags secret-shaped literals** (`ghp_`, `github_pat_`, `sk-`, `AKIA`, `xox`, `Bearer …`) in config values that do not use `{env:…}`, without printing the value.

## Capabilities

### New Capabilities

None — every change lands in an existing capability.

### Modified Capabilities

- `tool-invocation`: dedicated call timeout distinct from discovery; honest `retryable` on self-imposed deadlines; structural (parseable) truncation with in-band recovery guidance.
- `tool-search`: small-schema fast path (inline schema + recommendedCall, skip describe); `maxResults`-bounded alternatives; stopword-free match reasons; ambiguous decision self-consistent with its inlined payload.
- `mcp-adapter`: findTool broker errors surface as §9.3 envelopes; actionable guidance in `content` (in-band), `_meta` as mirror only; MCP server `instructions` at initialize; `describeTool` description matches its actual payload.
- `catalog-persistence`: tool deletion; staleness/callability as derived, reconciled state (never hardcoded fresh/callable); catalog age exposed to responses.
- `tool-discovery`: index runs reconcile the catalog — delete vanished tools, degrade tools of unreachable/removed servers — instead of upsert-only.
- `result-cache`: cache hits for brokered calls are visibly stamped (cached + age) in the result; caching stays default-on and read-only-gated.
- `configuration`: per-server `callTimeout` (default 60s) alongside the existing discovery `timeout`; `budgets.findTool.maxResults` becomes load-bearing.
- `cli-interface`: `doctor` warns on inline secret-shaped config values.

## Impact

- **Prerequisite**: the `zero-touch-lifecycle` change (recovered from `git stash@{0}`) must land first — it fixes the audit's #1 finding (MCP path is lexical-only) and this change builds on its `Daemon.Start`/broker-swap wiring and logging.
- **Code**: `internal/broker/live.go` (timeout, truncation, fast path, alternatives), `internal/mcp/adapter.go` (error envelope, trailers, instructions, descriptions), `internal/catalog/` (DeleteTools + derived status on the Store seam and both implementations), `internal/index/index.go` (reconciliation), `internal/broker/cache.go` (hit stamping), `internal/search/lexical.go` (reasons), `internal/config/config.go` + scaffold (callTimeout), `internal/cli/doctor.go` (secret scan), `internal/contract/` + `SPEC.md` + `README.md` (dead-surface removal, honesty pass).
- **Surface**: no breaking agent-visible changes — response fields are added or corrected, never renamed; removed contract constants were never emitted. Existing configs keep working (`callTimeout` defaults sensibly; `maxResults` starts being respected).
- **Out of scope (deliberate)**: downstream session pooling / persistent connections (F3 — own change; the `Connector` seam is ready), adoption telemetry reports, OAuth flow, elicitation/sampling proxying.
