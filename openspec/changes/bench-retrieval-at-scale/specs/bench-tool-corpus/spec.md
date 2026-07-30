## ADDED Requirements

### Requirement: Corpus size and realism floor

The bench SHALL ship a checked-in tool corpus of at least 500 tools across at
least 25 fixture MCP servers. Every corpus tool SHALL have a production-grade
name, a 1–3 sentence description in realistic product tone, and a valid JSON
input schema in which every property is typed and described. Where a real public
MCP server's surface is documented, the corpus SHALL mirror it and record the
mirror source; synthesized servers SHALL record `synthesized` as their source.

#### Scenario: Floor is enforced by test

- **WHEN** the corpus validation test loads `bench/corpus/`
- **THEN** it fails if the corpus has fewer than 500 tools or fewer than 25 servers, if any tool lacks a description or a valid schema, if any schema property lacks a type or description, or if tool names collide within a server

#### Scenario: Mirror provenance is recorded

- **WHEN** a corpus server file is read
- **THEN** it names its mirror source (a real server reference with capture date, or `synthesized`)

### Requirement: Deterministic stub behavior

Corpus tools SHALL be served as stubs that return a deterministic,
schema-plausible canned response for any valid input, and SHALL never return an
error for input that satisfies their schema. Stub responses SHALL NOT contain
the ground-truth facts of any scenario, so a wrong-tool path cannot accidentally
satisfy grading.

#### Scenario: Stub calls are deterministic

- **WHEN** the same stub tool is called twice with the same arguments
- **THEN** both calls return byte-identical responses

#### Scenario: Stubs cannot fake task success

- **WHEN** an agent answers the task using only stub responses
- **THEN** the answer cannot contain the scenario's ground-truth facts, and grading fails the run

### Requirement: Distractor families

The corpus SHALL contain deliberate near-miss families aligned against the
canonical scenario tools: at least 3 rival search tools, at least 3
weather/climate lookalike tools, and at least 3 document/PDF rival tools, each
plausibly described so that retrieval must select the exact canonical tool
rather than its neighborhood.

#### Scenario: Near misses exist for each canonical family

- **WHEN** the corpus validation test inspects the corpus
- **THEN** it finds at least 3 rival tools in each of the search, weather, and document/PDF families, none of which is a canonical scenario tool

### Requirement: Identical-environment attachment

When a scenario attaches the corpus (the default), the full corpus SHALL be part
of both modes' environments: direct mode advertises every corpus tool to the
agent at startup, and ozy indexes every corpus tool into its catalog. The
harness SHALL NOT reduce the corpus for one mode.

#### Scenario: Ozy indexes the full estate

- **WHEN** an ozy-mode run starts with the corpus attached
- **THEN** `ozy index` reports at least 500 + (scenario functional) tools in the per-run catalog

#### Scenario: Direct mode faces the same estate

- **WHEN** a direct-mode run starts with the corpus attached
- **THEN** the generated agent config wires every corpus server, and the surface tier counts the same tool set ozy indexed

### Requirement: Corpus is data, not code

Each corpus server SHALL be defined by a data file under `bench/corpus/` and
served by the generic corpus server mode of the fixture binary. Adding a corpus
server SHALL require only adding a data file — no Go changes.

#### Scenario: New server is a data change

- **WHEN** a new `bench/corpus/<server>.json` file is added
- **THEN** the next invocation serves, indexes, and counts its tools with no code change
