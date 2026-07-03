# Design: audit-remediation

## Context

The 2026-07-03 audit (see `~/.claude` memory `ozy-audit-2026-07-03` and the audit report in session history) ranked Ozy's agent-surface defects. The #1 finding (MCP path is lexical-only) is fixed by landing the stashed `zero-touch-lifecycle` change — a prerequisite, not part of this change. This change addresses the remaining High/Medium findings that share one theme: the interface asserts things the runtime does not do (live-sounding catalog state, retryable-labeled deterministic failures, guidance in a channel clients drop, config knobs read by nothing), and the golden path is more expensive than it needs to be. The audit also confirmed the strengths to preserve unchanged: typed errors with `agentInstruction`, pre-flight arg validation, decision-shaped `findTool`, breadcrumb, compact single-representation JSON, one broker behind CLI and MCP.

## Goals / Non-Goals

**Goals:**

- Every state Ozy reports (callable, fresh, retryable, cached, truncated) is derived from something the runtime actually knows, and every instruction the agent must act on arrives in `content`.
- Cut the golden path from three calls toward two (often effectively one decision) for small-schema tools.
- Remove or de-advertise every dead knob, field, decision, and doc claim the audit found.

**Non-Goals:**

- Downstream session pooling / persistent connections (audit F3) — separate change; `Connector` is the seam.
- Adoption telemetry and eval additions beyond keeping existing suites green.
- OAuth, elicitation/sampling proxying, resource/prompt brokering.
- Semantic-on-MCP wiring — that is `zero-touch-lifecycle` (prerequisite).

## Decisions

**D1 — One dedicated clock for invocation.** `CallTool` runs entirely under a new per-server `callTimeout` (ms, default 60 000), replacing the reuse of `DiscoveryTimeout()` (5 s). One clock for connect+invoke, not two: the failure the audit measured was the *total* budget, and two nested deadlines add config surface without changing outcomes. When our own `callCtx` deadline fires, the error is `DOWNSTREAM_CALL_FAILED` with `retryable: false` and a message naming `callTimeout` — retrying a deterministic timeout is the retry-storm generator. *Alternative rejected:* global `budgets.callTool.timeoutMs` — per-server matches the existing `timeout` field's shape and servers differ wildly (npx spawn vs warm remote).

**D2 — Reconciliation is scoped to what the index run actually learned.** Per server where `tools/list` *succeeded*: cataloged tools not re-discovered are deleted (new batch `DeleteTools` on `catalog.Store`, both implementations). Servers absent from config: their tools are deleted (config is the universe; `callTool` already refuses them). Servers present but unreachable or disabled: tools are *kept* but flipped to `ServerStatus` offline/unknown, `CallableNow: false`, `Freshness: stale` — never deleted on a flake. Deletions computed by the indexer are pushed to the embedding sink via the existing `Delete`, replacing the no-op `List`-based reconciliation. *Alternative rejected:* marking vanished tools stale instead of deleting — a tool the server no longer serves is not stale, it is gone; keeping it preserves the ghost-selection failure.

**D3 — Catalog age over renamed fields.** `callableNow`/`serverStatus`/`freshness` keep their names (agent-visible renames are the real breaking change) but become derived, reconciled values per D2, and `catalogStats` gains `catalogAgeSeconds` (now − LastIndexedAt) so an agent can weigh how old "now" is. *Alternative rejected:* renaming to `callableAtLastIndex` — honest but breaks every consumer for a semantic we can instead make true.

**D4 — Guidance travels as a second `TextContent` block.** Actionable notices — truncation recovery, cache-hit stamp — are appended as one short separate text block: `[ozy] <one line>`. Separate block, not string-append, so the payload block stays byte-identical for parsers; MCP content is an array and clients render all text blocks. `_meta` keeps carrying `toolRef`/`resultSummary`/`nextActions` as a mirror for clients that read it. Nothing agents must act on lives only in `_meta` anymore.

**D5 — Truncation cuts at structure, not bytes.** Top-level JSON array over budget: drop tail elements to fit, trailer says "showing N of M items — narrow the call". String (or non-array JSON rendering): cut at the last line break (fallback: last space) under the limit, trailer names the byte budget and that the payload is partial (and, for JSON, unparseable). No continuation tokens — "narrow the call" plus an honest label is the v1 contract. *Alternative rejected:* a full JSON-aware pruner — complexity disproportionate to catalogs of small tools; the array case covers the common bulk shape.

