## Context

The prior `tool-index-hybrid-search` change built the whole hybrid-search frame
but deliberately deferred the semantic half. Concretely, today:

- `internal/search` ranks the catalog with a field-weighted BM25 lexical scorer
  and fuses an optional semantic signal with a **normalized weighted sum**
  (`fuseScores` in `fusion.go`). The fusion was chosen as a stopgap precisely
  because no dense signal existed to fuse.
- `search.Semantic` is an interface — `Score(ctx, query, tools) ([]float64,
  error)` + `Available()` — and the daemon wires `search.New(store, nil)`, so it
  is permanently lexical-only. `Daemon.SemanticDegraded()` reports "requested but
  unavailable" whenever `search.semantic.enabled` is true.
- `internal/config` already has an `embedding` section (`provider`, `required`)
  and a `search.semantic` toggle, but nothing reads them at runtime.
- `internal/index` connects → `tools/list` → normalizes → `store.PutTool`, and
  the daemon runs it on startup when the catalog is stale.

`SPEC.md` §10.4 names the target boundary directly: *Go owns runtime, catalog
authority, and online search; Python may be an optional embedding/indexing
worker; Python must not own the authoritative catalog; lexical must keep working
without it.* This change implements exactly that boundary as an integrated
sidecar, swaps the stopgap weighted-sum for RRF now that a real second signal
exists, and keeps every degradation path the prior change established.

## Goals / Non-Goals

**Goals:**

- Add an optional Python sidecar that embeds cataloged tools (FastEmbed) and
  serves semantic nearest-neighbor queries, talking to the Go daemon over stdio.
- Make `findTool` fuse the Go lexical ranking with the sidecar's semantic ranking
  using **RRF**, returning a winner (`selected`) and a single runner-up
  (`alternatives[0]`).
- Make hybrid search the **default experience** while keeping setup zero-touch:
  semantic is on by default, turbovec is the zero-config default backend, the
  sidecar and its environment are auto-provisioned on demand, and the user is
  never asked about vector storage unless they opt into FAISS (set before the
  first index).
- Preserve graceful degradation end to end: no Python, failed provisioning, or a
  crashed/timed-out sidecar all fall back to lexical-only, surfaced, never fatal.
- Keep Go the sole authority for the catalog, `describeTool`, and `callTool`; the
  sidecar's SQLite is source-of-truth only for the embedding subsystem.

**Non-Goals:**

- Building the full install/bootstrap flow — the planned
  `go run github.com/.../ozy@<version>`-style installer that downloads deps,
  provisions the environment, and scaffolds config. This change assumes
  automated on-demand provisioning at daemon start and MUST NOT structurally
  depend on semantic being off; the dedicated bootstrap change owns the polished
  first-run mechanics later.
- A reranker stage, query expansion/rewriting, or multi-vector/late-interaction
  retrieval. Two legs (lexical + dense) fused by RRF is the whole pipeline here.
- GPU embedding, remote/hosted embedding providers, or batched background
  re-embedding on a schedule. Embedding is pull-driven by index runs.
- Changing the `findTool`/`describeTool`/`callTool` contract shape or the MCP
  surface. Only the ranking mechanism and the semantic provider change.
- Calibrating the final RRF `k` and confidence floors — seeded conservatively and
  tuned by the discovery evals (`SPEC.md` §14.1).

## Decisions

### Boundary: Go keeps lexical + fusion + authority; Python owns embeddings + vectors

The sidecar produces **only** the semantic ranking. Go keeps the existing lexical
BM25 ranker, performs RRF, maps the fused ranking to the decision, and remains the
authority for the catalog, `describeTool`, and `callTool`. This is the `SPEC.md`
§10.4 boundary verbatim and reuses everything the prior change shipped.

_Alternative considered:_ let Python own both lexical and semantic search (e.g.
SQLite FTS5 + vectors) and have Go just call it. Rejected — Go already has a
tested lexical ranker, `SPEC.md` §10.4 keeps online search and catalog authority
in Go, and it would make lexical search depend on Python, violating §10.1.

### Transport: newline-delimited JSON over the sidecar's stdio

