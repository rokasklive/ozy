## MODIFIED Requirements

### Requirement: Parameterized fixture MCP server

A single fixture server binary SHALL expose a named toolset selected at launch
(`code-search`, `git`, `incident-db`, `filesystem`, `time`, `memory`, `notes`,
`weather`, `duckduckgo`, `wikipedia`, `pdf-toolkit`), each presenting its own
MCP tools over stdio, so launching the binary multiple times with different
toolsets yields multiple distinct MCP server surfaces. The same binary SHALL
also serve any corpus server via a generic corpus mode
(`--server <name> --corpus-dir <dir>`) that loads the server's tool definitions
from data.

#### Scenario: Toolset selects the advertised tools

- **WHEN** the server is launched with `--toolset code-search`
- **THEN** it advertises the code-search tools (`search_text`, `search_symbol`, `read_file`, `find_references`) and no others

#### Scenario: Many surfaces from one binary

- **WHEN** the binary is launched once per toolset for the scenario's server set
- **THEN** each launch is an independent MCP server surface with its own tool list

#### Scenario: Corpus mode serves data-defined servers

- **WHEN** the binary is launched with `--server salesforce-crm --corpus-dir bench/corpus`
- **THEN** it advertises exactly the tools defined in that server's corpus file and serves their canned responses

## ADDED Requirements

### Requirement: Mirrored real-service toolsets

The `weather`, `duckduckgo`, `wikipedia`, and `pdf-toolkit` toolsets SHALL
mirror the tool surfaces of their real public counterparts (weather-mcp,
agenticmarket/duckduckgo, agenticmarket/wikipedia, AryanBV/pdf-toolkit-mcp):
tool names, descriptions, and input schemas pinned from the real servers at
authoring time, with the mirror source and capture date recorded. Mirrored tools
the scenario does not need SHALL behave as corpus-grade stubs.

#### Scenario: Surfaces match the mirrored servers

- **WHEN** the `weather` toolset is enumerated
- **THEN** it advertises the mirrored weather-mcp surface (including `get_historical_weather` and `search_location`) with the pinned descriptions and schemas

#### Scenario: Unneeded siblings are stubs

- **WHEN** a mirrored tool outside the scenario's canonical set (e.g. `pdf_encrypt`) is called
- **THEN** it returns a deterministic stub response and logs the invocation, and no scenario ground-truth fact is obtainable from it

### Requirement: Functional task-critical tools

The task-critical tools SHALL be genuinely functional and deterministic:
`get_historical_weather` (and `search_location`) answer from the scenario's
baked weather dataset; the duckduckgo search tool ranks the baked result corpus
by deterministic token overlap and never returns an empty result set; the
wikipedia tools serve the baked articles; `pdf_create`,
`pdf_create_from_markdown`, `pdf_extract_text`, `pdf_get_metadata`, and
`pdf_search` operate on real PDF files, with relative output paths resolved
under `OZY_BENCH_OUTPUT_DIR`.

#### Scenario: Historical weather is answered from baked data

- **WHEN** `get_historical_weather` is called for the scenario's city and date
- **THEN** it returns the baked values (which match ground truth), deterministically across runs

#### Scenario: Search is phrasing-robust

- **WHEN** the duckduckgo search tool is called with two different phrasings about the scenario city
- **THEN** both return deterministic, non-empty results from the baked corpus, ranked by token overlap

#### Scenario: PDF creation writes a real file

- **WHEN** `pdf_create_from_markdown` is called with a relative `outputPath`
- **THEN** a parseable PDF file exists under `OZY_BENCH_OUTPUT_DIR` and `pdf_extract_text` recovers its text content

### Requirement: Server-side invocation logging

Every fixture server (functional toolset and corpus stub alike) SHALL append one
JSONL record `{server, tool, argsDigest, ts}` per tool call to the file named by
`OZY_BENCH_CALL_LOG` when that variable is set, using append-only single-line
writes. When the variable is unset, servers SHALL work normally without logging.

#### Scenario: Calls are recorded server-side

- **WHEN** an agent calls a fixture tool through Ozy's `call_tool`
- **THEN** the invocation log contains a record naming the downstream server and tool, independent of what the agent transcript shows

#### Scenario: Logging is optional

- **WHEN** a fixture server runs without `OZY_BENCH_CALL_LOG`
- **THEN** tool calls succeed and no log file is written
