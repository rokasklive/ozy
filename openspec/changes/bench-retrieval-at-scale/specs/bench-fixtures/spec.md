## MODIFIED Requirements

### Requirement: Machine-checkable ground truth

`expected/ground_truth.json` SHALL be a declarative check specification the
generic grader executes: `answer_must_contain[]` (strings the final answer must
include), optional `commit_check` (`subject` plus the fixture-recorded hash),
`required_tools[]` (`{server, tool}` pairs that must appear in the run's
invocation log), `forbidden_tool_patterns[]` (substring patterns no called tool
may match), and `artifacts[]` (`{path, type, must_contain[]}` checks against
files the agent produced). Each scenario SHALL express all of its criteria as
this data; no scenario-specific check logic SHALL exist in the grader. For the
acme-billing scenario, the previous hardcoded criteria (root-cause file,
function, culprit commit, expected test, patch file, distractor/web/refactor
bans) SHALL be expressed 1:1 in its ground-truth file with unchanged grading
outcomes.

#### Scenario: Ground truth points at the real fixture

- **WHEN** `ground_truth.json` is validated against the generated fixture
- **THEN** every `answer_must_contain` entry, required tool, and artifact reference corresponds to something the fixture actually provides, and the commit check matches a real commit

#### Scenario: Checks are data, not code

- **WHEN** a new scenario adds a new kind of expectation expressible in the declarative fields
- **THEN** no grader code changes are needed

#### Scenario: Old scenario grades identically

- **WHEN** the acme-billing scenario is graded on the same transcripts before and after the migration
- **THEN** the per-criterion outcomes and overall verdict are unchanged

## ADDED Requirements

### Requirement: Historical-weather-report scenario fixture

The bench SHALL ship a `historical-weather-report` scenario: a fixed city and
fixed past date (Vilnius, Lithuania, 2024-07-15), a baked weather dataset
holding the real archive values for that city/date (captured at authoring time),
a baked search-result corpus and two condensed wikipedia articles containing the
city facts, a task prompt requiring the agent to (1) find the historical weather
for the date, (2) research the city via a privacy-focused search engine — not
named directly — and Wikipedia, and (3) produce `output/report.pdf` containing
the weather values and referenced facts. Canonical facts SHALL exist only in the
canonical servers' baked data.

#### Scenario: Task is repeatable by construction

- **WHEN** the scenario is run on different days or machines
- **THEN** the weather values, search results, and article content the agent can obtain are identical

#### Scenario: Deliverable is a real PDF

- **WHEN** a run succeeds
- **THEN** `output/report.pdf` exists in the run's workspace, parses as PDF, and its extracted text contains the ground-truth weather values and city facts

#### Scenario: Wrong tools cannot satisfy the task

- **WHEN** an agent uses only rival search/weather/PDF stubs from the corpus
- **THEN** the obtainable content lacks the ground-truth facts and grading fails the run

### Requirement: Retrieval ground truth

The `historical-weather-report` ground truth SHALL require the four canonical
servers (`weather`, `duckduckgo`, `wikipedia`, `pdf-toolkit`) via
`required_tools`, so grading distinguishes "task answered" from "task answered
with the right tools".

#### Scenario: Canonical-tool usage is graded

- **WHEN** a run produces a correct-looking PDF without ever calling the canonical weather tool
- **THEN** the `required_tools` check fails and the run does not pass overall
