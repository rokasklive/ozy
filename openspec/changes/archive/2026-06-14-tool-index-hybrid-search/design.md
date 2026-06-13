## Context

Ozy's §9 triad is in place but only partly aligned with the spec's catalog model:

- `findTool` (`internal/broker/live.go`) does **live discovery**: every call runs
  `connector.ConnectAll`, lists tools from each enabled server, and returns all of
  them as `choose_from_candidates`. There is **no ranking** — the catalog-backed
  `skeleton.FindTool` explicitly returns `no_good_match` with "ranking is not
  implemented." So the agent receives the whole tool universe, reconnects to every
  server on each query, and loses discovery entirely when a server is offline.
- `describeTool` (`skeleton.DescribeTool`) already does **exact** `toolRef` lookup
  via `store.GetTool` and returns the exact input schema + status. It only needs a
  reliably-populated catalog.
- `callTool` (`live.CallTool`) already does **live** brokered invocation
  (shipped in `mcp-calltool-live-invocation`); it resolves via config, not the
  catalog, and is unchanged here.
- `internal/index` already connects → `tools/list` → normalizes → persists, but
  runs only from `ozy index`. `internal/catalog` persists tools with per-tool
  `LastIndexedAt` but has no run-level index timestamp. `config.Loaded.Path` holds
  the source path; the daemon's `Run` just prints ready and blocks.

The spec's intended model (`SPEC.md` §4.4, §10) is a persistent catalog that
`findTool` ranks with hybrid search. This change implements that: index on
startup when stale, and rank the catalog instead of dumping live results.

## Goals / Non-Goals

**Goals:**

- Populate the catalog on daemon startup, but only when stale relative to the
  config file's mtime; never block readiness on indexing failure.
- Replace live `findTool` with hybrid (lexical + optional semantic) ranking over
  the persistent catalog, returning a best match and one runner-up with an
  explicit decision, `confidence`, and a `reason`.
- Keep semantic search optional with graceful, surfaced fallback to lexical.
- Keep `describeTool` (exact catalog lookup) and `callTool` (live) behavior intact
  and prove the full `findTool` → `describeTool` → `callTool` loop end to end.
- Implement ranking once behind the broker seam so MCP and CLI stay in parity.

**Non-Goals:**

- The Python embedding/indexing worker (`SPEC.md` §10.4). The semantic seam ships
  with an "unavailable" default; the runtime stays lexical-only until a provider
  lands in a later change.
- A durable inverted index or external search engine (e.g. SQLite FTS). The
  catalog is small (tens–low hundreds of tools); ranking runs in-process.
- Background/scheduled refresh, tool-list-change subscriptions, schema-drift
  rejection, and argument validation — all out of scope here.
- Quantified token-economy targets. Per `SPEC.md` §13 these are set after eval
  baselines exist; this change states directionality only.

## Decisions

### `findTool` becomes catalog-backed; the broker delegates to a search engine

`live.FindTool` stops calling `ConnectAll`. A new `internal/search` package owns
retrieval: `Engine.Find(ctx, query) -> Ranking` reads the catalog via
`catalog.Store`, scores and orders candidates, and returns the ranked list plus
the fused scores and match explanations. The broker translates the `Ranking` into
the `contract.FindResult` (decision, `selected`, `alternatives`, `nextAction`,
instructions). Retrieval logic lives in `search`; contract/instruction shaping
stays in `broker`. `describeTool`/`callTool` delegation is untouched.

_Alternative considered:_ keep live discovery and rank the live results.
Rejected — it reconnects to every server per query, breaks offline discovery, and
contradicts the persistent-catalog model (`SPEC.md` §4.4). Live discovery was the
bootstrap; the catalog is now populated on startup, so ranking the catalog is the
correct path.

### Lexical baseline: field-weighted BM25-style scoring over the indexed fields

The lexical scorer tokenizes the query and the catalog's indexed fields
(`SPEC.md` §10.2) and scores each tool with BM25-style term weighting plus
per-field boosts, so a hit in `toolRef`/`name`/`title` outranks a hit in a schema
field description. Capability aliases and exact/substring matches on the tool name
get an additional boost. Matched terms and their top contributing fields are
retained for the `reason`. Scoring is a linear pass over the catalog (small N);
an inverted index is a later optimization that does not change the contract.

_Alternative considered:_ SQLite FTS5 / Bleve. Rejected for now — adds a storage
dependency and migration surface for a catalog that fits comfortably in memory.

### Hybrid fusion: normalized weighted sum, not RRF (so the confidence floor is meaningful)

Each signal is mapped to `[0,1]` on an **absolute** scale — lexical via a
saturating transform `s/(s+k)` (BM25 scores are unbounded; min-max over a tiny
candidate set is unstable), semantic via `(cos+1)/2`. The fused score is
`w_lex·lexNorm + w_sem·semNorm` with weights renormalized when a signal is absent
(semantic weight → 0, lexical → 1). The fused score therefore stays in `[0,1]` and
supports an **absolute confidence floor** and a **separation margin** directly.

_Alternative considered:_ Reciprocal Rank Fusion (`Σ 1/(k+rank)`). RRF is robust
when score scales are incomparable, but it is purely rank-based, so it gives no
absolute relevance to threshold "is this a real match at all?" against. We need
that threshold for the `no_good_match` decision, so weighted-normalized-sum is the
default; RRF can sit behind a config knob if score calibration proves brittle.

### Confidence → decision mapping (the two-best contract)

