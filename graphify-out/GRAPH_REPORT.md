# Graph Report - ozy  (2026-06-14)

## Corpus Check
- 92 files · ~60,892 words
- Verdict: corpus is large enough that graph structure adds value.

## Summary
- 1269 nodes · 1967 edges · 69 communities (66 shown, 3 thin omitted)
- Extraction: 92% EXTRACTED · 8% INFERRED · 0% AMBIGUOUS · INFERRED: 155 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Graph Freshness
- Built from commit: `cd0ba596`
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
- [[_COMMUNITY_Community 56|Community 56]]
- [[_COMMUNITY_Community 57|Community 57]]
- [[_COMMUNITY_Community 58|Community 58]]
- [[_COMMUNITY_Community 60|Community 60]]
- [[_COMMUNITY_Community 61|Community 61]]
- [[_COMMUNITY_Community 62|Community 62]]
- [[_COMMUNITY_Community 66|Community 66]]
- [[_COMMUNITY_Community 72|Community 72]]
- [[_COMMUNITY_Community 74|Community 74]]
- [[_COMMUNITY_Community 77|Community 77]]
- [[_COMMUNITY_Community 78|Community 78]]
- [[_COMMUNITY_Community 80|Community 80]]
- [[_COMMUNITY_Community 83|Community 83]]

## God Nodes (most connected - your core abstractions)
1. `contains()` - 41 edges
2. `NewMemory()` - 35 edges
3. `live` - 32 edges
4. `Config` - 31 edges
5. `SPEC.md — Ozy Living Specification` - 25 edges
6. `Requirement: Opencode MCP section compatibility` - 21 edges
7. `Connector` - 20 edges
8. `NewLive()` - 18 edges
9. `File` - 18 edges
10. `newCallBroker()` - 17 edges

## Surprising Connections (you probably didn't know these)
- `main()` --calls--> `Execute()`  [INFERRED]
  cmd/ozy/main.go → internal/cli/cli.go
- `newCallBroker()` --calls--> `NewLive()`  [INFERRED]
  internal/broker/call_test.go → internal/broker/live.go
- `newCallBroker()` --calls--> `NewMemory()`  [INFERRED]
  internal/broker/call_test.go → internal/catalog/memory.go
- `newBrokerWithTools()` --calls--> `NewLive()`  [INFERRED]
  internal/broker/live_test.go → internal/broker/live.go
- `newLiveBroker()` --calls--> `NewLive()`  [INFERRED]
  internal/broker/live_test.go → internal/broker/live.go

## Import Cycles
- None detected.

## Communities (69 total, 3 thin omitted)

### Community 0 - "Community 0"
Cohesion: 0.08
Nodes (50): NewLive(), ClientSession, Cmd, Connection, blockingTransport, hasEnv(), inMemoryFactory(), resultsByID() (+42 more)

### Community 1 - "Community 1"
Cohesion: 0.13
Nodes (14): Execute(), Daemon, app, Command, Error, Writer, Writer, T (+6 more)

### Community 2 - "Community 2"
Cohesion: 0.07
Nodes (51): BudgetsConfig, CallToolBudget, BudgetsConfig, CallToolBudget, Config, cloneConfig(), cloneServerConfig(), cloneStringMap() (+43 more)

### Community 3 - "Community 3"
Cohesion: 0.41
Nodes (11): skillPath, sourceType, computedHash, source, skills, chunking-strategy-design, drift-management-and-freshness, evaluation-operations (+3 more)

### Community 4 - "Community 4"
Cohesion: 0.11
Nodes (21): Alternative, CallNextAction, CallResult, Candidate, CatalogStats, DescribeResult, DoctorCheck, DoctorResult (+13 more)

### Community 5 - "Community 5"
Cohesion: 0.09
Nodes (22): CallToolRequest, asError(), WriteStarter(), Error, ErrorEnvelope, NewErrorEnvelope(), NotImplemented(), app (+14 more)

### Community 6 - "Community 6"
Cohesion: 0.12
Nodes (30): Bool, newCallBroker(), TestCallTool_DisabledServerReturnsConfigError(), TestCallTool_DownstreamToolErrorReturnsDownstreamCallFailed(), TestCallTool_MalformedToolRefReturnsToolNotFound(), TestCallTool_ResultExceedingBudgetIsTruncated(), TestCallTool_ResultWithinBudgetIsUnchanged(), TestCallTool_StructuredContentPreferredOverText() (+22 more)

