## MODIFIED Requirements

### Requirement: Retrieval-stack provenance

`environment.json` SHALL record which retrieval stack ozy used
(`retrieval: semantic|lexical`), and the comparison SHALL display it. The value
SHALL reflect what the run actually exercised, verified at build time — never
assumed. With the semantic embedder baked into the bench image, the recorded
value SHALL be `semantic`, and the comparison verdict SHALL NOT describe ozy as
lexical-only.

#### Scenario: Retrieval stack is visible

- **WHEN** any invocation completes
- **THEN** `environment.json` names the retrieval stack and `comparison.md` displays it alongside the model ID

#### Scenario: Hermetic image records semantic

- **WHEN** a run executes in the image with the baked semantic embedder
- **THEN** `environment.json` records `retrieval: semantic` and no artifact frames ozy's retrieval as lexical-only
