## REMOVED Requirements

### Requirement: findTool returns the top match and one runner-up

**Reason**: Superseded — the hard-coded single runner-up ignored the configured `budgets.findTool.maxResults` (a scaffolded knob read by nothing), hid viable candidates below rank 2, and prescribed a `describeTool` hop even when the full schema is small enough to deliver inline.
**Migration**: Replaced by "findTool returns the top match and bounded alternatives" and "Small-schema fast path skips the describe hop" below.

## ADDED Requirements

### Requirement: findTool returns the top match and bounded alternatives

For a non-empty catalog where the top-ranked tool clears the confidence floor and is sufficiently separated from the next candidate, `findTool` SHALL return `decision: use` with the single best tool in `selected` (carrying its `toolRef`, schema information per the fast-path requirement, and live/freshness status), a `confidence`, a `reason` explaining the match, up to `budgets.findTool.maxResults − 1` runner-ups in `alternatives` (each with a `toolRef` and a one-line reason), and a `nextAction`. The `reason` SHALL name at most the highest-signal matched terms (ranked by corpus IDF) so that low-signal stopwords do not appear as evidence.

#### Scenario: A confident query returns the best tool and bounded alternatives

- **WHEN** an agent calls `findTool`, one cataloged tool is the clear best match, and `budgets.findTool.maxResults` is 5
- **THEN** Ozy returns `decision: use`, the `selected` best tool with status, a `confidence`, a `reason`, up to 4 runner-ups in `alternatives`, and a `nextAction`

#### Scenario: The reason lists signal terms, not stopwords

- **WHEN** `findTool` matches a query such as "search the web for recent news"
- **THEN** the `reason` names the highest-IDF matched terms (for example `search`, `web`, `news`) and does not present stopwords such as `the` or `for` as match evidence

#### Scenario: Response stays within the findTool budget

- **WHEN** `findTool` returns a selected tool and alternatives
- **THEN** the total candidates surfaced (selected plus alternatives) do not exceed `budgets.findTool.maxResults`

### Requirement: Small-schema fast path skips the describe hop

When `decision: use` selects a tool whose canonical `inputSchema` encoding is at or under the fast-path size threshold, `findTool` SHALL inline the full `inputSchema` and a `recommendedCall` (a `callTool` argument skeleton derived from the schema's required fields) in `selected`, and SHALL set `nextAction` to `callTool` with guidance that `describeTool` is needed only if the schema is unclear. When the schema exceeds the threshold, `findTool` SHALL return the bounded schema preview and a `nextAction` of `describeTool`, as before.

#### Scenario: A small-schema selection is directly callable

- **WHEN** `findTool` returns `decision: use` for a tool whose full input schema is under the fast-path threshold
- **THEN** the response inlines the complete `inputSchema` and a `recommendedCall`, and `nextAction` directs the agent to `callTool` without an intervening `describeTool`

#### Scenario: A large-schema selection keeps the describe-first flow

- **WHEN** `findTool` returns `decision: use` for a tool whose full input schema exceeds the fast-path threshold
- **THEN** the response carries a bounded schema preview and `nextAction` directs the agent to `describeTool` before invoking

### Requirement: findTool responses expose catalog age

`findTool` responses SHALL include the catalog's age (seconds since the last successful index run) in `catalogStats`, so an agent can weigh how current the reported tool set and per-tool status are.

#### Scenario: Catalog age accompanies every decision

- **WHEN** an agent calls `findTool` against a catalog last indexed some time ago
- **THEN** the response's `catalogStats` reports the age of the catalog alongside the existing tool counts

## MODIFIED Requirements

### Requirement: Ambiguous, no-match, and empty-catalog decisions are explicit

`findTool` SHALL map ranking outcomes to explicit decisions: when the top two candidates are too close to separate confidently it SHALL return `ambiguous` and surface both for the agent to choose; when no candidate clears the confidence floor it SHALL return `no_good_match`; when the catalog has no indexed tools it SHALL return `catalog_empty`. Because the fused ranking is rank-based (RRF) and therefore not an absolute relevance measure, the confidence floor for `no_good_match` SHALL be evaluated against the underlying component scores (the lexical relevance and, when available, the semantic similarity), not against the RRF score alone. Every such response SHALL remain instructional and SHALL NOT instruct the agent to infer that the capability is unavailable. An `ambiguous` response SHALL be self-consistent: because it already inlines the close candidates' schemas, its instruction SHALL direct the agent to compare the inlined candidates and call the chosen one, not to re-fetch those same schemas with `describeTool`.

#### Scenario: Closely-ranked candidates yield an ambiguous decision

- **WHEN** the two best-ranked tools have fused ranks too close to separate confidently
- **THEN** Ozy returns `decision: ambiguous`, surfaces both candidates with their schemas, and instructs the agent to compare the inlined candidates and invoke the chosen one rather than auto-selecting

#### Scenario: Ambiguous guidance never prescribes re-fetching delivered schemas

- **WHEN** an `ambiguous` response inlines both candidates' input schemas
- **THEN** its `agentInstruction` does not direct the agent to call `describeTool` for those same candidates

#### Scenario: A query below the component-score floor yields no_good_match

- **WHEN** no cataloged tool clears the confidence floor on either the lexical or semantic component score for a query
- **THEN** Ozy returns `decision: no_good_match` with guidance to refine the query or run diagnostics, and does not fabricate a selection

#### Scenario: An empty catalog yields catalog_empty

- **WHEN** an agent calls `findTool` while the catalog has no indexed tools
- **THEN** Ozy returns `decision: catalog_empty`, instructs the agent not to infer the capability is unavailable, and directs it toward indexing or `ozy doctor` (`SPEC.md` §9.1)
