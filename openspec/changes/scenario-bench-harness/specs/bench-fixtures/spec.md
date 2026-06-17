## ADDED Requirements

### Requirement: Deterministic fixture generation

A generator SHALL build the `acme-billing` fixture reproducibly: fixed file
content, fixed git author/committer identity and commit dates, and fixed commit
order, so repeated generation on the same generator version yields the same tree
and the same culprit commit identity. The generator SHALL record the resolved
culprit commit hash after generation.

#### Scenario: Regeneration is reproducible

- **WHEN** the generator is run twice
- **THEN** both produce an identical file tree and the same culprit commit subject, and the resolved culprit hash is recorded for the run

#### Scenario: Fixture needs no build system

- **WHEN** the fixture is generated
- **THEN** its source files are present to be read and searched but are never compiled or executed by the harness (no Java build/run step)

### Requirement: Real git history with a deterministic culprit commit

The fixture SHALL contain multiple commits, including one culprit commit that
introduces the bug by normalizing an external `"SUSPENDED"` status to `ACTIVE`
(correct behavior maps `SUSPENDED → SUSPENDED`), plus unrelated commits that act
as history distractors.

#### Scenario: Culprit commit is identifiable in history

- **WHEN** the fixture git log is inspected
- **THEN** exactly one commit carries the expected culprit subject and its diff changes the `SUSPENDED` mapping to `ACTIVE`

#### Scenario: Unrelated commits add realistic noise

- **WHEN** the history is inspected
- **THEN** it contains commits unrelated to the bug (docs, cleanup) so identifying the culprit requires evidence, not guessing the only change

### Requirement: Source, docs, and incident database content

The fixture SHALL contain the billing source (an account-status enum, a status
mapper holding the bug, an invoice-eligibility service, a billing-run processor),
a test file, docs that state plainly that **suspended accounts are not invoice
eligible**, and a read-only incident SQLite database with rows showing suspended
accounts that received invoices on the incident date.

#### Scenario: Docs assert the eligibility rule

- **WHEN** the account-status lifecycle doc is read
- **THEN** it states that SUSPENDED accounts are not invoice eligible

#### Scenario: Incident database evidences the regression

- **WHEN** the incident database is queried
- **THEN** it returns rows for suspended accounts that were incorrectly invoiced on the 2026-06-14 nightly run

### Requirement: Rigid task prompt

The scenario SHALL provide a rigid task prompt that states the incident, enumerates
the five required outputs (root cause; exact source file and function; culprit git
commit; minimal patch; regression test), and states the rules (use only the local
fixture/docs/git/incident DB; no public internet; no system redesign; no broad
refactor; no speculation beyond evidence).

#### Scenario: Prompt constrains the agent

- **WHEN** the task prompt is loaded
- **THEN** it contains the five required answer items and the rule set discouraging tangents, internet use, redesign, and broad refactors

### Requirement: Machine-checkable ground truth

`expected/ground_truth.json` SHALL declare `root_cause_file`,
`root_cause_function`, `culprit_commit_subject`, `expected_test`,
`expected_patch_file`, and `forbidden_behaviors`, each referencing real fixture
paths/symbols; the generator SHALL augment the run with the resolved culprit hash.

#### Scenario: Ground truth points at the real fixture

- **WHEN** `ground_truth.json` is validated against the generated fixture
- **THEN** its file path, function name, expected test, and patch file all exist in the fixture and the culprit subject matches a real commit

#### Scenario: Forbidden behaviors are declared

- **WHEN** `ground_truth.json` is read
- **THEN** `forbidden_behaviors` lists public web search, architecture redesign, and unrelated refactor
