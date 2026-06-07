<!--
Sync Impact Report
==================
Version change: new → 1.0.0 (initial constitution)
Modified principles: N/A (initial creation)
Added sections:
  - Core Principles (5 principles)
  - Technical Architecture Constraints
  - Quality & Performance Gates
  - Governance
Removed sections: N/A
Templates requiring updates:
  - .specify/templates/plan-template.md: ✅ compatible (Constitution Check gate aligns)
  - .specify/templates/spec-template.md: ✅ compatible (no principle-specific references)
  - .specify/templates/tasks-template.md: ✅ compatible (no principle-specific references)
  - .specify/templates/checklist-template.md: ✅ compatible
Follow-up TODOs: None
-->

# Ozy Constitution

## Core Principles

### I. Agent-First Design

Every interface, API, capability, and tool surface MUST be designed for AI agent
consumption as the primary consumer. Human usability is secondary. The measure of
success is how naturally, efficiently, and correctly an agent discovers, selects, and
invokes capabilities without reading extraneous metadata.

- Tool descriptions MUST be concise and structured for machine parsing, not prose.
- Error messages MUST be actionable by an agent (structured, typed, with recovery hints).
- Output schemas MUST be explicit and parseable — never free-text where structure is
  expected.
- Agent ergonomics (context efficiency, discovery cost, invocation friction) are first-order
  design constraints, not afterthoughts.

**Rationale**: The project exists to serve AI agents. Human-friendly interfaces that waste
agent context or cause tool-selection errors directly undermine the project's north star.

### II. Token Economics as Core Metric

Context bloat from tool descriptions and skill frontmatter is the primary enemy. Every
design decision — what metadata to include, when to surface it, how to format it — MUST
quantify its token cost. Capability metadata MUST only enter an agent's context when the
agent explicitly needs it for the task at hand.

- Token budgets MUST be tracked per agent session. The broker MUST report savings
  against a baseline of bulk-loaded tool descriptions.
- Tool descriptions in the registry MUST be stored in compact, minified form; expanded
  representations are served only on demand.
- Redundant metadata (duplicate fields, verbose examples, unused parameters) MUST be
  stripped before surfacing to any agent.
- Any feature that adds more than 500 tokens of unavoidable context overhead MUST be
  justified with a cost/benefit analysis and approved.

**Rationale**: LLM context windows are finite and expensive. Every token spent on tool
metadata is a token not spent on reasoning. The broker's primary value proposition is
token savings — this principle makes that measurable and enforceable.

### III. Semantic Capability Discovery

Tools and capabilities MUST be discovered by what they DO, not by what they are NAMED.
The broker uses semantic vector search (turbovec) to match an agent's expressed task
intent against the actual capability surfaces available. Name-based enumeration is an
anti-pattern.

- Capability indexing MUST use dense vector embeddings generated from semantic
  descriptions of each tool's purpose, inputs, and outputs.
- Search queries from agents MUST be natural language descriptions of the task — the
  broker maps intent to capability, not keywords to names.
- Retrieval accuracy MUST be measured and tracked (recall@k, MRR, NDCG). Regressions
  in search quality are regressions in the product.
- The vector index MUST support fast, incremental updates as capabilities are
  added, modified, or removed from the registry.

**Rationale**: Name-based tool lookup forces agents to memorize tool names — brittle,
unscalable, and foreign to how agents reason. Semantic search lets agents express intent
naturally and get back exactly the relevant tools. This is the "native feeling" interface.

### IV. Platform & Framework Agnosticism

The daemon and MCP server MUST operate identically across all AI platforms (Claude,
GPT, Gemini, open-source models, etc.) and agent frameworks (LangChain, CrewAI,
AutoGen, custom orchestrators, etc.). No platform-specific assumptions, no vendor
lock-in, no special-casing for any single ecosystem.

- The MCP protocol is the sole contract between the broker and consuming agents. All
  capability brokering flows through standard MCP messages.
- Internal implementation MUST be isolated behind the MCP interface — platform-specific
  optimizations are forbidden unless they transparently degrade on other platforms.
- Testing MUST include multi-platform verification: at minimum two distinct LLM
  providers and two distinct agent frameworks per release gate.
