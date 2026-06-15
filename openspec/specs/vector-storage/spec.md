# vector-storage

## Purpose

Define Ozy's pluggable vector storage backends for the semantic search path:
turbovec as the zero-configuration default, FAISS as an opt-in alternative
selected before the first index, backend immutability guarantees, facet-scoped
nearest-neighbor search, and index persistence with full rebuildability from the
SQLite embedding store (`SPEC.md` §10.4).

## Requirements

### Requirement: turbovec is the default vector backend

When semantic search is enabled and no vector backend is explicitly configured, Ozy SHALL use turbovec as the vector index without requiring any user action or vector-storage configuration. The default path SHALL provision and initialize turbovec automatically, so a user who only enables semantic search is never required to choose or configure vector storage.

#### Scenario: Enabling semantic search uses turbovec with no extra setup

- **WHEN** a user enables semantic search and does not configure a vector backend
- **THEN** Ozy provisions turbovec as the vector index automatically and begins embedding and serving semantic search without further configuration

### Requirement: FAISS is an opt-in alternative chosen before indexing

Ozy SHALL allow a user to select FAISS instead of turbovec by setting the vector backend in configuration before the first index is built. When FAISS is selected, Ozy SHALL use FAISS as the vector index for embedding and search in place of turbovec, and SHALL require FAISS only on that opt-in path.

#### Scenario: FAISS selected before first index is honored

- **WHEN** a user sets the vector backend to `faiss` before any index has been built and then indexes
- **THEN** Ozy builds and serves the semantic index with FAISS rather than turbovec

#### Scenario: Default remains turbovec when FAISS is not selected

- **WHEN** a user enables semantic search without selecting `faiss`
- **THEN** Ozy uses turbovec and does not require FAISS to be installed

### Requirement: Vector backend is immutable after the first index

Once an index has been built with a given vector backend, Ozy SHALL treat that backend as fixed for the existing index. Changing the configured backend SHALL NOT silently reinterpret an index built by the other backend; Ozy SHALL require a rebuild (reindex) to adopt the newly selected backend and SHALL surface this rather than mixing incompatible artifacts.

#### Scenario: Switching backend requires a reindex

- **WHEN** a user changes the configured vector backend after an index already exists
- **THEN** Ozy detects the mismatch and rebuilds the index under the new backend (or instructs the user to reindex) instead of reading the existing index with the wrong backend

### Requirement: Facet-scoped nearest-neighbor search

The vector backend SHALL support restricting a nearest-neighbor search to a candidate set derived from the SQLite facet filter (an allowlist of vector ids), so filtered semantic queries return only matching tools without over-fetching and post-filtering. Vector ids SHALL map back to `toolRef` through the SQLite store.

#### Scenario: Allowlist restricts the search inside the backend

- **WHEN** a semantic query carries a facet filter that SQLite resolves to a set of allowed vector ids
- **THEN** the backend searches within that allowlist and returns only allowed neighbors, mapped back to their `toolRef`

### Requirement: Persistence and rebuild from the embedding store

The vector index SHALL be persisted to disk so it survives a restart, and SHALL be fully rebuildable from the SQLite embedding store (which retains the raw embeddings) without re-contacting downstream servers. On load, Ozy SHALL verify that the persisted index matches the configured backend, embedding model, and dimension before serving it.

#### Scenario: Index survives restart

- **WHEN** the daemon restarts after an index was built
- **THEN** the persisted vector index is loaded and serves semantic queries without re-embedding the whole catalog

#### Scenario: Index is rebuilt from SQLite when missing or incompatible

- **WHEN** the persisted vector index is absent, or its recorded model, dimension, or backend does not match the current configuration
- **THEN** Ozy rebuilds the index from the SQLite embedding store rather than serving a stale or incompatible index
