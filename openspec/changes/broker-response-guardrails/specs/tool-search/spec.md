## MODIFIED Requirements

### Requirement: Ambiguous, no-match, and empty-catalog decisions are explicit

`findTool` SHALL map ranking outcomes to explicit decisions: when the top two candidates are too close to separate confidently it SHALL return `ambiguous` and surface both for the agent to choose; when no candidate clears the confidence floor it SHALL return `no_good_match`; when the catalog has no indexed tools it SHALL return `catalog_empty`. Because the fused ranking is rank-based (RRF) and therefore not an absolute relevance measure, the confidence floor for `no_good_match` SHALL be evaluated against the underlying component scores (the lexical relevance and, when available, the semantic similarity), not against the RRF score alone. Every such response SHALL remain instructional and SHALL NOT instruct the agent to infer that the capability is unavailable. The `ambiguous` and `no_good_match` decisions SHALL each additionally carry a structured `nextAction` describing the next concrete Ozy call — `ambiguous` directing the agent to call `describeTool` on a surfaced candidate, and `no_good_match` directing the agent to retry `findTool` with a refined query — so an agent can branch on structured fields rather than parsing the `agentInstruction` prose.

#### Scenario: Closely-ranked candidates yield an ambiguous decision

- **WHEN** the two best-ranked tools have fused ranks too close to separate confidently
- **THEN** Ozy returns `decision: ambiguous`, surfaces both candidates, and instructs the agent to inspect them with `describeTool` or choose between them rather than auto-selecting

#### Scenario: A query below the component-score floor yields no_good_match

- **WHEN** no cataloged tool clears the confidence floor on either the lexical or semantic component score for a query
- **THEN** Ozy returns `decision: no_good_match` with guidance to refine the query or run diagnostics, and does not fabricate a selection

#### Scenario: An empty catalog yields catalog_empty

- **WHEN** an agent calls `findTool` while the catalog has no indexed tools
- **THEN** Ozy returns `decision: catalog_empty`, instructs the agent not to infer the capability is unavailable, and directs it toward indexing or `ozy doctor` (`SPEC.md` §9.1)

#### Scenario: Ambiguous decision carries a structured next action

- **WHEN** `findTool` returns `decision: ambiguous`
- **THEN** the response includes a structured `nextAction` pointing the agent at `describeTool` for one of the surfaced candidates, in addition to the instructional prose

#### Scenario: No-good-match decision carries a structured next action

- **WHEN** `findTool` returns `decision: no_good_match`
- **THEN** the response includes a structured `nextAction` directing the agent to retry `findTool` with a refined query, in addition to the instructional prose
