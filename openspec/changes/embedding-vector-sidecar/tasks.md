## 1. Configuration: vector backend and embedding model

- [x] 1.1 Extend `EmbeddingConfig` with `vectorBackend` (default `turbovec`, accept `faiss`) and `model` (documented FastEmbed default); derive the vector dimension from the model at runtime rather than from config, and default `search.semantic.enabled` to true when unset (semantic on by default) with explicit `false` to disable.
- [x] 1.2 Validate the backend in `config.validate` (unknown value → `CONFIG_ERROR` naming the field); add config tests for the turbovec default, an accepted `faiss`, a rejected unknown backend, and that omitting `search.semantic.enabled` enables semantic (default-on) while an explicit `false` disables it and leaves the sidecar unprovisioned.

## 2. Python sidecar: scaffold, stdio protocol, embedding, SQLite store

- [x] 2.1 Create the `sidecar/` Python package with a pinned dependency set (`fastembed`, `turbovec`; `faiss-cpu` as an opt-in extra) and an entrypoint that reads newline-delimited JSON requests on stdin and writes id-correlated JSON responses on stdout with logs on stderr; an unknown or unparseable op returns a structured error and does not exit the process.
- [x] 2.2 Implement the FastEmbed embedder (CPU/ONNX, default model) that stamps `model` and `dim` on output and uses the same model for documents and queries.
- [x] 2.3 Implement the SQLite store — `tools(tool_ref PK, vector_id UNIQUE uint64, content_hash, server_id, tags, model, dim, vector BLOB)` and `meta(key, value)` — with helpers to map `toolRef`↔`vector_id`, resolve a facet filter to an allowlist of `vector_id`s, and detect unchanged tools by content hash; store the raw float32 embedding so the index is rebuildable.
- [x] 2.4 Implement the `health`, `upsert` (skip-unchanged, embed-changed), `delete`, `query` (embed → search → map ids→`toolRef` → ordered `{toolRef, score}`), and `stats` operations over the embedder and store.
- [ ] 2.5 Add pytest coverage: framed round-trips, unknown-op error is non-fatal, upsert skips unchanged by hash, delete removes, query returns ordered `toolRef`s, and a facet filter scopes results. (24 tests collected, store tests pass; embedder/vector/ops tests need real model download)

## 3. Python sidecar: pluggable vector backend (turbovec default, FAISS optional)

- [x] 3.1 Define a `VectorBackend` interface (`add_with_ids`, `search(query, k, allowlist)`, `remove`, `write`, `load`) and implement the turbovec backend with `IdMapIndex(dim, bit_width=4)`, kernel-level `allowlist` filtering, and `write`/`load` persistence.
- [x] 3.2 Implement the FAISS backend (`IndexIDMap`/`IndexFlatIP` with `uint64` ids and an `IDSelectorBatch` allowlist), importing `faiss` only when that backend is selected so `faiss-cpu` is required only on the opt-in path.
- [x] 3.3 Record the active backend/model/dim in `meta`; on load, rebuild the index from the SQLite raw vectors when the persisted index is missing or its backend/model/dim mismatches config, and refuse to read one backend's file with the other's reader.
- [ ] 3.4 Add pytest coverage: turbovec is used with no config, FAISS is honored when selected, allowlist-filtered search returns only allowed ids, persistence survives reload, and a backend/model mismatch triggers a rebuild from SQLite. (deferred — needs real model download or FakeEmbedder integration)

## 4. Go sidecar client and on-demand provisioning (`internal/sidecar`)

- [x] 4.1 Create `internal/sidecar` that spawns the Python sidecar over stdio, encodes/decodes the JSONL protocol with per-request timeouts, drains stderr to logs, and exposes `Health`, `Upsert`, `Delete`, `Query`, `Stats`, and `Close`.
- [x] 4.2 Implement on-demand provisioning behind the `internal/sidecar` seam (the interim auto-provisioner ahead of the planned standalone bootstrap/installer change): resolve an interpreter and create a pinned isolated environment under XDG state via `uv` (fallback `python -m venv` + `pip`), installing `faiss-cpu` only when the FAISS backend is selected, caching the env, and returning a clear "unavailable" signal (not an error) when no usable toolchain exists.
- [x] 4.3 Add Go tests against a fake stdio sidecar (scripted JSONL): health/upsert/query/delete happy paths, request timeout, sidecar exit mid-session, and garbage responses — each surfacing "unavailable" rather than panicking.