- Configuration and deployment MUST be containerized and OS-agnostic (Linux, macOS).

**Rationale**: The value of a capability broker grows with the number of agents and
platforms it serves. Locking into one ecosystem caps that value and creates fragility
if the ecosystem shifts.

### V. Progressive Capability Disclosure

Capabilities are NEVER bulk-loaded into an agent's context. The broker presents tool
surfaces in stages: a minimal, high-signal initial surface based on the agent's declared
task, with progressive expansion as the agent requests more detail or as the task scope
evolves.

- Initial capability suggestions MUST be capped at the top K (default: 5) most relevant
  tools, with compact summaries only.
- Full tool schemas (parameters, return types, examples) are served only on explicit
  agent request for a specific tool.
- The broker MUST support follow-up refinement: if the initial K suggestions are
  insufficient, the agent can ask for more or refine its query.
- Capability metadata that is irrelevant to the current task domain MUST be excluded
  entirely — the broker is a filter, not a firehose.

**Rationale**: Agents operate under severe context constraints. Showing 100 tools when
only 3 are relevant wastes thousands of tokens and degrades tool-selection accuracy.
Progressive disclosure mirrors how humans use documentation — scan summaries first,
drill into details only when needed.

## Technical Architecture Constraints

- **Language**: Go (Golang). All core daemon, MCP server, and search infrastructure MUST
  be implemented in Go for performance, concurrency, and single-binary deployment.
- **Vector Engine**: turbovec. Semantic embeddings and similarity search MUST use
  turbovec as the primary vector store and retrieval engine.
- **Protocol**: MCP (Model Context Protocol). All agent-broker communication MUST use
  standard MCP messages (tools/list, tools/call, resources/read, etc.).
- **Deployment Modes**: The system MUST support both daemon mode (long-running
  background process serving multiple agent sessions) and embedded MCP server mode
  (per-session lifecycle).
- **Storage**: The capability registry MUST be persistent across restarts. Embeddings
  MUST survive daemon restarts without full re-indexing.
- **No External API Dependency**: Semantic search and capability brokering MUST
  function fully offline — no cloud API calls required for core broker operations.

## Quality & Performance Gates

- **Search Accuracy**: Recall@5 MUST be ≥ 0.90 on a curated benchmark of 100+
  task-to-tool mappings. MRR MUST be ≥ 0.85. Measured per release.
- **Token Savings**: The broker MUST demonstrate ≥ 60% token reduction versus
  bulk-loaded tool descriptions on standard agent task benchmarks (measured in total
  context tokens consumed for tool metadata across a session).
- **Latency**: Capability search (query → ranked results) MUST complete in ≤ 50ms p95
  for registries up to 10,000 tools. MCP message handling overhead MUST be ≤ 10ms p99.
- **Correctness**: Tool selection accuracy (does the agent pick the right tool for the
  task) MUST be measured against bulk-loaded baseline. The broker MUST never be worse
  than bulk loading — it is strictly a Pareto improvement.
- **Concurrency**: The daemon MUST handle ≥ 100 concurrent agent sessions without
  degradation in search latency or accuracy.

## Governance

This constitution is the supreme design and implementation authority for Ozy. Where
any other document, practice, or convention conflicts, this constitution prevails.

- **Amendment Process**: Changes require (1) a documented rationale, (2) impact analysis
  on existing code and benchmarks, and (3) approval via PR with at least one review.
  Principle removals or redefinitions constitute a MAJOR version bump.
- **Versioning**: MAJOR.MINOR.PATCH per Semantic Versioning. MAJOR: principle
  removal/redefinition or backward-incompatible governance change. MINOR: new principle
  or material expansion. PATCH: clarifications, wording, non-semantic fixes.
- **Compliance**: All PRs and code reviews MUST verify compliance with applicable
  principles. Violations without explicit justification in the PR description are
  grounds for rejection.
- **Complexity Justification**: Any architectural complexity beyond the simplest viable
  approach MUST be justified in writing — describing the simpler alternative and why it
  was insufficient.
- **Runtime Guidance**: See `README.md` and `AGENTS.md` for day-to-day development
  guidance that supplements but does not override this constitution.

**Version**: 1.0.0 | **Ratified**: 2026-06-07 | **Last Amended**: 2026-06-07
