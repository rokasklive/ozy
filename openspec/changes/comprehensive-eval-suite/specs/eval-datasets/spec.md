## ADDED Requirements

### Requirement: Synthetic downstream MCP catalog

The corpus SHALL include a committed, synthetic-but-realistic downstream MCP
catalog — "the world" — of multiple servers and tools, where each tool carries a
real-looking `toolRef`, title, description, and JSON input schema modeled on
actual MCP servers (e.g. Atlassian/Confluence, Jira, GitHub, Slack, Gmail,
calendar, filesystem, code search, database). The catalog MUST be large and
varied enough to make discovery non-trivial, including near-duplicate
capabilities across different servers so wrong-server selection is measurable.

#### Scenario: Catalog is loadable into the broker

- **WHEN** the harness loads the corpus catalog
- **THEN** each entry populates a catalog tool with toolRef, server id, title, description, and input schema, indexable by the lexical and semantic legs

#### Scenario: Catalog contains wrong-server traps

- **WHEN** the catalog is inspected
- **THEN** it contains at least two servers exposing a similarly-named capability (e.g. a "search" tool on more than one server) so wrong-server rate is meaningful

### Requirement: Labeled discovery gold sets

The corpus SHALL include labeled discovery gold sets mapping representative user
intents to the acceptable target tool(s) and server(s), as `SPEC.md` §14
requires. The gold sets MUST cover, as tagged categories: lexical-overlap
intents, semantic paraphrase intents with little or no lexical overlap, no-match
intents whose capability is absent, ambiguous intents with more than one
acceptable target, and wrong-server trap intents. Each label MUST record a short
rationale.

#### Scenario: Categories are represented

- **WHEN** the discovery gold sets are loaded
- **THEN** each of the lexical, semantic-paraphrase, no-match, ambiguous, and wrong-server categories has at least one labeled intent

#### Scenario: Semantic paraphrase intents have no lexical shortcut

- **WHEN** a semantic-paraphrase intent is evaluated by the lexical-only ranker
- **THEN** the lexical baseline does not already pick the expected target by term overlap alone, so the semantic leg has measurable effect

#### Scenario: Each label carries a rationale

- **WHEN** a gold label is inspected
- **THEN** it records the acceptable target(s) and a short human-readable reason the target is correct

### Requirement: Invocation and failure scenario sets

The corpus SHALL include invocation scenario sets that exercise the `callTool`
contract and its failure modes: valid-argument calls, invalid-argument calls
paired with the expected corrected call, schema-drift fixtures (a cataloged
schema that no longer matches the downstream schema), and offline-server
fixtures. Each scenario MUST declare its expected outcome (success, or a specific
structured error type).

#### Scenario: Valid and invalid invocation pairs exist

- **WHEN** the invocation scenarios are loaded
- **THEN** at least one valid-argument success case and one invalid-argument case with its expected corrected call are present per exercised tool family

#### Scenario: Failure fixtures declare expected error types

- **WHEN** a schema-drift or offline-server scenario is loaded
- **THEN** it declares the expected structured error type (e.g. `TOOL_SCHEMA_CHANGED`, `DOWNSTREAM_SERVER_OFFLINE`) the harness must observe

### Requirement: Dataset schema and validation

All corpus data SHALL conform to a documented, versioned schema, and the harness
SHALL validate every dataset file against it before use. The schema MUST be
explicit enough that a contributor can add a scenario by following it without
reading harness code.

#### Scenario: Datasets validate against the schema

- **WHEN** the corpus is loaded
- **THEN** every catalog, gold-set, and scenario file is validated against the documented schema and rejected with a precise error if non-conforming

### Requirement: Gold-set provenance and hygiene

The corpus SHALL document the provenance of each dataset and the hygiene rules
that keep the gold sets honest: queries MUST NOT be authored to trivially echo a
tool's exact name where the category claims to test semantic matching, labels MUST
be justified, and the rules for adding, changing, or retiring labels MUST be
recorded so the gold set can evolve without silently drifting into a test the
system is guaranteed to pass.

#### Scenario: Provenance and rules are documented

- **WHEN** a contributor opens the dataset documentation
- **THEN** it states where the corpus came from, how to add a labeled scenario, and the hygiene rules that prevent leakage between the corpus text and the query text