Go spawns the sidecar as a child process and speaks **JSONL** over its
stdin/stdout; stderr is drained separately for logs. Each request is one JSON
object `{ "id", "op", ... }` with `op ∈ {health, upsert, query, delete, stats}`;
each response is one JSON object correlated by `id`. Requests are issued one at a
time per index/query operation (simple, no pipelining needed at this scale).

_Alternative considered:_ a local HTTP or gRPC server. Rejected for the first cut
— stdio binds the sidecar lifecycle to the daemon (no port allocation, no auth,
no leaked listeners), mirrors how Ozy already launches local MCP servers, and is
trivial to supervise. A socket transport can replace the seam later without
changing the protocol shape.

### Embeddings: FastEmbed, CPU/ONNX, model stamped per vector

The sidecar embeds with FastEmbed (ONNX Runtime, no PyTorch) so the environment
stays small and CPU-only. Default model **`BAAI/bge-small-en-v1.5`** (384-dim) —
strong zero-shot quality at a small footprint; the dimension is derived from the
model, never configured separately. Every stored vector records its
`model` id and `dim`; a model change marks existing vectors stale and triggers a
rebuild rather than mixing embedding spaces (the model-versioning discipline from
retrieval-pipeline production-readiness).

### Storage: SQLite is the embedding subsystem's source of truth; the index is derived

The sidecar keeps a SQLite DB (under XDG state, e.g.
`~/.local/state/ozy/embeddings.db`) with roughly:

- `tools(tool_ref TEXT PRIMARY KEY, vector_id INTEGER UNIQUE, content_hash TEXT,
  server_id TEXT, tags TEXT, model TEXT, dim INTEGER, vector BLOB)` — `vector_id`
  is a monotonic `uint64` external id; `vector` is the raw float32 embedding.
- `meta(key, value)` — records the active `backend`, `model`, `dim`, and a schema
  version.

Storing the **raw** embedding makes the vector index a pure derived artifact: it
can be rebuilt from SQLite without re-embedding or re-contacting downstream
servers. `server_id`/`tags` columns are the facet source for filtered queries.
SQLite is explicitly *not* Ozy's catalog — `describeTool`/`callTool` never read
it.

### Vector index: turbovec default, FAISS optional, behind one Python interface

A small `VectorBackend` Python interface (`add_with_ids`, `search(query, k,
allowlist)`, `remove`, `write`, `load`) has two implementations:

- **turbovec (default):** `IdMapIndex(dim, bit_width=4)`. `add_with_ids(vectors,
  np.uint64 ids)` ties vectors to the SQLite `vector_id`; `search(q, k,
  allowlist=ids)` does kernel-level filtered ANN; `write(path)`/`load(path)`
  persist. No train step, quantized footprint, zero config — the reason it's the
  default the user never has to think about.
- **FAISS (opt-in):** `IndexIDMap(IndexFlatIP)` (or HNSW) with the same `uint64`
  ids; facet filtering via an `IDSelectorBatch` built from the SQLite allowlist;
  `write_index`/`read_index` persist.

The active backend is recorded in `meta`. It is selected **before the first
index** via `embedding.vectorBackend` and is immutable afterward: on load, a
mismatch between configured backend and recorded backend forces a rebuild
(reindex) rather than reading one backend's file with the other's reader.

_Alternative considered:_ make turbovec the only backend. Rejected — the user
explicitly wants FAISS as an escape hatch; the interface costs little and isolates
the dependency so FAISS is required only on its opt-in path.

### Filtered search: SQLite resolves facets to an allowlist, the kernel filters

