## MODIFIED Requirements

### Requirement: Machine-checkable ground truth

`expected/ground_truth.json` SHALL be a declarative check specification the generic
grader executes, and run **success** SHALL be determined **only** by result
criteria: `answer_must_contain[]` (strings the final answer must include), optional
`commit_check` (`subject` plus the fixture-recorded hash), `forbidden_answer_patterns[]`
(strings the answer may not contain), and `artifacts[]` (`{path, type, must_contain[]}`
checks against files the agent produced). Tool-usage fields — `required_tools[]` and
`forbidden_tool_patterns[]` — SHALL be recorded as **informational** telemetry only
and SHALL NOT affect the pass/fail decision. A scenario's task prompt SHALL specify
the **capability** a step needs (e.g. "research via a web search engine"), not a
named tool, because success no longer depends on which tool the agent selects. Each
scenario SHALL express its criteria as this data; no scenario-specific check logic
SHALL exist in the grader.

#### Scenario: Ground truth points at the real fixture

- **WHEN** `ground_truth.json` is validated against the generated fixture
- **THEN** every `answer_must_contain` entry and artifact reference corresponds to something the fixture actually provides, and any commit check matches a real commit

#### Scenario: Task completed via an alternative tool still passes

- **WHEN** an agent completes the task using a non-canonical but valid same-capability tool (a different search engine) and the final answer and artifacts satisfy the result criteria
- **THEN** the run passes, regardless of which tool was called

#### Scenario: Result criteria alone gate success

- **WHEN** the final answer or a required artifact is missing a `must_contain` fact
- **THEN** the run fails; and when only `required_tools`/`forbidden_tool_patterns` differ from expectation while all result criteria pass, the run still passes and those tool results are recorded as informational only

## ADDED Requirements

### Requirement: Fixtures captured from real no-auth MCP servers

Fixtures captured from real MCP servers SHALL be baked at generation time and
replayed hermetically. Fixture generation MAY contact the real **no-auth** MCP a
scenario mirrors (e.g. DuckDuckGo, weather-mcp, Wikipedia) with network access at
**generation** time (outside the hermetic runtime) to capture its tool schemas +
representative responses, and SHALL bake the captured data into the corpus/fixtures.
Captured responses SHALL be deterministic at replay (byte-stable) and SHALL NOT
require any network access during a benchmark run.

#### Scenario: Capture happens at generation time only

- **WHEN** a fixture is generated with capture enabled
- **THEN** the real MCP is contacted at generation time and its response is baked into the fixture, and a subsequent benchmark run replays the baked response with the hermetic runtime's network restrictions intact

#### Scenario: Replay is deterministic

- **WHEN** a captured tool is invoked at runtime with the same input twice
- **THEN** it returns the baked response byte-for-byte, with no network access