### Community 7 - "Community 7"
Cohesion: 0.14
Nodes (20): skeleton, NewSkeleton(), newBroker(), TestCallTool_KnownToolReturnsNotImplemented(), TestCallTool_UnknownToolReturnsToolNotFound(), TestDescribeTool_UnknownReturnsToolNotFound(), TestFindTool_EmptyCatalogReturnsCatalogEmpty(), TestFindTool_NonEmptyCatalogReturnsNoGoodMatch() (+12 more)

### Community 8 - "Community 8"
Cohesion: 0.15
Nodes (21): Client, Connector, configError(), connectionError(), isOAuthAuthFailure(), Scrub(), secretValues(), WithMaxConcurrency() (+13 more)

### Community 9 - "Community 9"
Cohesion: 0.16
Nodes (18): Connector, New(), normalizeSchema(), normalizeTool(), scrub(), secretValues(), Indexer, Option (+10 more)

### Community 10 - "Community 10"
Cohesion: 0.10
Nodes (36): Context, Ranking, Engine, RankedEntry, Ranking, Context, RankedEntry, Store (+28 more)

### Community 11 - "Community 11"
Cohesion: 0.12
Nodes (16): 11. Configuration model, 12. Refresh and freshness behavior, 13. Token economy requirements, 15. CLI contract, 16. Security and privacy boundaries, 17. Observability and diagnostics, 1. Purpose of this document, 20. Accepted architectural baseline (+8 more)

### Community 12 - "Community 12"
Cohesion: 0.11
Nodes (17): Requirement: Ambiguous, no-match, and empty-catalog decisions are explicit, Requirement: Catalog-backed retrieval tolerates offline downstream servers, Requirement: findTool returns the top match and one runner-up, Requirement: Graceful degradation from semantic to lexical search, Requirement: Hybrid ranking over the persistent catalog, Scenario: A cataloged tool is still discoverable while its server is offline, Scenario: A confident query returns one best tool and one runner-up, Scenario: A query below the confidence floor yields no_good_match (+9 more)

### Community 13 - "Community 13"
Cohesion: 0.10
Nodes (19): Requirement: Ambiguous, no-match, and empty-catalog decisions are explicit, Requirement: Catalog-backed retrieval tolerates offline downstream servers, Requirement: findTool returns the top match and one runner-up, Requirement: Graceful degradation from semantic to lexical search, Requirement: Hybrid ranking over the persistent catalog, Scenario: A cataloged tool is still discoverable while its server is offline, Scenario: A confident query returns one best tool and one runner-up, Scenario: A query below the confidence floor yields no_good_match (+11 more)

### Community 14 - "Community 14"
Cohesion: 0.25
Nodes (8): File, fileDocument, Context, RWMutex, Server, Stats, Time, Tool

### Community 15 - "Community 15"
Cohesion: 0.33
Nodes (5): configuration, Purpose, Requirements, Requirement: Redaction in diagnostics, Scenario: Diagnostics show redacted configuration

### Community 16 - "Community 16"
Cohesion: 0.10
Nodes (20): daemon-runtime, Purpose, Requirements, Requirement: Catalog store interface placeholder, Requirement: Conditional indexing on startup, Requirement: Daemon lifecycle, Requirement: Graceful degradation of optional subsystems, Requirement: Shared in-process broker seam (+12 more)

### Community 17 - "Community 17"
Cohesion: 0.18
Nodes (14): NewFile(), TestFile_EmptyStoreQueriesAreClean(), TestFile_LastIndexedAtSurvivesRestart(), TestFile_OverwriteKeepsValidJSON(), TestFile_PersistedCatalogContainsNoConfigSecrets(), TestFile_WritesAndReloadsCatalog(), indexedToolCounts(), serverHealthChecks() (+6 more)

### Community 18 - "Community 18"
Cohesion: 0.08
Nodes (24): cli-interface, Purpose, Requirements, ADDED Requirements, Requirement: CLI exposes tools from explicit MCP configuration, Scenario: Describing an indexed tool from the CLI, Scenario: Indexing tools from the example config, Scenario: Listing indexed tools from the CLI (+16 more)

### Community 19 - "Community 19"
Cohesion: 0.13
Nodes (14): Confidence → decision mapping (the two-best contract), Context, Decisions, `findTool` becomes catalog-backed; the broker delegates to a search engine, Goals / Non-Goals, Hybrid fusion: normalized weighted sum, not RRF (so the confidence floor is meaningful), Lexical baseline: field-weighted BM25-style scoring over the indexed fields, Migration Plan (+6 more)