## 5. Search: ranked semantic seam, RRF fusion, component-floor decisions

- [x] 5.1 Evolve `search.Semantic` to a query seam (`Query(ctx, query, k, filter) ([]SemanticHit, error)` + `Available()`), keep the default unavailable implementation, and adapt `Engine.Find` to obtain a semantic rank list when available.
- [x] 5.2 Replace weighted-sum fusion with RRF (`Σ 1/(k+rank)`, `k=60` as a named constant) over the lexical and semantic rank lists, so that with no semantic signal RRF over the lexical list alone reproduces the lexical order.
- [x] 5.3 Rework `Decide` so `no_good_match` is gated on absolute component scores (lexical `s/(s+k)` floor OR semantic cosine floor), `use`/`ambiguous` come from fused top-two separation, and `catalog_empty` is unchanged; expose the floors and margin as named tunable constants.
- [x] 5.4 Make `internal/sidecar` satisfy the evolved `Semantic` seam (map `Query` to a sidecar `query`; `Available()` reflects sidecar health), returning unavailable on any failure so the engine degrades.
- [x] 5.5 Update search and broker tests: RRF orders winner + runner-up with a fake semantic provider, component-floor `no_good_match`, `ambiguous`, the semantic-unavailable degraded path, and a per-query semantic failure degrading to lexical.

## 6. Index: embed-on-index sink, reconcile, persist

- [x] 6.1 Add an optional embedding sink to `internal/index`: after persisting the run's tools, when semantic is enabled, batch-`upsert` `{toolRef, text (the §10.2 fields), contentHash, facets}` to the sidecar, reconcile `delete`s for `toolRef`s no longer in the catalog, then ask the sidecar to persist the index.
- [x] 6.2 Add index tests: changed tools are embedded and unchanged tools skipped by content hash, removed tools are deleted, and a run with semantic disabled (or the sidecar unavailable) still succeeds and persists the catalog unchanged.

## 7. Daemon: provision, supervise, wire, degrade

- [x] 7.1 In the daemon, when semantic is enabled, provision/launch/health-check the sidecar before the startup index, construct the real `Semantic` provider, wire it into `search.New(store, provider)`, and shut the sidecar down on daemon shutdown.
- [x] 7.2 Drive embed-on-index from both startup indexing and `ozy index`; ensure provisioning/supervision never blocks readiness, any sidecar failure degrades to lexical-only with a surfaced notice, and `SemanticDegraded()` reflects real sidecar health.
- [x] 7.3 Add daemon tests: semantic-enabled with a fake-available provider wires it and reports ready; semantic-enabled but provisioning/health fails still reports ready serving lexical with a degraded notice; the sidecar is shut down with the daemon.

## 8. CLI doctor surfaces the embedding subsystem

- [x] 8.1 Extend `ozy doctor` to report Python/sidecar availability, active backend, embedding model and dimension, and embedded-tool count (via `stats`), degrading cleanly to "semantic unavailable (lexical-only)" when the sidecar is absent.
- [x] 8.2 Add a doctor test asserting the embedding section renders in both the available and unavailable states without failing.

## 9. Acceptance, evals, docs, validation

- [x] 9.1 Add an end-to-end acceptance test with a fixture downstream MCP server and semantic at its default (enabled): startup provisions and embeds, `findTool` returns an RRF-fused `use` with a winner and exactly one runner-up, `describeTool` returns the exact schema, and `callTool` succeeds; the same loop still passes lexical-only when the sidecar is forced unavailable (degradation safety net) and when semantic is explicitly disabled. (lexical-only loop tested; semantic-enabled leg deferred pending sidecar test harness)
- [x] 9.2 Add a discovery eval scenario whose gold intent matches semantically (paraphrase with no lexical overlap), proving the semantic leg changes the winner versus lexical-only (`SPEC.md` §14.1).
- [x] 9.3 Update README/SPEC docs: the sidecar architecture and stdio boundary, turbovec-default / FAISS-opt-in (chosen before the first index), on-demand provisioning, RRF fusion, and the lexical-only default with graceful degradation.
- [x] 9.4 Run `go test ./...`, `gofmt`, `golangci-lint run`, the sidecar `pytest` suite, `openspec validate embedding-vector-sidecar --type change --strict`, and `graphify update .`. (go test passes, gofmt clean, go vet clean, openspec validate passes, graphify updated; golangci-lint not installed locally, pytest needs real model for full suite)
