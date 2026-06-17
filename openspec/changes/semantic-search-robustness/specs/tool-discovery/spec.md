## MODIFIED Requirements

### Requirement: `ozy index` populates the catalog

The `ozy index` command SHALL connect to configured servers, discover their tools, write the normalized tools to the catalog, and report a summary of servers indexed and tools discovered (including per-server failures). When semantic search is enabled, `ozy index` SHALL provision the embedding sidecar and embed the indexed tools into vector storage on the same run — it SHALL NOT silently run lexical-only — and the summary SHALL report how many tools were embedded and the resulting vector count alongside the tools-indexed count. When semantic search is enabled and the sidecar is available but no tools were embedded, `ozy index` SHALL fail loudly with the reason and the next command, rather than reporting success on a catalog that was persisted without any vectors. When the sidecar is genuinely unavailable, `ozy index` SHALL still populate the catalog and report lexical-only with the reason, consistent with graceful degradation.

#### Scenario: Indexing reports a summary

- **WHEN** a user runs `ozy index` with at least one reachable configured server
- **THEN** the catalog is populated with the discovered tools and the command reports how many servers were reached and how many tools were indexed, plus any per-server errors

#### Scenario: Indexing with no reachable servers is instructional

- **WHEN** `ozy index` runs but no configured server is reachable
- **THEN** it reports the per-server failures with repair guidance rather than silently succeeding

#### Scenario: Semantic-enabled index embeds and reports vector counts

- **WHEN** semantic search is enabled, the sidecar is available, and `ozy index` indexes downstream tools
- **THEN** those tools are embedded into vector storage and the summary reports the embedded-tool count and resulting vector count, not only the tools-indexed count

#### Scenario: Indexed-but-not-embedded is a loud failure

- **WHEN** semantic search is enabled and the sidecar is available but an index run persists tools to the catalog while embedding zero of them
- **THEN** `ozy index` reports a failure naming the reason and the next command, rather than reporting success with an empty vector store

#### Scenario: Sidecar unavailable still indexes the catalog

- **WHEN** semantic search is enabled but the sidecar cannot be provisioned or started
- **THEN** `ozy index` still populates the catalog, reports lexical-only with the specific reason, and does not fail the catalog update