### Community 20 - "Community 20"
Cohesion: 0.07
Nodes (28): Requirement: Discover tools via tools/list, Requirement: Discovered tools carry freshness and runtime status, Requirement: List and describe reflect discovered tools, Requirement: `ozy index` populates the catalog, Requirement: Stable toolRef normalization, Scenario: A discovered tool gets a stable toolRef, Scenario: A freshly indexed tool is marked fresh, Scenario: describe returns a discovered tool's schema (+20 more)

### Community 21 - "Community 21"
Cohesion: 0.14
Nodes (16): ADDED Requirements, Requirements, Requirement: Connect to local (stdio) servers, Requirement: Connect to remote (HTTP) servers, Requirement: Non-leaky connection errors, Scenario: Connection failure error is redacted, Scenario: Local command environment is applied, Scenario: Local server connects and initializes (+8 more)

### Community 22 - "Community 22"
Cohesion: 0.15
Nodes (12): ADDED Requirements, Requirement: Catalog store interface placeholder, Requirement: Daemon lifecycle, Requirement: Graceful degradation of optional subsystems, Requirement: Shared in-process broker seam, Scenario: Catalog store seam is present, Scenario: Daemon refuses to start on invalid configuration, Scenario: Daemon shuts down cleanly (+4 more)

### Community 23 - "Community 23"
Cohesion: 0.28
Nodes (7): Memory, Context, RWMutex, Server, Stats, Time, Tool

### Community 24 - "Community 24"
Cohesion: 0.07
Nodes (29): downstream-connection, Purpose, Scenario: findTool returns a contract-shaped result, ADDED Requirements, Requirement: CLI command surface, Requirement: CLI mirrors broker operations, Requirement: Output formats, Requirement: Structured handling of unimplemented operations (+21 more)

### Community 25 - "Community 25"
Cohesion: 0.11
Nodes (25): failingSession, fakeConnector, fakeSession, newBrokerWithTools(), newLiveBroker(), searchSubstring(), TestFindTool_CatalogEmpty(), TestFindTool_NoGoodMatch() (+17 more)

### Community 26 - "Community 26"
Cohesion: 0.11
Nodes (17): Requirement: Invocation does not amplify retries, Requirement: Invoke the downstream tool via tools/call, Requirement: Normalize results and downstream errors, Requirement: Resolve toolRef to a downstream server and tool, Scenario: A malformed toolRef is rejected instructionally, Scenario: A reachable tool is invoked and returns a success result, Scenario: A valid toolRef resolves to its server and tool, Scenario: An unknown or disabled server is rejected instructionally (+9 more)

### Community 27 - "Community 27"
Cohesion: 0.07
Nodes (27): mcp-adapter, Purpose, Requirements, Requirement: MCP findTool performs live downstream discovery, Scenario: findTool keeps downstream tools as data, Scenario: findTool reports empty live discovery, Scenario: findTool reports partial downstream failures, Scenario: findTool reports total downstream failure (+19 more)

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
Cohesion: 0.08
Nodes (35): Alternative, Connector, live, applyCallBudget(), candidateRefs(), catalogToolToCandidate(), disabledServer(), extractErrorText() (+27 more)

### Community 32 - "Community 32"
Cohesion: 0.08
Nodes (24): catalog-persistence, Purpose, Requirements, ADDED Requirements, Requirement: Catalog storage holds no secrets, Requirement: Catalog survives restarts, Requirement: Durable catalog store, Requirement: Offline catalog reads (+16 more)

### Community 33 - "Community 33"
Cohesion: 0.22
Nodes (8): 1. Catalog last-index time, 2. Indexer records successful runs, 3. Search engine — lexical baseline, 4. Hybrid fusion and decision model, 5. Broker findTool over the catalog, 6. Daemon startup indexing, 7. Adapter and CLI parity, 8. Acceptance, eval, and documentation

### Community 34 - "Community 34"
Cohesion: 0.20
Nodes (9): Requirement: Single binary build, Requirement: Standard repository layout, Requirement: Test and lint tooling, Scenario: Binary exposes the command tree, Scenario: Building from a clean checkout, Scenario: Continuous integration runs build, test, and lint, Scenario: Entry point and internal packages present, Scenario: Tests run green (+1 more)

### Community 35 - "Community 35"
Cohesion: 0.36
Nodes (7): Freshness, Server, ServerStatus, Stats, Store, Tool, Time

