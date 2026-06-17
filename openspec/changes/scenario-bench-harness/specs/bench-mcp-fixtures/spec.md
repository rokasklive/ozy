## ADDED Requirements

### Requirement: Parameterized fixture MCP server

A single fixture server binary SHALL expose a named toolset selected at launch
(`code-search`, `git`, `incident-db`, `filesystem`, `time`, `memory`, `notes`),
each presenting its own MCP tools over stdio, so launching the binary multiple
times with different toolsets yields multiple distinct MCP server surfaces.

#### Scenario: Toolset selects the advertised tools

- **WHEN** the server is launched with `--toolset code-search`
- **THEN** it advertises the code-search tools (`search_text`, `search_symbol`, `read_file`, `find_references`) and no others

#### Scenario: Many surfaces from one binary

- **WHEN** the binary is launched once per toolset for the scenario's server set
- **THEN** each launch is an independent MCP server surface with its own tool list

### Requirement: Useful capability surfaces over the fixture

The `code-search`, `git`, `incident-db`, and `filesystem` toolsets SHALL answer
deterministically over the mounted fixture: code search surfaces the status-mapper
source, git tools surface the culprit commit and its diff, the incident database
returns the suspended-invoice rows, and filesystem reads the fixture docs/source.

#### Scenario: Code search finds the bug site

- **WHEN** `search_text` is called for `SUSPENDED`
- **THEN** the result includes the status-mapper source file that holds the bug

#### Scenario: Git tools surface the culprit

- **WHEN** `git_log` and `git_show` are used over the fixture
- **THEN** the culprit commit and its `SUSPENDED → ACTIVE` diff are returned

### Requirement: Read-only incident database

The `incident-db` toolset SHALL expose `list_tables`, `describe_table`, and
`query_readonly`, and MUST reject any statement that is not a read
(`SELECT`/`PRAGMA`/`EXPLAIN`) with a structured error, leaving the database
unchanged.

#### Scenario: Reads are allowed

- **WHEN** `query_readonly` runs a `SELECT` over the incident table
- **THEN** it returns the matching rows

#### Scenario: Writes are rejected

- **WHEN** `query_readonly` is given an `INSERT`, `UPDATE`, `DELETE`, or `DROP`
- **THEN** the call returns a structured error and the database is unchanged

### Requirement: Distractor surfaces

The `time`, `memory`, and `notes` toolsets SHALL present plausible-but-irrelevant
tools (`current_time`/`convert_timezone`, `search_memory`/`store_memory`,
`create_plan`/`append_note`) that are functional but never required to solve the
scenario.

#### Scenario: Distractors are tempting but unnecessary

- **WHEN** the distractor toolsets are exposed alongside the useful ones
- **THEN** their tools are advertised and callable, yet none is needed to satisfy the ground truth, so calling them counts against tool-use focus

### Requirement: Dual exposure direct and brokered

The same fixture server set SHALL be exposable unchanged both directly to the agent
(direct mode) and as Ozy downstream servers (ozy mode).

#### Scenario: Identical servers, two wirings

- **WHEN** the scenario's server set is used in `direct` and then in `ozy` mode
- **THEN** the identical server commands are wired directly to the agent in direct mode and configured as Ozy downstream in ozy mode, with no change to the servers themselves
