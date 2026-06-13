## Why

Ozy's promise is a persistent, searchable capability catalog that an agent
queries by intent instead of loading every downstream tool into context
(`SPEC.md` §3, §4.4, §10). Today the catalog is only populated by an explicit
`ozy index`, and `findTool` does **live discovery** on every call: it connects to
all enabled servers, runs `tools/list`, and returns every tool as
`choose_from_candidates` with **no ranking** (the skeleton states "ranking is not
implemented"). The agent gets the whole tool universe back, which is the exact
context-bloat outcome Ozy exists to prevent, and discovery breaks whenever a
downstream server is momentarily offline. This change makes the catalog populate
itself on startup and gives `findTool` real hybrid-search ranking so it returns a
single best tool plus one runner-up rather than a dump.

## What Changes

- Index downstream tools into the persistent catalog **on daemon startup**, but
  only when the catalog is stale — when no prior index exists, or the last index
  time predates the configuration file's (`ozy.jsonc`) modification time.
  Indexing failures degrade gracefully: the daemon still serves whatever catalog
  exists and surfaces the indexing status rather than blocking startup.
- **BREAKING (behavioral):** `findTool` moves from live all-server discovery to
  **hybrid search over the persistent catalog**. It combines a mandatory lexical
  signal with an optional semantic signal into one explainable ranking, then
  returns the two best tools — a high-certainty top match (`decision: use`,
  `selected`) and a single runner-up (`alternatives[0]`) — each with a
  `confidence` and a `reason` saying why it matched. Low-separation or
  below-floor results map to `ambiguous` / `no_good_match`; an unpopulated
  catalog maps to `catalog_empty`.
- Semantic search remains optional and **degrades to lexical-only** when disabled
  or when no embedding provider is available; the degraded mode is surfaced
  explicitly and never hard-fails (`SPEC.md` §4.10, §10.1).
- `findTool` searches the **catalog** (stale-tolerant), so discovery keeps
  working when a downstream server is temporarily offline; invocation stays
  live-gated.
- `describeTool` continues to resolve **exactly** by `toolRef` against the
  catalog, now reliably populated by startup indexing (`SPEC.md` §9.2). No
  contract change — this satisfies the "exact search" criterion as-is.
- `callTool` is unchanged: live brokered invocation already shipped in
  `mcp-calltool-live-invocation`. It is exercised here only by an end-to-end
  acceptance test of the full loop.

## Capabilities

### New Capabilities
- `tool-search`: Hybrid (lexical + optional semantic) ranking of cataloged tools
  for a capability query, the `findTool` decision model (top match + one runner-up,
  with `confidence`, `reason`, and an explicit decision), confidence-to-decision
  mapping, graceful semantic→lexical degradation, and catalog-backed
  (stale-tolerant) retrieval independent of live downstream availability.

### Modified Capabilities
- `daemon-runtime`: The daemon SHALL run a conditional index on startup, gated on
  catalog staleness relative to the configuration file's modification time, and
  SHALL degrade gracefully (serve the existing catalog, surface status) when
  startup indexing fails or no server is reachable.
- `catalog-persistence`: The catalog SHALL record the timestamp of the last
  successful index run and expose it, so startup can compute staleness against the
  configuration file's modification time.

## Impact

- Affected code:
  - `internal/search` (**new**): hybrid ranker over `catalog.Store` — lexical
    scorer over the indexed fields (`SPEC.md` §10.2), a semantic-scorer seam with
    an "unavailable" default, score fusion, and the confidence/decision mapping.
  - `internal/broker`: `live.FindTool` stops doing live discovery and delegates to
    the search engine over the catalog; `describeTool` (exact catalog lookup) and
    `callTool` (live) are unchanged.
  - `internal/daemon`: on `Run`, compute staleness (`config.Loaded.Path` mtime vs.
    catalog last-index time) and conditionally invoke the indexer before reporting
    ready; wire the search engine into the broker.
  - `internal/catalog`: add a last-index-time marker to the `Store` interface and
    its file/in-memory implementations.
  - `internal/index`: reused as-is for the indexing run; record the index-run
    timestamp on success.
  - `internal/contract`: reuse the existing `FindResult` fields
    (`selected`, `confidence`, `reason`, `alternatives`, `nextAction`); no new
    types expected.
- Affected behavior:
  - An agent runs `findTool` and gets a ranked decision (best + runner-up) from
    the catalog rather than an unranked live dump, then `describeTool` → `callTool`.
  - First run with an empty/stale catalog indexes automatically; subsequent runs
    skip indexing until config changes.
  - Discovery works while a downstream server is offline; invocation still verifies
    live reachability.
- Dependencies: no new external dependency. The Python embedding worker
  (`SPEC.md` §10.4) is **out of scope**; the semantic seam ships with an
  unavailable default so the runtime stays lexical-only until a provider lands.
