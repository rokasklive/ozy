## ADDED Requirements

### Requirement: Partial embedding coverage is reported, not silently accepted

When semantic search is enabled and the embedding sidecar is healthy, an index pass that leaves the queryable vector count below the catalog tool count SHALL be reported as a degraded, non-OK outcome with a repair instruction. The system SHALL NOT treat "some vectors present" as success: the loud-fail signal SHALL fire whenever embedding coverage is incomplete, not only when the vector count is exactly zero.

#### Scenario: Indexed-but-undercount embed is flagged

- **WHEN** an index pass completes with the sidecar healthy, the catalog holding N tools, and the embedding store holding fewer than N queryable vectors
- **THEN** the index outcome is reported as degraded/non-OK, names the tool-versus-vector counts, and instructs the user to re-run `ozy index`

#### Scenario: Full coverage reports success

- **WHEN** an index pass completes with the sidecar healthy and the embedding store holding at least N queryable vectors for N catalog tools
- **THEN** the index outcome is reported OK with the embedded and vector counts
