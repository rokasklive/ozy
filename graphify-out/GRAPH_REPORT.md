# Graph Report - ozy  (2026-06-12)

## Corpus Check
- 58 files · ~34,671 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 771 nodes · 1056 edges · 56 communities (53 shown, 3 thin omitted)
- Extraction: 95% EXTRACTED · 5% INFERRED · 0% AMBIGUOUS · INFERRED: 51 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `6c6eea50`
- Run `git rev-parse HEAD` and compare to check if the graph is stale.
- Run `graphify update .` after code changes (no API cost).

## Community Hubs (Navigation)
- [[_COMMUNITY_Community 0|Community 0]]
- [[_COMMUNITY_Community 1|Community 1]]
- [[_COMMUNITY_Community 2|Community 2]]
- [[_COMMUNITY_Community 3|Community 3]]
- [[_COMMUNITY_Community 4|Community 4]]
- [[_COMMUNITY_Community 5|Community 5]]
- [[_COMMUNITY_Community 6|Community 6]]
- [[_COMMUNITY_Community 7|Community 7]]
- [[_COMMUNITY_Community 8|Community 8]]
- [[_COMMUNITY_Community 9|Community 9]]
- [[_COMMUNITY_Community 10|Community 10]]
- [[_COMMUNITY_Community 11|Community 11]]
- [[_COMMUNITY_Community 12|Community 12]]
- [[_COMMUNITY_Community 13|Community 13]]
- [[_COMMUNITY_Community 14|Community 14]]
- [[_COMMUNITY_Community 15|Community 15]]
- [[_COMMUNITY_Community 16|Community 16]]
- [[_COMMUNITY_Community 17|Community 17]]
- [[_COMMUNITY_Community 18|Community 18]]
- [[_COMMUNITY_Community 19|Community 19]]
- [[_COMMUNITY_Community 20|Community 20]]
- [[_COMMUNITY_Community 21|Community 21]]
- [[_COMMUNITY_Community 22|Community 22]]
- [[_COMMUNITY_Community 23|Community 23]]
- [[_COMMUNITY_Community 24|Community 24]]
- [[_COMMUNITY_Community 25|Community 25]]
- [[_COMMUNITY_Community 26|Community 26]]
- [[_COMMUNITY_Community 27|Community 27]]
- [[_COMMUNITY_Community 28|Community 28]]
- [[_COMMUNITY_Community 29|Community 29]]
- [[_COMMUNITY_Community 30|Community 30]]
- [[_COMMUNITY_Community 31|Community 31]]
- [[_COMMUNITY_Community 32|Community 32]]
- [[_COMMUNITY_Community 33|Community 33]]
- [[_COMMUNITY_Community 34|Community 34]]
- [[_COMMUNITY_Community 35|Community 35]]
- [[_COMMUNITY_Community 36|Community 36]]
- [[_COMMUNITY_Community 37|Community 37]]
- [[_COMMUNITY_Community 38|Community 38]]
- [[_COMMUNITY_Community 39|Community 39]]
- [[_COMMUNITY_Community 40|Community 40]]
- [[_COMMUNITY_Community 41|Community 41]]
- [[_COMMUNITY_Community 42|Community 42]]
- [[_COMMUNITY_Community 43|Community 43]]
- [[_COMMUNITY_Community 44|Community 44]]
- [[_COMMUNITY_Community 45|Community 45]]
- [[_COMMUNITY_Community 46|Community 46]]
- [[_COMMUNITY_Community 47|Community 47]]
- [[_COMMUNITY_Community 48|Community 48]]
- [[_COMMUNITY_Community 49|Community 49]]
- [[_COMMUNITY_Community 50|Community 50]]
- [[_COMMUNITY_Community 51|Community 51]]
- [[_COMMUNITY_Community 52|Community 52]]
- [[_COMMUNITY_Community 53|Community 53]]
- [[_COMMUNITY_Community 54|Community 54]]
- [[_COMMUNITY_Community 55|Community 55]]