A `query` with a facet filter (e.g. `server_id = "atlassian"`) is resolved in
SQLite to the set of allowed `vector_id`s, which is passed as turbovec's
`allowlist` (or FAISS's `IDSelectorBatch`). Filtering happens inside the search
kernel — no over-fetch-then-post-filter — and returned ids map back to `toolRef`
via SQLite. This is the concrete payoff of keeping metadata in SQLite next to the
index.

### Fusion: RRF over the two rank lists, replacing the weighted sum

Go builds two ranked lists — lexical (existing BM25 order) and semantic (the
sidecar's returned order) — and fuses them with Reciprocal Rank Fusion:
`score(tool) = Σ_lists 1/(k + rank)` with `k = 60` (a named constant). The
winner is the top fused tool, the runner-up is the second. RRF is rank-based, so
it is robust to the incomparable scales of BM25 scores and cosine similarity —
the documented default for hybrid retrieval and exactly the user's acceptance
criterion. When semantic is unavailable, RRF over the single lexical list reduces
to the lexical order, so the degraded path is unchanged.

_Alternative considered:_ keep the normalized weighted sum. Rejected — it depends
on calibrating two different score distributions onto one scale and re-breaks on
every embedding-model change; RRF needs no score calibration. (The prior change
only chose weighted-sum because it had a single signal and wanted an absolute
floor; see the next decision for how the floor survives under RRF.)

### Decision model: RRF orders; absolute component floors gate `no_good_match`

RRF gives ordering, not "is this a real match at all," so the confidence floor is
evaluated on the **component** signals, not the RRF score: the top tool must clear
a lexical floor (the existing `s/(s+k)` normalized lexical score) **or** a
semantic floor (cosine similarity) to be `use`/`ambiguous`; otherwise
`no_good_match`. `ambiguous` is decided by closeness of the top two in the fused
ranking; `catalog_empty` is unchanged. This preserves the prior change's
decision contract (winner + runner-up, four verdicts) on top of rank-based
fusion.

### Go seam: evolve `Semantic` from per-tool scoring to a ranked query

`search.Semantic` changes from `Score(ctx, query, tools) ([]float64, error)` to a
query-shaped seam, roughly `Query(ctx, query string, k int, filter Filter)
([]SemanticHit, error)` plus `Available() bool`, where `SemanticHit{ToolRef
string; Score float64}`. ANN returns a ranked top-K, not a score per catalog
tool, so this fits the data and feeds RRF directly. The default implementation
still reports `Available() == false`, keeping `search.New(store, nil)` lexical-
only. A new `internal/sidecar` package provides the real implementation: it owns
the child process, encodes/decodes JSONL, and returns `Available() == false` (not
an error) whenever the sidecar is absent or unhealthy, so the engine degrades.

### Lifecycle & provisioning: on-demand isolated env, supervised by the daemon

When `search.semantic.enabled`, the daemon provisions and launches the sidecar
before driving the first index, health-checks it, and shuts it down on its own
shutdown — none of which blocks readiness. Provisioning prefers **`uv`** to
create a pinned, isolated environment under XDG state (fast, reproducible,
no global pollution); it falls back to `python -m venv` + `pip` when `uv` is
absent; if no usable Python toolchain exists, it degrades to lexical-only and
says so. The sidecar source ships with Ozy (a `sidecar/` package); provisioning
installs its pinned deps (`fastembed`, `turbovec`, and `faiss-cpu` only when the
FAISS backend is selected) once and caches the env. This on-demand provisioning
is the interim path that makes default-on semantic work today; the planned
standalone bootstrap/installer change (deps download, env setup, config
scaffolding) will likely take over the polished first-run mechanics, so this
logic is kept behind the `internal/sidecar` seam rather than spread through the
daemon.

_Alternative considered:_ require a user-managed Python and a manual `pip
install`. Rejected — it breaks "lean and simple"; auto-provisioning is the whole
point of the user never being bothered with vector storage. Shipping a frozen
PyInstaller binary is a heavier future alternative noted under Open Questions.

### Embed-on-index: index runs push upserts; reconcile deletes; persist once

`internal/index` gains an optional sink: after persisting tools to the catalog,
when semantic is enabled it sends a batched `upsert` of `{toolRef, text,
contentHash, facets}` for the run's tools, then a reconcile so the sidecar
`delete`s any `toolRef` no longer in the catalog, then asks the sidecar to persist
(`write`) the index. The sidecar skips unchanged tools by `contentHash`, so
re-indexing an unchanged catalog embeds nothing. Embedding text is the same
indexed-field set the lexical scorer uses (`SPEC.md` §10.2).

### Config: backend + model under `embedding`; semantic on by default

`EmbeddingConfig` gains `vectorBackend` (`turbovec` default, `faiss` opt-in,
validated) and `model` (FastEmbed model id, documented default).
`search.semantic.enabled` **defaults to true** (resolved like the opencode
`enabled` fields — unset means on), so hybrid search is the out-of-the-box
behavior and setting it `false` is the explicit escape hatch to lexical-only.
Validation rejects an unknown backend with a `CONFIG_ERROR`; nothing about vector
storage is required for the default turbovec path. Crucially, default-on does
**not** mean hard-fail without Python: if provisioning fails the daemon still
degrades to lexical (see the lifecycle decision and Risks), so the default value
proposition ships without making the app brittle.

## Risks / Trade-offs

- **First-run provisioning latency, now on the default path** (download model +
  install deps) → provision lazily and cache the env; never block readiness;
  surface progress; embedding runs in the background of the index pass.
  Subsequent starts reuse the env and the persisted index, and the planned
  bootstrap flow can move the heavy download to install time.
- **turbovec quantization (2/4-bit) loses some recall** → default to 4-bit;
  RRF + the two-best contract is tolerant of small ANN error; expose `bit_width`
  later if evals show loss. FAISS flat is available for exactness-sensitive users.
- **New Python dependency / supply-chain surface** → optional, isolated, pinned;
  lexical search never imports it; the Go binary and lexical-only users gain no
  mandatory dependency.
- **stdio framing / backpressure / deadlock** → newline-delimited messages,
  bounded message size, per-request timeouts, stderr drained on its own goroutine,
  one in-flight request per operation. A hung/timed-out request degrades that
  query to lexical.
- **Embedding-space drift / model mismatch** → `model`+`dim` stamped per vector
  and in `meta`; a mismatch on load rebuilds rather than serving mixed spaces.
- **RRF discards absolute relevance** → mitigated by gating `no_good_match` on the
  absolute component scores, not the RRF score (decision above).
- **Cross-platform Python discovery (esp. Windows)** → resolve an interpreter
  robustly (`uv`, then `python3`/`py`), and degrade cleanly when none is usable.
- **Catalog ↔ embedding-store divergence** → index runs reconcile deletes and the
  index rebuilds from SQLite on mismatch; SQLite is derived, never authoritative,
  so a divergence is always recoverable by reindex.

## Migration Plan

Additive and opt-in. Existing lexical-only users are unaffected: `search.New(store,
nil)` behavior is preserved whenever semantic is disabled or the sidecar is
unavailable. Enabling `search.semantic.enabled` on the next daemon start (or `ozy
index`) provisions the environment, embeds the catalog, and persists the index;
`findTool` then returns RRF-fused results. Choosing FAISS requires setting
`embedding.vectorBackend: faiss` before the first index (or reindexing to switch).
Rollback is config-only: disable semantic search to return to lexical-only, or
revert the `Semantic` seam to the unavailable default. No catalog migration; the
embedding store and vector index live in separate files and can be deleted and
rebuilt safely.

## Open Questions

- **Bootstrap ownership.** Default-on is decided — semantic is on by default. The
  open question is how much provisioning this change does inline versus defers to
  the planned standalone installer/bootstrap flow (`go run …ozy@<version>`), and
  where first-run UX (progress, retries, offline model cache) lives.
- **Provisioning mechanism:** standardize on `uv`, or also support pip-only /
  pipx, or ship a frozen single-file binary (PyInstaller) to avoid a Python
  toolchain entirely? Current choice: `uv` with a `venv`+`pip` fallback.
- **turbovec `bit_width`:** default 2-bit or 4-bit, and expose it as config? Seed
  4-bit; revisit with recall evals.
- **RRF `k` and component floors:** seed `k = 60` and reuse the prior change's
  lexical floor plus a new semantic cosine floor; calibrate against the discovery
  gold sets (`SPEC.md` §14.1). Do these belong in the `search` config section?
- **Embedding source on rebuild:** store raw float32 vectors in SQLite (chosen,
  rebuild without re-embedding) vs. re-embed from text on rebuild (smaller DB,
  slower rebuild, needs the model). Current choice: store raw vectors.
- **Re-embedding ownership:** keep embedding strictly pull-driven by Go index runs
  (chosen), or let the sidecar own drift/refresh scheduling later?