Let `top` and `second` be the two best fused scores. With a relevance floor
`F` and a separation margin `M` (illustrative defaults `F≈0.25`, `M≈0.10`,
high-confidence at `top≥~0.6`, to be calibrated by evals):

- `top < F` → `no_good_match` (refine query / `ozy doctor`; never imply absence).
- `top ≥ F` and `top − second ≥ M` → `use`: `selected` = rank 1 (with
  `schemaPreview` + status), `alternatives` = `[rank 2]` (the runner-up),
  `nextAction` = `describeTool(selectedToolRef)`. `confidence` is `high` when
  `top` is high, else `medium`.
- `top ≥ F` and `top − second < M` → `ambiguous`: surface both best tools and
  instruct the agent to `describeTool`/choose; do not auto-select.
- zero indexed tools → `catalog_empty` (existing decision/instruction).

This is exactly "two best tools — one with high certainty, one runner-up": rank 1
is the high-certainty `selected`, rank 2 is the single runner-up, surfaced as the
alternative in `use` and as a co-candidate in `ambiguous`.

### Semantic seam with an "unavailable" default; explicit degradation

`search.Semantic` is an interface (`Score(query, tools) ([]float64, bool)` or
similar) with a default `unavailable` implementation. The engine fuses semantic
only when `search.semantic.enabled` **and** a provider reports available;
otherwise it ranks lexical-only and the result carries a `degraded` marker that
the broker surfaces (e.g. a note / `SEMANTIC_SEARCH_UNAVAILABLE` signal) without
failing. This honors `SPEC.md` §4.10/§10.1 and the "never unusable without
embeddings" anti-pattern, and reuses the daemon's existing `SemanticDegraded()`
notion.

### Startup indexing gated on config mtime; synchronous but bounded; graceful

In `daemon.Run`, before reporting ready: read `lastIdx, ok :=
store.LastIndexedAt(ctx)` and `cfgMtime := os.Stat(cfg.Path).ModTime()`. The
catalog is **stale** when `!ok` or `cfgMtime.After(lastIdx)`. When stale, run
`index.Indexer.Run(ctx, cfg.Resolved)` synchronously (bounded by the indexer's
existing per-server discovery timeouts), then report ready **regardless of
outcome**, surfacing the index summary. When fresh, skip indexing. If `os.Stat`
fails, fall back to "index only if no prior index exists" to avoid thrashing.

_Alternative considered:_ index in the background after reporting ready. Rejected
for the first cut — it races the first `findTool` against an empty/stale catalog;
the spec scenario wants a stale catalog refreshed *before* readiness. Async
refresh is a clean follow-up once startup latency is measured.

### Record a run-level last-index time in the catalog

`catalog.Store` gains `LastIndexedAt(ctx) (time.Time, bool, error)` and a setter
(written by a successful index run). The file store persists it as a small
metadata record alongside tools; the in-memory store keeps it in a field. The
`bool` distinguishes "never indexed" from a zero time so staleness is unambiguous.

_Alternative considered:_ derive `max(tool.LastIndexedAt)`. Rejected — a reachable
server that exposes zero tools is a *successful* run that records nothing, and
per-tool times don't represent a run boundary. An explicit marker is correct.

### No adapter/CLI surface change

`internal/mcp` and `internal/cli` already route `findTool`/`describeTool`/
`callTool` through the broker and render the contract types. Parity is automatic;
the advertised MCP surface stays exactly the three tools.

## Risks / Trade-offs

- **Confidence thresholds are guesses until evals exist** → ship `F`/`M`/weights
  as named, tunable constants with conservative defaults; the discovery eval
  families (`SPEC.md` §14.1) calibrate them; record `reason`/scores so decisions
  are auditable.
- **Synchronous startup indexing slows daemon start with many/slow servers** →
  bounded by existing per-server timeouts; failures never block readiness; async
  refresh is a noted follow-up.
- **Stale catalog can surface a tool whose server is now offline** → intended:
  search is stale-tolerant and marks freshness; `callTool` remains live-gated and
  returns `DOWNSTREAM_SERVER_OFFLINE` if it is actually down.
- **min-max normalization instability** → avoided by using an absolute saturating
  transform for the lexical signal rather than normalizing across the candidate
  set.
- **"Hybrid" with semantic defaulting to unavailable looks lexical-only at
  runtime** → accepted and explicit: the fusion framework and degradation are
  real and tested with a fake semantic scorer; the provider is a separate change.
- **Behavioral change to `findTool`** → previously `choose_from_candidates` over
  live tools, now a ranked decision over the catalog. Documented as a behavioral
  break; the contract shape (`FindResult`) is unchanged, so adapters/clients are
  unaffected structurally.

## Migration Plan

Additive and within the existing `findTool` contract shape. First daemon start
after this change indexes the (empty/stale) catalog, then serves ranked results;
later starts skip indexing until `ozy.jsonc` changes. No config migration. The
semantic seam defaults to unavailable, so no embedding setup is required.
Rollback: revert `live.FindTool` to live discovery and skip the startup-index
call; the new `LastIndexedAt` store method is backward-compatible and harmless.

## Open Questions

- Default values for `F`, `M`, and `w_lex`/`w_sem` — seed conservatively and let
  the discovery eval gold sets (`SPEC.md` §14.1) tune them; do they belong in
  `config` (`search` section) eventually?
- Should startup indexing become asynchronous (ready-then-refresh) once startup
  latency is measured, or stay synchronous for first-query correctness?
- Should `findTool` ever fall back to live discovery when the catalog is
  `catalog_empty` and servers are reachable, or always instruct `ozy index`?
  Current choice: instruct indexing, keeping search strictly catalog-backed.