## God Nodes (most connected - your core abstractions)
1. `Config` - 30 edges
2. `SPEC.md — Ozy Living Specification` - 25 edges
3. `Connector` - 20 edges
4. `File` - 15 edges
5. `run()` - 15 edges
6. `T` - 14 edges
7. `Adapter` - 14 edges
8. `NewMemory()` - 13 edges
9. `newBroker()` - 12 edges
10. `Memory` - 12 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Execute()`  [INFERRED]
  cmd/ozy/main.go → internal/cli/cli.go
- `newBroker()` --calls--> `NewSkeleton()`  [INFERRED]
  internal/broker/skeleton_test.go → internal/broker/skeleton.go
- `NewWithStore()` --calls--> `NewSkeleton()`  [INFERRED]
  internal/daemon/daemon.go → internal/broker/skeleton.go
- `newBroker()` --calls--> `NewMemory()`  [INFERRED]
  internal/broker/skeleton_test.go → internal/catalog/memory.go
- `TestDoctorReportsServerHealthAndRedactsSecrets()` --calls--> `NewFile()`  [INFERRED]
  internal/cli/cli_test.go → internal/catalog/file.go

## Import Cycles
- None detected.

## Communities (56 total, 3 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.08
Nodes (32): NewMemory(), TestMemory_EmptyStoreQueriesAreClean(), TestMemory_StatsCountFreshness(), Daemon, New(), NewWithStore(), TestNew_UsesPersistentCatalogStore(), TestNew_WiresBrokerAndStore() (+24 more)

### Community 1 - "Community 1"
Cohesion: 0.13
Nodes (14): Execute(), Daemon, app, Command, Error, Writer, Writer, T (+6 more)

### Community 2 - "Community 2"
Cohesion: 0.07
Nodes (37): BudgetsConfig, CallToolBudget, BudgetsConfig, CallToolBudget, Config, cloneConfig(), cloneServerConfig(), cloneStringMap() (+29 more)

### Community 3 - "Community 3"
Cohesion: 0.07
Nodes (27): computedHash, skillPath, source, sourceType, computedHash, skillPath, source, sourceType (+19 more)

### Community 4 - "Community 4"
Cohesion: 0.12
Nodes (19): Alternative, CallNextAction, CallResult, CatalogStats, DescribeResult, DoctorCheck, DoctorResult, Example (+11 more)

### Community 5 - "Community 5"
Cohesion: 0.16
Nodes (17): CallToolRequest, Error, ErrorEnvelope, NewErrorEnvelope(), NotImplemented(), Broker, CallToolResult, Context (+9 more)

### Community 6 - "Community 6"
Cohesion: 0.16
Nodes (20): Client, Connector, configError(), connectionError(), isOAuthAuthFailure(), scrub(), secretValues(), WithMaxConcurrency() (+12 more)

### Community 7 - "Community 7"
Cohesion: 0.14
Nodes (18): skeleton, NewSkeleton(), CallResult, CatalogStats, ClientSession, DescribeResult, FindResult, Broker (+10 more)

### Community 8 - "Community 8"
Cohesion: 0.14
Nodes (24): Cmd, Connection, blockingTransport, hasEnv(), inMemoryFactory(), resultsByID(), TestConnector_ConnectionErrorExcludesSecretValues(), TestConnector_ConnectsInMemoryServerAndListsTools() (+16 more)

### Community 9 - "Community 9"
Cohesion: 0.16
Nodes (19): Connector, New(), normalizeSchema(), normalizeTool(), scrub(), secretValues(), WithClock(), Indexer (+11 more)

### Community 10 - "Community 10"
Cohesion: 0.18
Nodes (13): NewFile(), TestFile_EmptyStoreQueriesAreClean(), TestFile_OverwriteKeepsValidJSON(), TestFile_PersistedCatalogContainsNoConfigSecrets(), TestFile_WritesAndReloadsCatalog(), indexedToolCounts(), serverHealthChecks(), DoctorCheck (+5 more)

### Community 11 - "Community 11"
Cohesion: 0.12
Nodes (16): 11. Configuration model, 12. Refresh and freshness behavior, 13. Token economy requirements, 15. CLI contract, 16. Security and privacy boundaries, 17. Observability and diagnostics, 1. Purpose of this document, 20. Accepted architectural baseline (+8 more)

### Community 12 - "Community 12"
Cohesion: 0.15
Nodes (16): MODIFIED Requirements, Scenario: JSONC comments and trailing commas are accepted, Scenario: Loading a valid JSONC configuration, Scenario: Local server without a command is rejected, Scenario: Remote server without a url is rejected, Scenario: Unknown server type is rejected, MODIFIED Requirements, Requirement: Configuration discovery and loading (+8 more)

### Community 13 - "Community 13"
Cohesion: 0.13
Nodes (14): Context, D1: CLI framework — `spf13/cobra`, D2: MCP server library — official `modelcontextprotocol/go-sdk`, hidden behind an internal adapter, D3: One broker interface shared by both adapters, D4: In-process broker now, client/server split deferred, D5: Typed results that marshal to §9 contracts, D6: Catalog store as an interface with an in-memory placeholder, D7: Config via `gopkg.in/yaml.v3` with a typed model + redaction (+6 more)

### Community 14 - "Community 14"
Cohesion: 0.28
Nodes (7): File, fileDocument, Context, RWMutex, Server, Stats, Tool

### Community 15 - "Community 15"
Cohesion: 0.13
Nodes (14): configuration, Purpose, Requirements, Requirement: Configuration discovery and loading, Requirement: Configuration validation, Requirement: Environment reference resolution, Requirement: Redaction in diagnostics, Scenario: Diagnostics show redacted configuration (+6 more)

### Community 16 - "Community 16"
Cohesion: 0.13
Nodes (14): daemon-runtime, Purpose, Requirements, Requirement: Catalog store interface placeholder, Requirement: Daemon lifecycle, Requirement: Graceful degradation of optional subsystems, Requirement: Shared in-process broker seam, Scenario: Catalog store seam is present (+6 more)

### Community 17 - "Community 17"
Cohesion: 0.21
Nodes (5): asError(), WriteStarter(), app, Command, Error

### Community 18 - "Community 18"
Cohesion: 0.14
Nodes (13): cli-interface, Purpose, Requirements, Requirement: CLI command surface, Requirement: CLI mirrors broker operations, Requirement: Output formats, Requirement: Structured handling of unimplemented operations, Scenario: All MVP commands are registered (+5 more)

### Community 19 - "Community 19"
Cohesion: 0.14
Nodes (13): Context, D1: JSONC config via `tailscale/hujson`, replacing YAML, D2: opencode `mcp` shape → transport mapping, D3: Environment references use opencode `{env:NAME}` syntax, D4: `internal/downstream` connector, D5: `internal/index` discovery, D6: Persistence as an atomic JSON document store, D7: Per-server isolation and live-gating preserved (+5 more)

### Community 20 - "Community 20"
Cohesion: 0.14
Nodes (13): ADDED Requirements, Requirement: Discover tools via tools/list, Requirement: Discovered tools carry freshness and runtime status, Requirement: List and describe reflect discovered tools, Requirement: `ozy index` populates the catalog, Requirement: Stable toolRef normalization, Scenario: A discovered tool gets a stable toolRef, Scenario: A freshly indexed tool is marked fresh (+5 more)

### Community 21 - "Community 21"
Cohesion: 0.19
Nodes (13): ADDED Requirements, ADDED Requirements, Requirement: Configuration discovery and loading, Requirement: Configuration validation, Requirement: Environment reference resolution, Requirement: Redaction in diagnostics, Scenario: Diagnostics show redacted configuration, Scenario: Explicit config path override (+5 more)

### Community 22 - "Community 22"
Cohesion: 0.15
Nodes (12): ADDED Requirements, Requirement: Catalog store interface placeholder, Requirement: Daemon lifecycle, Requirement: Graceful degradation of optional subsystems, Requirement: Shared in-process broker seam, Scenario: Catalog store seam is present, Scenario: Daemon refuses to start on invalid configuration, Scenario: Daemon shuts down cleanly (+4 more)

### Community 23 - "Community 23"
Cohesion: 0.33
Nodes (6): Memory, Context, RWMutex, Server, Stats, Tool

### Community 24 - "Community 24"
Cohesion: 0.21
Nodes (12): ADDED Requirements, ADDED Requirements, Requirement: CLI command surface, Requirement: CLI mirrors broker operations, Requirement: Output formats, Requirement: Structured handling of unimplemented operations, Scenario: All MVP commands are registered, Scenario: CLI routes through the shared broker (+4 more)

### Community 25 - "Community 25"
Cohesion: 0.27
Nodes (15): ConfigHome(), configHomeFor(), DefaultPath(), TestConfigHomeFallbacks(), TestDefaultPathPrecedence(), TestLoad_ExampleMCPFixture(), TestLoad_MissingEnvVarIsDiagnostic(), TestLoad_MissingFile() (+7 more)

### Community 26 - "Community 26"
Cohesion: 0.17
Nodes (11): ADDED Requirements, Requirement: Connect to local (stdio) servers, Requirement: Connect to remote (HTTP) servers, Requirement: Non-leaky connection errors, Requirement: Per-server isolation, Scenario: Connection failure error is redacted, Scenario: Disabled servers are skipped, Scenario: Local command environment is applied (+3 more)

### Community 27 - "Community 27"
Cohesion: 0.17
Nodes (11): mcp-adapter, Purpose, Requirements, Requirement: Agent-facing tool registration, Requirement: Instructional placeholder responses conform to contracts, Requirement: MCP adapter shares the broker seam, Scenario: Adapter advertises the three stable tools, Scenario: Adapter delegates to the shared broker (+3 more)

### Community 28 - "Community 28"
Cohesion: 0.17
Nodes (11): Requirement: Single binary build, Requirement: Standard repository layout, Requirement: Test and lint tooling, Scenario: Binary exposes the command tree, Scenario: Building from a clean checkout, Scenario: Continuous integration runs build, test, and lint, Scenario: Entry point and internal packages present, Scenario: Tests run green (+3 more)

### Community 29 - "Community 29"
Cohesion: 0.17
Nodes (12): 4.10 Local-first and privacy-respecting defaults, 4.11 Observable and diagnosable behavior, 4.1 Capability brokerage over naive proxying, 4.2 One configuration source of truth, 4.3 Small stable agent surface, 4.4 Persistent searchable capability catalog, 4.5 Instructional responses for agent certainty, 4.6 Live-gated invocation (+4 more)

### Community 30 - "Community 30"
Cohesion: 0.18
Nodes (10): 10. Verification, 1. Module and scaffold, 2. Configuration, 3. Catalog store, 4. Broker seam and contract models, 5. CLI interface, 6. MCP adapter, 7. Daemon runtime (+2 more)

### Community 31 - "Community 31"
Cohesion: 0.40
Nodes (10): newBroker(), TestCallTool_KnownToolReturnsNotImplemented(), TestCallTool_UnknownToolReturnsToolNotFound(), TestDescribeTool_UnknownReturnsToolNotFound(), TestFindTool_EmptyCatalogReturnsCatalogEmpty(), TestFindTool_NonEmptyCatalogReturnsNoGoodMatch(), TestList_EmptyCatalogIsInstructional(), Broker (+2 more)

### Community 32 - "Community 32"
Cohesion: 0.20
Nodes (9): ADDED Requirements, Requirement: Catalog storage holds no secrets, Requirement: Catalog survives restarts, Requirement: Durable catalog store, Requirement: Offline catalog reads, Scenario: A new process sees previously indexed tools, Scenario: Describe works while the server is offline, Scenario: Indexed tools are written to durable storage (+1 more)

### Community 33 - "Community 33"
Cohesion: 0.20
Nodes (9): ADDED Requirements, Requirement: Agent-facing tool registration, Requirement: Instructional placeholder responses conform to contracts, Requirement: MCP adapter shares the broker seam, Scenario: Adapter advertises the three stable tools, Scenario: Adapter delegates to the shared broker, Scenario: Adapter starts over the MCP transport, Scenario: callTool returns a contract-shaped failure (+1 more)

### Community 34 - "Community 34"
Cohesion: 0.20
Nodes (9): Requirement: Single binary build, Requirement: Standard repository layout, Requirement: Test and lint tooling, Scenario: Binary exposes the command tree, Scenario: Building from a clean checkout, Scenario: Continuous integration runs build, test, and lint, Scenario: Entry point and internal packages present, Scenario: Tests run green (+1 more)

### Community 35 - "Community 35"
Cohesion: 0.36
Nodes (7): Freshness, Server, ServerStatus, Stats, Store, Tool, Time

### Community 36 - "Community 36"
Cohesion: 0.25
Nodes (7): Acceptance Note, Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 37 - "Community 37"
Cohesion: 0.25
Nodes (7): 1. Configuration (JSONC + opencode shape), 2. Persistent catalog store, 3. Downstream connection layer, 4. Tool discovery / indexing, 5. CLI, broker, and doctor wiring, 6. Docs and spec note, 7. Verification

### Community 38 - "Community 38"
Cohesion: 0.29
Nodes (6): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 39 - "Community 39"
Cohesion: 0.33
Nodes (5): Agent interface, Build, Configuration, ozy, Usage

### Community 40 - "Community 40"
Cohesion: 0.33
Nodes (6): 19.1 Contract gate, 19.2 Context gate, 19.3 Runtime gate, 19.4 Safety gate, 19.5 Eval gate, 19. Quality gates

### Community 41 - "Community 41"
Cohesion: 0.40
Nodes (5): 10.1 Baseline requirement, 10.2 Indexed fields, 10.3 Hybrid search, 10.4 Embedding/indexing architecture, 10. Search behavior

### Community 42 - "Community 42"
Cohesion: 0.50
Nodes (4): 14.1 Required scenario families, 14.2 Core metrics, 14.3 ContextSpy integration, 14. Eval framework

### Community 43 - "Community 43"
Cohesion: 0.50
Nodes (4): 9.1 `findTool`, 9.2 `describeTool`, 9.3 `callTool`, 9. Agent-facing contracts

### Community 44 - "Community 44"
Cohesion: 0.67
Nodes (3): 18.1 When to update `SPEC.md`, 18.2 Proposal checklist, 18. Change governance with OpenSpec

### Community 45 - "Community 45"
Cohesion: 0.67
Nodes (3): 5.1 Included in MVP, 5.2 Excluded from MVP, 5. Current MVP scope

### Community 46 - "Community 46"
Cohesion: 0.67
Nodes (3): 6.1 Main components, 6.2 Adapter paths, 6. System model

### Community 50 - "Community 50"
Cohesion: 0.08
Nodes (23): Requirement: Configuration initialization writes to user config home, Requirement: Opencode MCP section compatibility, Scenario: Default timeout is applied, Scenario: Enabled defaults to true, Scenario: Enabled false disables server, Scenario: Example fixture loads successfully, Scenario: Init honors explicit config override, Scenario: Init refuses to overwrite config (+15 more)

### Community 51 - "Community 51"
Cohesion: 0.28
Nodes (19): run(), runTestMCPServer(), TestCallStructuredFailureExitsNonZero(), TestCLIIndexesAndExposesToolsFromExplicitMCPConfig(), TestDoctorDoesNotLeakSecret(), TestDoctorReportsMissingEnv(), TestDoctorReportsServerHealthAndRedactsSecrets(), TestEvalReturnsNotImplemented() (+11 more)

### Community 52 - "Community 52"
Cohesion: 0.18
Nodes (10): Context, D1: Make config-home resolution explicit in `internal/config`, D2: `ozy init` writes only to the resolved config path, D3: Match opencode `mcp` section shape, not full opencode config, D4: Add deterministic CLI acceptance coverage plus opt-in real-server check, Decisions, Goals / Non-Goals, Migration Plan (+2 more)

### Community 53 - "Community 53"
Cohesion: 0.29
Nodes (6): Requirement: CLI exposes tools from explicit MCP configuration, Scenario: Describing an indexed tool from the CLI, Scenario: Indexing tools from the example config, Scenario: Listing indexed tools from the CLI, Scenario: Search uses the populated catalog, ADDED Requirements

### Community 54 - "Community 54"
Cohesion: 0.29
Nodes (6): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 55 - "Community 55"
Cohesion: 0.33
Nodes (5): 1. Config Home Resolution, 2. Init And Opencode MCP Compatibility, 3. Downstream Timeout Handling, 4. CLI Tool Resolution From Config, 5. Documentation And Verification

## Knowledge Gaps
- **324 isolated node(s):** `Broker`, `CatalogStats`, `FindResult`, `DescribeResult`, `CallResult` (+319 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `NewFile()` connect `Community 10` to `Community 0`, `Community 51`, `Community 14`?**
  _High betweenness centrality (0.109) - this node is a cross-community bridge._
- **Why does `NewErrorEnvelope()` connect `Community 5` to `Community 1`?**
  _High betweenness centrality (0.105) - this node is a cross-community bridge._
- **Why does `run()` connect `Community 51` to `Community 1`?**
  _High betweenness centrality (0.100) - this node is a cross-community bridge._
- **What connects `Broker`, `CatalogStats`, `FindResult` to the rest of the system?**
  _324 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.07575757575757576 - nodes in this community are weakly interconnected._
- **Should `Community 1` be split into smaller, more focused modules?**
  _Cohesion score 0.12987012987012986 - nodes in this community are weakly interconnected._
- **Should `Community 2` be split into smaller, more focused modules?**
  _Cohesion score 0.07200929152148665 - nodes in this community are weakly interconnected._