### Community 36 - "Community 36"
Cohesion: 0.05
Nodes (44): ADDED Requirements, MODIFIED Requirements, Requirement: Configuration initialization writes to user config home, Requirement: Opencode MCP section compatibility, Scenario: Default timeout is applied, Scenario: Enabled defaults to true, Scenario: Enabled false disables server, Scenario: Example fixture loads successfully (+36 more)

### Community 37 - "Community 37"
Cohesion: 0.28
Nodes (14): rankedEntry, Tool, fieldTokens, indexedField, containsToken(), extractFields(), extractSchemaText(), gatherTokens() (+6 more)

### Community 38 - "Community 38"
Cohesion: 0.10
Nodes (19): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why, Acceptance Note, Capabilities (+11 more)

### Community 39 - "Community 39"
Cohesion: 0.22
Nodes (8): Agent interface, Build, Configuration, Minimal opencode configuration, ozy, Quick start, Usage, Using Ozy as an MCP server

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
Cohesion: 0.25
Nodes (7): ADDED Requirements, Requirement: Conditional indexing on startup, Requirement: Startup indexing degrades gracefully, Scenario: A fresh catalog skips startup indexing, Scenario: A stale catalog is reindexed on startup, Scenario: Partial startup indexing still serves reachable results, Scenario: Startup indexing failure does not block readiness

### Community 51 - "Community 51"
Cohesion: 0.25
Nodes (22): run(), runTestMCPServer(), TestCall_InvokesFixtureDownstreamViaCLIAndParityMatchesMCPPath(), TestCallStructuredFailureExitsNonZero(), TestCLIIndexesAndExposesToolsFromExplicitMCPConfig(), TestDiscoveryEval_GoldIntentMatchesExpectedToolRef(), TestDoctorDoesNotLeakSecret(), TestDoctorReportsMissingEnv() (+14 more)

### Community 52 - "Community 52"
Cohesion: 0.12
Nodes (15): Requirement: Invocation does not amplify retries, Requirement: Invoke the downstream tool via tools/call, Requirement: Normalize results and downstream errors, Requirement: Resolve toolRef to a downstream server and tool, Scenario: A malformed toolRef is rejected instructionally, Scenario: A reachable tool is invoked and returns a success result, Scenario: A valid toolRef resolves to its server and tool, Scenario: An unknown or disabled server is rejected instructionally (+7 more)

### Community 53 - "Community 53"
Cohesion: 0.16
Nodes (15): Scenario: JSONC comments and trailing commas are accepted, Scenario: Loading a valid JSONC configuration, Requirement: Configuration discovery and loading, Scenario: Explicit config path override, Scenario: Missing configuration file, Requirement: Configuration discovery and loading, Scenario: Explicit config path override, Scenario: Loading a valid default user configuration file (+7 more)

### Community 54 - "Community 54"
Cohesion: 0.05
Nodes (45): Context, D1: CLI framework — `spf13/cobra`, D2: MCP server library — official `modelcontextprotocol/go-sdk`, hidden behind an internal adapter, D3: One broker interface shared by both adapters, D4: In-process broker now, client/server split deferred, D5: Typed results that marshal to §9 contracts, D6: Catalog store as an interface with an in-memory placeholder, D7: Config via `gopkg.in/yaml.v3` with a typed model + redaction (+37 more)

### Community 55 - "Community 55"
Cohesion: 0.29
Nodes (6): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 56 - "Community 56"
Cohesion: 0.07
Nodes (42): NewMemory(), TestMemory_EmptyStoreQueriesAreClean(), TestMemory_LastIndexedAt(), TestMemory_StatsCountFreshness(), Daemon, New(), NewWithStore(), TestNew_UsesPersistentCatalogStore() (+34 more)

### Community 57 - "Community 57"
Cohesion: 0.33
Nodes (5): ADDED Requirements, Requirement: Catalog records the last successful index time, Scenario: A successful index run records its timestamp, Scenario: Absence of a prior index is distinguishable, Scenario: Last index time is readable without re-discovery

### Community 58 - "Community 58"
Cohesion: 0.14
Nodes (13): Requirement: Redaction in diagnostics, Scenario: Diagnostics show redacted configuration, MODIFIED Requirements, Requirement: Environment reference resolution, Requirement: Redaction in diagnostics, Scenario: Diagnostics show redacted configuration, Scenario: Missing environment reference is diagnosable, Scenario: Resolving a present environment reference (+5 more)

