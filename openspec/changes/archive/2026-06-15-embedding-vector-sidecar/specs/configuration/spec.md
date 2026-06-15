## ADDED Requirements

### Requirement: Vector backend selection

Configuration SHALL allow selecting the semantic vector backend under the `embedding` section, defaulting to `turbovec` when unset and accepting `faiss` as the only alternative. Ozy SHALL validate the value and report a structured `CONFIG_ERROR` for an unknown backend. Ozy SHALL NOT require any vector-storage configuration for the default turbovec path.

#### Scenario: Default backend is turbovec when unset

- **WHEN** configuration enables semantic search but omits the vector backend
- **THEN** Ozy resolves the backend to `turbovec` without requiring any vector-storage configuration

#### Scenario: FAISS backend is accepted

- **WHEN** configuration sets the `embedding` vector backend to `faiss`
- **THEN** Ozy resolves the backend to FAISS for the semantic index

#### Scenario: Unknown backend is rejected

- **WHEN** configuration sets the vector backend to a value other than `turbovec` or `faiss`
- **THEN** Ozy reports a structured `CONFIG_ERROR` naming the invalid backend rather than starting with an undefined backend

### Requirement: Embedding model selection

Configuration SHALL allow selecting the FastEmbed embedding model under the `embedding` section, applying a documented CPU-friendly default when unset. The vector dimension SHALL be derived from the selected model rather than configured separately.

#### Scenario: Default model applied when unset

- **WHEN** configuration enables semantic search but omits the embedding model
- **THEN** Ozy uses the documented default FastEmbed model and derives the vector dimension from it

#### Scenario: Explicit model is honored

- **WHEN** configuration sets an explicit FastEmbed model under `embedding`
- **THEN** Ozy uses that model for both document and query embeddings and derives the dimension from it

### Requirement: Semantic search is enabled by default

The configuration model SHALL treat semantic search as enabled by default: when `search.semantic.enabled` is unset, Ozy SHALL treat semantic search as on and SHALL consult the embedding model and vector-backend settings (defaulting to the FastEmbed default model and turbovec), so the out-of-the-box experience is hybrid search with the sidecar auto-provisioned rather than lexical-only. A user SHALL be able to disable semantic search explicitly, in which case Ozy SHALL run lexical-only and SHALL NOT provision the sidecar. Default-on SHALL NOT make Ozy fail when the sidecar is unavailable: provisioning failures are handled by graceful degradation, not by changing the default.

#### Scenario: Semantic enabled by default when unset

- **WHEN** configuration omits `search.semantic.enabled`
- **THEN** Ozy treats semantic search as enabled and uses the default embedding model and turbovec backend for the semantic path

#### Scenario: Disabling semantic is honored

- **WHEN** configuration sets `search.semantic.enabled` to false
- **THEN** Ozy runs lexical-only and does not provision or launch the embedding sidecar
