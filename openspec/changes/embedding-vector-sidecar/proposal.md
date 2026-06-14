## Why

Semantic search is the half of Ozy's hybrid ranking that has never actually run.
The prior `tool-index-hybrid-search` change shipped the `search.Semantic` seam
with a permanent "unavailable" default, so `findTool` is lexical-only today and
the `embedding` / `search.semantic` config is inert. `SPEC.md` §10.4 already
prescribes the fix — an optional Python embedding/indexing worker behind the Go
daemon — but no provider exists. This change lands that worker as an integrated
sidecar so `findTool` fuses lexical and semantic signals **by default**, with the
sidecar auto-provisioned on the user's behalf and the lexical baseline kept as a
robust fallback if provisioning ever fails.

## What Changes

- Add a **Python embedding + vector-storage sidecar** that the Go daemon
  launches and talks to over **stdio** (newline-delimited JSON): Go sends
  embed/query/delete jobs and the sidecar replies with "embeddings stored" and
  ranked semantic hits. The sidecar never owns Ozy's authoritative catalog
  (`SPEC.md` §10.4); it only owns the embedding subsystem.
- Embeddings via **FastEmbed** (ONNX, CPU-only, no PyTorch) so the worker stays
  small; the model id and vector dimension are recorded with every vector for
  safe migration.
- **SQLite** is the sidecar's source of truth for embedding-side metadata — the
  `toolRef ↔ vector-id` map, content hashes (to skip re-embedding unchanged
  tools), the embedding model/version, and the filterable facets (server id,
  tags) used to scope a query. The vector index is a derived artifact,
  rebuildable from SQLite.
- **turbovec** is the **default** vector index (`IdMapIndex` with kernel-level
  `allowlist` filtering); **FAISS** is an opt-in alternative. The backend is
  chosen **before the first index is built** and is immutable afterward
  (switching requires a reindex). A user who does nothing gets turbovec and is
  never asked about vector storage.
- **BREAKING (behavioral):** `findTool` fusion moves from the prior normalized
  weighted-sum to **Reciprocal Rank Fusion (RRF)** over the lexical and semantic
  rank lists, returning the two best tools — winner (`selected`) and runner-up
  (`alternatives[0]`). An absolute relevance floor on the underlying component
  scores is retained so `no_good_match` stays meaningful under rank-based fusion.
- On `ozy index` and on stale-catalog startup indexing, Go **also pushes each
  cataloged tool's indexed text to the sidecar to embed and store**, but only
  when semantic search is enabled.
- **Graceful degradation is preserved end to end:** if Python or the sidecar is
  unavailable, fails to provision, or crashes, `findTool` degrades to
  lexical-only and surfaces the degraded mode rather than failing (`SPEC.md`
  §4.10, §10.1).

## Capabilities

### New Capabilities
- `embedding-sidecar`: The optional Python worker and its Go integration — the
  stdio request/response protocol, FastEmbed embedding, the SQLite
  embedding-metadata store, embed-on-index ingestion, semantic query (ranked
  `toolRef` + score with facet filtering), sidecar lifecycle (provision, spawn,
  health, shutdown), and graceful degradation when the worker is absent.
- `vector-storage`: The pluggable vector index behind the sidecar — turbovec as
  the zero-config default and FAISS as an explicit opt-in, backend selection
  fixed before the first index and immutable after, `allowlist`/facet-scoped
  nearest-neighbor search, model/dimension stamping, and on-disk persistence
  that is rebuildable from SQLite.

### Modified Capabilities
- `tool-search`: Hybrid fusion becomes RRF over the lexical and semantic rank
  lists; the semantic signal is now produced by the sidecar instead of the
  unavailable stub; the winner + runner-up and the `use` / `ambiguous` /
  `no_good_match` decisions are derived from the fused ranking with an absolute
  relevance floor retained.
- `daemon-runtime`: The daemon SHALL provision and supervise the sidecar when
  semantic search is enabled, push embedding jobs during startup and explicit
  indexing, and SHALL degrade to lexical-only (never blocking readiness) when the
  sidecar is unavailable.
- `configuration`: Add vector-backend selection (`turbovec` default, `faiss`
  opt-in) and embedding-model selection, fix the backend before the first index,
  and make **semantic search enabled by default** (auto-provisioned, with an
  explicit disable as the escape hatch) so hybrid search is the out-of-the-box
  experience.

## Impact

- Affected code:
  - `sidecar/` (**new**, Python): FastEmbed embedder, SQLite metadata store,
    turbovec/FAISS backend behind one interface, and the stdio JSON loop.
  - `internal/sidecar` (**new**, Go): the stdio client — spawn/health/encode/
    decode, embed-on-index calls, query calls, and the "unavailable → degrade"
    path that satisfies `search.Semantic`.
  - `internal/search` (**modified**): replace weighted-sum fusion with RRF;
    evolve the `Semantic` seam from "score every candidate" to "return a ranked
    semantic hit list (`toolRef`, score)"; retain an absolute floor for
    `no_good_match`.
  - `internal/daemon`: provision/supervise the sidecar, wire the real `Semantic`
    provider into the engine, and drive embed-on-index.
  - `internal/index`: after persisting a tool, emit it to the sidecar for
    embedding when semantic is enabled.
  - `internal/config`: extend `EmbeddingConfig` / `SearchConfig` for vector
    backend + model; validate "backend fixed before first index."
  - `internal/cli` (`doctor`): report sidecar/Python availability, backend,
    model, and embedded-tool counts.
- Affected behavior: an agent's `findTool` returns an RRF-fused winner +
  runner-up using real semantic intent; the first index with semantic enabled
  provisions the sidecar and embeds the catalog; with no sidecar, behavior is
  exactly today's lexical-only ranking.
- Dependencies: a Python toolchain (FastEmbed, turbovec, optional `faiss-cpu`)
  **auto-provisioned** into an isolated environment on first run as part of the
  default semantic experience; the Go binary still builds and starts without it
  and degrades to lexical-only if provisioning fails. The polished install/
  bootstrap flow that downloads and sets this up smoothly is planned as a
  **separate later change** and is out of scope here.
