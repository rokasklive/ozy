## MODIFIED Requirements

### Requirement: Deterministic stub behavior

Corpus tools SHALL be deterministic — the same valid input SHALL return a
byte-identical response — and each tool SHALL behave like the real service it
mirrors. A tool mirroring a **no-auth** service that shares a scenario's required
capability MAY be **functional**, returning real captured data (which for a
search/lookup tool legitimately includes task-relevant facts, because that is the
real service doing its job). A tool mirroring an **auth-gated** service SHALL return
a realistic authentication-required error rather than a schema-plausible success, so
that a keyless agent's pick fails legibly and it routes to a working tool. A tool
whose capability is **unrelated** to any required task step SHALL return a generic
schema-plausible stub and SHALL NOT contain the ground-truth facts of any scenario,
so a genuinely-wrong-capability path cannot fake success.

#### Scenario: Responses are deterministic

- **WHEN** the same tool is called twice with the same arguments
- **THEN** both calls return byte-identical responses

#### Scenario: Auth-gated distractor errors legibly

- **WHEN** an agent calls an auth-gated distractor (e.g. `brave-search`) with no credentials configured
- **THEN** it returns a realistic authentication-required error, not an empty success, so the agent must route to a working tool

#### Scenario: Wrong-capability tools cannot fake success

- **WHEN** an agent answers the task using only unrelated-capability tools
- **THEN** the answer cannot contain the scenario's ground-truth facts, and grading fails the run

### Requirement: Distractor families

The corpus SHALL contain deliberate near-miss families aligned against the canonical
scenario capabilities — at least 3 rival search tools, at least 3 weather/climate
lookalike tools, and at least 3 document/PDF rival tools — each plausibly described.
Within a family, each rival SHALL behave like the real service it mirrors: a no-auth
rival is functional, an auth-gated rival returns an authentication-required error —
so that under outcome grading a rival pick is either usable or legibly unusable,
never a silent dead end.

#### Scenario: Near misses exist and behave realistically

- **WHEN** the corpus validation test inspects the corpus
- **THEN** it finds at least 3 rivals in each of the search, weather, and document/PDF families, none of which is a canonical scenario tool, and each rival either functions (no-auth) or returns an authentication-required error (auth-gated)
