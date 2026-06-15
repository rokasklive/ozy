## MODIFIED Requirements

### Requirement: Hybrid ranking over the persistent catalog

Ozy SHALL rank cataloged tools for a `findTool` capability query by combining a mandatory lexical relevance signal with an optional semantic relevance signal into a single explainable ranking (`SPEC.md` §10.3). Ranking SHALL operate over the persistent catalog's indexed fields — server id, server labels, downstream tool name, title, description, input-schema field names and descriptions, annotations, examples, and capability aliases (`SPEC.md` §10.2) — and SHALL NOT require connecting to downstream servers to produce a result. When a semantic signal is available, the lexical and semantic rankings SHALL be combined with Reciprocal Rank Fusion (RRF), so the two signals are fused by rank rather than by mixing their incomparable raw scores. The semantic ranking SHALL be produced by the embedding sidecar.

#### Scenario: The most relevant cataloged tool ranks first

- **WHEN** an agent calls `findTool` with a capability query and the catalog contains a tool whose indexed fields clearly match that intent
- **THEN** Ozy ranks that tool ahead of less relevant cataloged tools using the lexical baseline, without connecting to any downstream server

#### Scenario: Lexical and semantic signals are fused with RRF when semantic is available

- **WHEN** semantic search is enabled and the embedding sidecar returns a semantic ranking for a `findTool` query
- **THEN** Ozy combines the lexical and semantic rankings with Reciprocal Rank Fusion into one ranking rather than using either signal alone or mixing their raw scores

### Requirement: Ambiguous, no-match, and empty-catalog decisions are explicit

`findTool` SHALL map ranking outcomes to explicit decisions: when the top two candidates are too close to separate confidently it SHALL return `ambiguous` and surface both for the agent to choose; when no candidate clears the confidence floor it SHALL return `no_good_match`; when the catalog has no indexed tools it SHALL return `catalog_empty`. Because the fused ranking is rank-based (RRF) and therefore not an absolute relevance measure, the confidence floor for `no_good_match` SHALL be evaluated against the underlying component scores (the lexical relevance and, when available, the semantic similarity), not against the RRF score alone. Every such response SHALL remain instructional and SHALL NOT instruct the agent to infer that the capability is unavailable.

#### Scenario: Closely-ranked candidates yield an ambiguous decision

- **WHEN** the two best-ranked tools have fused ranks too close to separate confidently
- **THEN** Ozy returns `decision: ambiguous`, surfaces both candidates, and instructs the agent to inspect them with `describeTool` or choose between them rather than auto-selecting

#### Scenario: A query below the component-score floor yields no_good_match

- **WHEN** no cataloged tool clears the confidence floor on either the lexical or semantic component score for a query
- **THEN** Ozy returns `decision: no_good_match` with guidance to refine the query or run diagnostics, and does not fabricate a selection

#### Scenario: An empty catalog yields catalog_empty

- **WHEN** an agent calls `findTool` while the catalog has no indexed tools
- **THEN** Ozy returns `decision: catalog_empty`, instructs the agent not to infer the capability is unavailable, and directs it toward indexing or `ozy doctor` (`SPEC.md` §9.1)

### Requirement: Graceful degradation from semantic to lexical search

`findTool` SHALL produce a ranked decision using the lexical baseline alone when semantic search is disabled, when the embedding sidecar is not provisioned or healthy, or when a semantic query fails or times out. Degradation SHALL be surfaced explicitly and SHALL NOT cause `findTool` to fail (`SPEC.md` §4.10, §10.1).

#### Scenario: Semantic disabled falls back to lexical ranking

- **WHEN** semantic search is disabled in configuration
- **THEN** `findTool` returns a ranked decision from the lexical baseline without error

#### Scenario: Sidecar unavailable is surfaced, not failed

- **WHEN** semantic search is enabled but the embedding sidecar is not provisioned or not healthy
- **THEN** `findTool` still returns a lexical-ranked decision and surfaces that semantic search was unavailable (degraded mode) rather than returning a hard failure

#### Scenario: A failed semantic query degrades for that query

- **WHEN** the embedding sidecar is healthy but a specific `query` request errors or times out
- **THEN** `findTool` returns the lexical-ranked decision for that query and surfaces degraded mode rather than propagating the failure
