## ADDED Requirements

### Requirement: Optional Python embedding sidecar over stdio

Ozy SHALL support an optional Python sidecar process that the Go daemon launches and supervises, communicating over the sidecar's standard input and standard output using newline-delimited JSON request/response messages, with the sidecar's standard error reserved for diagnostic logs. Each request SHALL name an operation (for example `health`, `upsert`, `query`, `delete`) and the sidecar SHALL return a structured response correlated to that request. The sidecar SHALL NOT be required for Ozy to start or to serve lexical search (`SPEC.md` §10.4).

#### Scenario: Daemon and sidecar exchange framed JSON over stdio

- **WHEN** semantic search is enabled and the daemon sends a `health` request on the sidecar's stdin
- **THEN** the sidecar replies on stdout with a single newline-delimited JSON response reporting its embedding model, vector dimension, active vector backend, and ready status

#### Scenario: Malformed or unknown request is reported, not fatal

- **WHEN** the daemon sends a request with an unknown operation or an unparseable body
- **THEN** the sidecar returns a structured error response for that request and continues serving subsequent requests rather than exiting

### Requirement: FastEmbed embeddings stamped with model and dimension

The sidecar SHALL produce embeddings with FastEmbed using a CPU-only ONNX runtime, and SHALL record the embedding model identifier and vector dimension alongside every stored vector so a later model or dimension change is detectable. Document and query embeddings SHALL use the same model.

#### Scenario: Stored vectors carry their model and dimension

- **WHEN** the sidecar embeds and stores a tool's indexed text
- **THEN** the persisted record includes the FastEmbed model id and the vector dimension used to produce it

#### Scenario: Model change forces a rebuild rather than mixing spaces

- **WHEN** the configured embedding model differs from the model recorded for the existing stored vectors
- **THEN** the sidecar treats the existing vectors as stale and rebuilds them rather than serving queries against mixed embedding spaces

### Requirement: Embed-on-index ingestion of cataloged tools

When semantic search is enabled, indexing SHALL send each cataloged tool's indexed text (`SPEC.md` §10.2) to the sidecar as an `upsert` keyed by `toolRef`. The sidecar SHALL skip re-embedding a tool whose content hash is unchanged, embed and store new or changed tools, and SHALL accept a `delete` for tools removed from the catalog so the embedding store tracks the catalog.

#### Scenario: New and changed tools are embedded; unchanged tools are skipped

- **WHEN** an index pass upserts a tool whose indexed text differs from its stored content hash, and another tool whose text is identical to its stored hash
- **THEN** the sidecar embeds and stores the changed tool and reports the unchanged tool as skipped

#### Scenario: Removed tools are deleted from the embedding store

- **WHEN** a tool that was previously embedded is no longer present in the catalog and the daemon issues a `delete` for its `toolRef`
- **THEN** the sidecar removes that tool's vector and metadata so it can no longer be returned by a query

### Requirement: Semantic query returns ranked tool references

Given a query string, a result limit `k`, and an optional facet filter, the sidecar SHALL embed the query, search the vector index, and return up to `k` matches as an ordered list of `toolRef` with a similarity score, best first. When a facet filter is supplied the sidecar SHALL restrict results to tools matching that filter.

#### Scenario: Query returns ordered toolRefs with scores

- **WHEN** the daemon sends a `query` with a capability phrase and a limit `k`
- **THEN** the sidecar returns up to `k` `{toolRef, score}` entries ordered by descending similarity

#### Scenario: Facet filter scopes the result set

- **WHEN** the daemon sends a `query` carrying a facet filter such as a server id
- **THEN** the sidecar returns only tools matching that facet, drawn from the nearest neighbors

### Requirement: Embedding store does not own the authoritative catalog

The sidecar's SQLite store SHALL be the source of truth only for the embedding subsystem — the `toolRef`-to-vector-id mapping, content hashes, the embedding model and version, the raw embeddings, and filterable facets — and SHALL NOT be treated as Ozy's authoritative catalog of tools, schemas, or runtime status, which remain owned by the Go catalog (`SPEC.md` §6.1, §10.4).

#### Scenario: Catalog authority stays with Go

- **WHEN** the sidecar has stored embedding metadata for a tool
- **THEN** Ozy continues to resolve `describeTool` and `callTool` against the Go-owned catalog, not the sidecar's SQLite store

#### Scenario: Embedding store is derived, not authoritative

- **WHEN** the sidecar's SQLite store is deleted while the Go catalog is intact
- **THEN** Ozy still serves lexical search and can re-embed the catalog into a fresh embedding store rather than losing catalog state

### Requirement: Graceful degradation when the sidecar is unavailable

Ozy SHALL treat the embedding sidecar as optional at every step: if Python is absent, provisioning fails, the sidecar cannot start, it crashes, or a request exceeds its timeout, the daemon SHALL mark semantic search unavailable, continue serving `findTool` from the lexical baseline, and surface the degraded mode rather than returning an error (`SPEC.md` §4.10, §10.1).

#### Scenario: Missing Python degrades to lexical-only

- **WHEN** semantic search is enabled but no usable Python runtime or sidecar environment can be provisioned
- **THEN** the daemon starts, `findTool` returns lexical-ranked results, and the response surfaces that semantic search is unavailable

#### Scenario: Sidecar crash mid-session does not fail findTool

- **WHEN** the sidecar process exits or a `query` times out during operation
- **THEN** `findTool` falls back to lexical ranking for that query and surfaces degraded mode rather than propagating a failure