**D6 — Fast path by schema size, constant threshold.** In a `use` decision, when the selected tool's canonical `inputSchema` JSON is ≤ 2 KiB, `findTool` inlines the full `inputSchema` and a `recommendedCall` (arguments skeleton from `required` fields) and sets `nextAction: callTool`; `agentInstruction` says to call directly and to use `describeTool` only if the schema is unclear. Larger schemas keep today's describe-first flow. The threshold is a code constant, not config — no evidence yet that anyone needs to tune it. The ambiguous branch, which already inlines both candidates' schemas, gets the same treatment: instruction becomes "compare the inlined schemas, then call", and `describeTool` is no longer prescribed for bytes already delivered.

**D7 — `maxResults` becomes the alternatives bound.** `budgets.findTool.maxResults` (default 5) caps selected + alternatives: up to `maxResults − 1` runner-ups with `toolRef` + one-line reason. Relevance floors keep gating the *decision*, not the listing — alternatives exist precisely for when the top pick is wrong. Match reasons list at most the 4 highest-IDF matched terms. *(Amended during apply: pure corpus IDF proved insufficient at ~70-tool scale — "and" at df 30/69 clears any sane relative floor — so the display additionally filters a small English function-word list plus a relative-IDF floor; scoring is untouched, and the strongest term is kept when only stopwords matched.)*

**D8 — Contract cleanup keeps one reserved error.** Delete `choose_from_candidates` and `known_but_unavailable` decisions and `RESULT_TRUNCATED` (truncation is a labeled partial success per D4/D5, not an error) from contract and SPEC §9. `TOOL_SCHEMA_CHANGED` stays — the eval corpus exercises its shape and schema-drift detection is a planned capability — but SPEC marks it explicitly "reserved: not yet emitted by the live broker". `handleFind`'s `res, _` becomes an error-envelope return, mirroring describe/call.

**D9 — MCP `instructions` at initialize.** `ozy mcp` sets `ServerOptions.Instructions`: two short paragraphs — when to reach for `findTool` (before shell exploration, for capabilities beyond built-ins) and the capability breadcrumb (server list, reusing `mcpBreadcrumb`). This is the only always-loaded, client-injected channel and it is empty today; the findTool description keeps its current text so non-instructions clients lose nothing.

**D10 — Cache hits are stamped, caching stays on.** `CallResult` gains additive `cachedAgeSeconds` (omitempty); the caching broker sets it on a shallow copy at hit time (never mutating the stored entry), and the adapter renders the D4 trailer `[ozy] cached result from Ns ago`. Read-only gating and default-on stay — the audit's problem was invisibility, not existence.

**D11 — Doctor secret scan is pattern + provenance, never value.** For each config header/environment value that does not contain `{env:`, match against a small pattern set (`ghp_`, `github_pat_`, `sk-`, `AKIA`, `xox[a-z]-`, `Bearer `); on match, WARN naming server + key + pattern name only. No entropy analysis — patterns catch the real cases (the audit found a live `ghp_` token) without false-positive noise.

## Risks / Trade-offs

- [Reconciliation deletes on a lying server: `tools/list` succeeds but returns partial results] → deletion only applies to servers whose list call succeeded, and reindex restores anything the server re-serves; vectors follow the catalog via the same run's sink deletes.
- [60 s default callTimeout can hold an agent turn on a hung server] → bounded and configurable per server; false-failure retry storms at 5 s cost more than one honest slow call.
- [Fast-path inlining grows findTool responses (~≤2 KiB)] → paid only on `use` decisions with small schemas, and it replaces a whole describeTool round-trip that costs more.
- [A second content block may render oddly in untested clients] → it is spec-conformant text; worst case it reads as a trailing line, which is the intent.
- [Deleting contract constants could break external consumers of SPEC §9] → the removed states were never emitted; consumers depending on them were depending on fiction.
- [Derived `callableNow` still goes stale between indexes] → `catalogAgeSeconds` makes the staleness itself visible; live probing per findTool stays rejected (it would reintroduce connect-the-world latency).

## Migration Plan

1. Land `zero-touch-lifecycle` from `stash@{0}` first (its own change), main green.
2. This change is additive/corrective on catalog *content*, not shape: existing `catalog.json` loads unchanged; first reindex after upgrade performs the initial reconciliation (ghost deletion happens then, not at load).
3. No config migration: `callTimeout` defaults when absent; `maxResults` already exists in scaffolds.
4. Rollback = revert; catalog entries deleted by reconciliation are restored by the next `ozy index` against live servers.

## Open Questions

None blocking. Deferred by design: session pooling (F3), adoption telemetry, continuation tokens for truncated results.
