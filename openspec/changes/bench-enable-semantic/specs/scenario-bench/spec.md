## MODIFIED Requirements

### Requirement: Hermetic runtime

After image build, a benchmark run SHALL perform no network access other than
OpenCode's model gateway for the selected model: the agent binary, ozy, ozy-bench,
and any state the agent needs to start are all resolved at build time and pinned.
This includes ozy's semantic retrieval stack — the FastEmbed virtualenv and the
embedding model SHALL be provisioned and cached into the image at build time, and
the runtime SHALL be configured for offline model use (e.g. `HF_HUB_OFFLINE`), so
that `ozy index` builds a vector index at run time with no embedder install and no
model-hub fetch. Hermeticity SHALL be enforced at build time by a smoke check that
fails the image build if the agent cannot reach its ready-to-run state — including
a semantic `ozy index` — without package-registry or model-hub access.

#### Scenario: No runtime package fetches

- **WHEN** a live run executes with the npm registry and model hub unreachable
- **THEN** the agent starts from build-time state, `ozy index` builds its vector index from the baked venv and model, and the run completes normally with no package or model fetches

#### Scenario: Broken build-time state fails the build, not the run

- **WHEN** an OpenCode version bump changes what the agent needs at startup
- **THEN** the image build fails with a named error before any benchmark is attempted

#### Scenario: Missing baked embedder fails the build

- **WHEN** the semantic venv or embedding model is not present in the built image
- **THEN** the build-time smoke check fails the image build with a named error, rather than letting a run silently fall back to lexical retrieval

## ADDED Requirements

### Requirement: Ozy exercises semantic retrieval

Ozy mode SHALL run ozy's default hybrid (semantic + lexical) retrieval, not a
lexical-only fallback: `ozy index` SHALL build a vector index from the baked
embedder, and `findTool` ranking SHALL use it. The retrieval stack the run
actually exercised SHALL be recorded as `semantic`.

#### Scenario: Index is semantic, not lexical fallback

- **WHEN** `setupOzy` runs `ozy index` in the built image
- **THEN** the index reports a non-zero vector count and the recorded retrieval stack is `semantic`

### Requirement: Unambiguous scenario tool-selection criterion

A scenario grading on a specific required tool MUST make that tool uniquely
identifiable in its task when the corpus holds near-synonym distractor tools —
naming it, or describing it so it matches no distractor — so the
`required_tools` criterion is winnable and does not confound the semantic-vs-lexical
retrieval delta. Specifically, `historical-weather-report` SHALL require the
DuckDuckGo search tool by name rather than the shared "a privacy-focused search
engine" paraphrase that also matches the `brave-search`/`kagi-search` distractors.

#### Scenario: Weather scenario names its search tool

- **WHEN** the `historical-weather-report` task instructs the agent to research the city
- **THEN** it names DuckDuckGo as the required search tool, and `expected/ground_truth.json` still requires `{ server: duckduckgo, tool: search }`

#### Scenario: Distractor pick is not the default outcome

- **WHEN** the agent selects a search tool for the research step
- **THEN** the criterion distinguishes `duckduckgo/search` from the near-synonym `brave-search`/`kagi-search` distractors, so a correct pick is achievable rather than an unwinnable near-duplicate collision