### Community 60 - "Community 60"
Cohesion: 0.40
Nodes (5): state, sessionID, sources, background-task, updatedAt

### Community 61 - "Community 61"
Cohesion: 0.14
Nodes (16): 1. Configuration (JSONC + opencode shape), 2. Persistent catalog store, 3. Downstream connection layer, 4. Tool discovery / indexing, 5. CLI, broker, and doctor wiring, 6. Docs and spec note, 7. Verification, 1. Contract and Test Coverage (+8 more)

### Community 62 - "Community 62"
Cohesion: 0.24
Nodes (10): Scenario: Invalid configuration is rejected with a structured error, Requirement: Configuration validation, Requirement: Configuration validation, Scenario: Local server without a command is rejected, Scenario: Remote server without a url is rejected, Scenario: Unknown server type is rejected, Requirement: Configuration validation, Scenario: Local server without a command is rejected (+2 more)

### Community 66 - "Community 66"
Cohesion: 0.33
Nodes (8): RankedEntry, Engine, RankedEntry, Store, Tool, Ranking, New(), Semantic

### Community 72 - "Community 72"
Cohesion: 0.33
Nodes (6): Requirement: Per-server isolation, Scenario: Disabled servers are skipped, Scenario: One unreachable server does not abort the others, Requirement: Per-server isolation, Scenario: Disabled servers are skipped, Scenario: One unreachable server does not abort the others

### Community 74 - "Community 74"
Cohesion: 0.15
Nodes (12): Context, Decisions, Extend two seams: `Session.CallTool` and the broker `Connector`, Goals / Non-Goals, Implement invocation in the live broker, not the skeleton, Map the downstream result onto the §9.3 envelope, Migration Plan, No adapter or CLI surface change (+4 more)

### Community 77 - "Community 77"
Cohesion: 0.29
Nodes (6): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 78 - "Community 78"
Cohesion: 0.29
Nodes (6): Capabilities, Impact, Modified Capabilities, New Capabilities, What Changes, Why

### Community 80 - "Community 80"
Cohesion: 0.22
Nodes (8): Scenario: Loading a valid configuration file, ADDED Requirements, Requirement: Configuration discovery and loading, Requirement: Environment reference resolution, Scenario: Explicit config path override, Scenario: Missing configuration file, Scenario: Missing environment reference is diagnosable, Scenario: Resolving a present environment reference

### Community 83 - "Community 83"
Cohesion: 0.40
Nodes (4): 1. Seams and Test Coverage, 2. Live Invocation Implementation, 3. Adapter and CLI Parity, 4. Acceptance and Documentation

## Knowledge Gaps
- **466 isolated node(s):** `Bool`, `ListToolsParams`, `ListToolsResult`, `CallToolParams`, `ServerConfig` (+461 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **3 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `contains()` connect `Community 6` to `Community 0`, `Community 1`, `Community 2`, `Community 8`, `Community 9`, `Community 10`, `Community 17`, `Community 51`, `Community 56`, `Community 25`?**
  _High betweenness centrality (0.112) - this node is a cross-community bridge._
- **Why does `NewMemory()` connect `Community 56` to `Community 0`, `Community 6`, `Community 7`, `Community 10`, `Community 51`, `Community 23`, `Community 25`?**
  _High betweenness centrality (0.059) - this node is a cross-community bridge._
- **Why does `NewLive()` connect `Community 0` to `Community 6`, `Community 51`, `Community 56`, `Community 25`, `Community 31`?**
  _High betweenness centrality (0.030) - this node is a cross-community bridge._
- **Are the 39 inferred relationships involving `contains()` (e.g. with `TestCallTool_DownstreamToolErrorReturnsDownstreamCallFailed()` and `TestCallTool_MalformedToolRefReturnsToolNotFound()`) actually correct?**
  _`contains()` has 39 INFERRED edges - model-reasoned connections that need verification._
- **Are the 34 inferred relationships involving `NewMemory()` (e.g. with `newCallBroker()` and `newBrokerWithTools()`) actually correct?**
  _`NewMemory()` has 34 INFERRED edges - model-reasoned connections that need verification._
- **What connects `Bool`, `ListToolsParams`, `ListToolsResult` to the rest of the system?**
  _466 weakly-connected nodes found - possible documentation gaps or missing edges._
- **Should `Community 0` be split into smaller, more focused modules?**
  _Cohesion score 0.08182349503214495 - nodes in this community are weakly interconnected._