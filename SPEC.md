# SPEC.md — Ozy Living Specification

**Project:** Ozy  
**Document status:** Living project specification  
**Audience:** Maintainer, contributors, coding agents, OpenSpec proposal authors, reviewers  
**Owner:** Project maintainer  
**Last reviewed:** 2026-06-12
**Review-board clarifications:** Applied 2026-06-12 — freshness value set, `catalog_empty`, `catalogStats`, `agentInstruction` quality criteria, retry-amplification guard, Ozy tool contract versioning, grader approach, and post-baseline token-economy target rule.

---

## 1. Purpose of this document

`SPEC.md` is the project-level source of truth for Ozy's product shape, architectural boundaries, behavioral contracts, and change discipline.

It is not a task list, sprint plan, or implementation diary. It defines the stable frame that OpenSpec-driven changes must preserve or deliberately amend.

Every significant change to Ozy should answer:

1. Which part of `SPEC.md` does this change implement, refine, or challenge?
2. Which Ozy contract does this change affect?
3. Which eval proves the change improved the system or preserved existing behavior?
4. What context, token, safety, or operability cost does the change introduce?

`SPEC.md` should remain concise enough to be usable by agents, but complete enough to prevent the project from drifting into an ordinary MCP proxy.

---

## 2. Project mission

Ozy is a local agent tool broker.

Users configure many downstream MCP servers once. Ozy discovers and indexes their tools into a persistent searchable capability catalog. Agents connect only to Ozy and use a small stable interface to discover, understand, and invoke the right downstream tool without loading the entire downstream tool universe into context.

Ozy exists to provide:

- one place to maintain MCP configuration;
- a durable searchable registry of tool capabilities;
- an agent-facing interface that is small, stable, and instructional;
- brokered invocation of downstream tools through Ozy;
- measurable reduction in tool/context overhead without sacrificing task success.

Ozy is not a dumb MCP proxy. It is a capability registry and instructional tool broker.

---

## 3. Product thesis

Modern agent environments increasingly support tools, MCP servers, and local/remote integrations. But users who work across more than one agent client quickly run into repeated configuration, excessive tool schemas, fragile auth/env duplication, and poor discovery ergonomics.

Ozy's thesis:

> Tool access should be configured once, indexed persistently, searched by capability, and exposed to agents through a minimal instructional interface.

The agent should not need to see every available downstream tool at startup. The agent should be able to ask for the capability it needs, inspect the selected tool, and call it through Ozy.

---

## 4. Core principles

### 4.1 Capability brokerage over naive proxying

Ozy must preserve the distinction between proxying and brokering.

A proxy forwards calls. A broker understands configured capability surfaces, maintains catalog state, guides tool selection, validates invocation, and reports operational truth.

Any change that makes Ozy merely expose all downstream MCP tools directly by default violates this principle unless it is explicitly isolated as an optional compatibility mode.

### 4.2 One configuration source of truth

Users should configure downstream MCP servers, commands, URLs, environment variables, auth references, enablement flags, labels, trust metadata, and refresh behavior in one Ozy-managed configuration source.

Ozy should reduce duplicated MCP configuration across agent clients.

### 4.3 Small stable agent surface

Agents should initially see a minimal stable Ozy-facing interface.

MVP agent-facing MCP tools:

- `findTool`
- `describeTool`
- `callTool`

Future additions must justify their context cost and prove they do not undermine the small-surface model.

### 4.4 Persistent searchable capability catalog

Ozy must not depend only on downstream servers being online at the exact moment an agent asks for a tool.

Ozy should preserve previously discovered tool metadata, schemas, capability text, freshness, runtime state, and schema hashes. Search and description may use cached catalog data when clearly marked. Invocation must still be live-gated.

### 4.5 Instructional responses for agent certainty

Ozy responses are for agents first and humans second.

Every response should reduce ambiguity. In general, Ozy should tell the agent:

- what was found;
- why it matched;
- whether it is callable now;
- what the next correct action is;
- what should be avoided;
- how to repair common errors.

The agent should not have to guess the intended workflow after calling Ozy.

`agentInstruction` and related instructional fields must satisfy these quality criteria:

- **Actionable:** tell the agent the next concrete action, including which Ozy tool or user-facing response to use when appropriate.
- **Conditional:** distinguish between retry, do-not-retry, ask-user, choose-alternative, refresh, and report-failure cases.
- **Grounded:** base instructions only on known catalog/runtime/error state; do not imply successful execution, hidden retries, or unavailable capabilities.

Vague restatements of the error are not acceptable instructional responses.

### 4.6 Live-gated invocation

Searching and describing cached tools may be stale-tolerant. Calling tools is not.

Ozy must not fabricate execution, silently call stale schemas, or hide downstream availability problems. If a downstream server is offline, schema drift is detected, or argument validation fails, Ozy must return structured, repair-oriented errors.

### 4.7 Token economy is a first-class quality attribute

Ozy's value depends on reducing unnecessary context and tool-schema load while maintaining or improving task success.

Changes must consider:

- startup tool schema tokens;
- per-turn input tokens;
- response payload size;
- largest response size;
- tool result verbosity;
- number of broker calls;
- total tokens-to-success;
- success/failure rate.

A feature that works but bloats context without justification is incomplete.

### 4.8 Evaluation before confidence

Ozy should be developed with scenario-based evals from the beginning.

Manual success is not enough. Features that affect discovery, invocation, error handling, response shape, or token economy require eval coverage.

### 4.9 Adapter-neutral core

The core broker behavior must not be locked to one interface.

MCP is required for broad agent compatibility. CLI is required for debugging, shell-capable agents, evals, and user inspection. Both must reflect the same underlying contracts.

### 4.10 Local-first and privacy-respecting defaults

Ozy should work locally by default. External services may be supported, but users must be able to understand and control what data leaves the machine.

Semantic search must not require hosted services. If local semantic search is unavailable, Ozy should gracefully fall back to lexical search.

### 4.11 Observable and diagnosable behavior

Users and agents should be able to inspect:

- configured servers;
- server health;
- indexed tools;
- last indexed time;
- schema freshness;
- why a tool matched a query;
- why a call failed;
- what changed after refresh;
- token and eval outcomes where available.

Silent fallback is disfavored. Explicit status is preferred.

---

## 5. Current MVP scope

### 5.1 Included in MVP

MVP includes:

- central Ozy configuration for downstream MCP servers;
- local Ozy daemon;
- MCP adapter exposing Ozy to agents;
- CLI mirror of the same broker operations;
- persistent tool catalog;
- lexical search baseline;
- optional semantic search;
- optional Python embedding/indexing worker;
- `findTool`, `describeTool`, and `callTool` contracts;
- brokered invocation of downstream MCP tools;
- offline searchable catalog behavior;
- schema freshness and drift detection;
- structured errors with agent instructions;
- `doctor` diagnostics;
- eval harness focused on discovery, invocation, repair, and token economy;
- optional ContextSpy integration for measurement/profiling.

### 5.2 Excluded from MVP

MVP excludes:

- usage-aware handoff between agents;
- dynamic native exposure of downstream tools as first-class agent tools;
- hosted cloud service;
- team/multi-user administration;
- approval UI;
- complex policy engine;
- full OAuth/device-flow productization;
- automatic learning from usage;
- tool result caching as a major subsystem;
- automatic task delegation to other agents;
- remote synchronization of catalogs.

These are not rejected forever. They require future OpenSpec proposals and must pass the principles in this document.

---

## 6. System model

### 6.1 Main components

Ozy consists of:

1. **Ozy daemon**  
   Owns configuration, catalog state, downstream server lifecycle, health, indexing coordination, and brokered invocation.

2. **Ozy MCP adapter**  
   Exposes the stable Ozy MCP tools to agent clients.

3. **Ozy CLI**  
   Provides user and shell-agent access to the same operations as the MCP adapter.

4. **Downstream MCP connector layer**  
   Starts/connects to configured MCP servers, calls `tools/list`, listens for tool list changes when supported, and invokes downstream `tools/call`.

5. **Catalog store**  
   Persists servers, tools, schemas, hashes, freshness, status, capability text, and search metadata.

6. **Search/index layer**  
   Provides lexical search as baseline and semantic search as optional enhancement.

7. **Embedding/indexing worker**  
   Optional component, likely Python-backed, responsible for local embeddings and future reranking/index enrichment.

8. **Eval/measurement layer**  
   Runs scenarios, compares direct-MCP vs Ozy flows, and reports correctness and token economy.

### 6.2 Adapter paths

MCP path:

```text
Agent / MCP client
  -> ozy mcp
  -> ozy daemon
  -> downstream MCP server
```

CLI path:

```text
User or shell-capable agent
  -> ozy search / describe / call
  -> ozy daemon
  -> downstream MCP server
```

Both paths must use the same broker contracts.

---

## 7. Tool lifecycle

A downstream tool moves through this lifecycle:

1. **Configured**  
   The downstream MCP server is present in Ozy configuration.

2. **Discovered**  
   Ozy successfully connects to the server and retrieves its tool list.

3. **Cataloged**  
   Ozy normalizes discovered tool metadata into a stable `toolRef`.

4. **Indexed**  
   Ozy indexes tool name, description, schema fields, annotations, examples, and capability text.

5. **Searchable**  
   Agents can discover the tool using `findTool` even if the downstream server is later offline, provided cached catalog data exists.

6. **Describable**  
   Agents can inspect schema and usage guidance using `describeTool`.

7. **Callable**  
   Agents can invoke the downstream tool through `callTool` only when the target server is reachable and schema/argument checks pass.

8. **Refreshed**  
   Ozy updates metadata when the server reconnects, config changes, refresh is requested, or downstream tool-list change notifications occur.

---

## 8. Tool reference model

Every downstream tool receives a stable Ozy tool reference:

```text
<serverId>.<downstreamToolName>
```

Examples:

```text
atlassian.confluence_search
atlassian.confluence_get_page
opengrok.search_code
github.search_issues
filesystem.read_file
```

A cataloged tool should contain at minimum:

```json
{
  "toolRef": "atlassian.confluence_search",
  "serverId": "atlassian",
  "downstreamToolName": "confluence_search",
  "title": "Search Confluence",
  "description": "Searches Confluence pages.",
  "inputSchema": {},
  "capabilityText": [],
  "runtime": {
    "serverStatus": "online",
    "callableNow": true
  },
  "catalog": {
    "lastIndexedAt": "2026-06-12T10:00:00+03:00",
    "schemaHash": "...",
    "freshness": "fresh"
  }
}
```

### 8.1 Catalog freshness values

Initial catalog freshness values are:

- `fresh`: the catalog entry reflects the latest successful discovery for that downstream server and is within the configured freshness policy.
- `stale`: the catalog entry is known only from cached or outdated discovery data, or the downstream server could not currently confirm it.

Search and description may use `stale` entries when clearly marked. Invocation must verify live downstream availability and schema safety before calling.

`toolRef` stability matters. Changes that rename toolRefs must define migration and compatibility behavior.

---

## 9. Agent-facing contracts

Ozy's own agent-facing tools are contracts, not incidental helper methods. Each exposed Ozy tool schema and response shape must carry or be associated with a contract version. Changes to required inputs, response fields, decision values, error types, or instructional semantics must be treated as contract changes. Description changes that materially alter how an agent should use a tool are also contract-affecting and require review, eval coverage, and compatibility/migration notes.

### 9.1 `findTool`

Purpose:

> Find the best known tool or tools for a capability query.

Input:

```json
{
  "query": "tool to search confluence wiki pages for internal documentation"
}
```

Response requirements:

- return a decision, not only a list;
- include selected best tool when confidence allows;
- include confidence and reason (the reason names the highest-signal matched terms, ranked by corpus IDF — never stopword noise);
- include `callableNow` and server status as reconciled state from the most recent index run;
- include the schema preview for large schemas — but inline the full `inputSchema` plus a `recommendedCall` when the schema is small (the fast path), so the agent can invoke directly and `describeTool` becomes the exception;
- include next action (`callTool` on the fast path, `describeTool` otherwise);
- include likely follow-up tools when obvious;
- include up to `budgets.findTool.maxResults − 1` ranked alternatives, each with a one-line reason;
- include avoid/repair guidance where useful;
- include lightweight catalog health (`catalogStats`), including `catalogAgeSeconds` — seconds since the last successful index run (omitted when never indexed) — so the agent can weigh how current the reported state is;
- explicitly handle empty-catalog and no-indexed-tool states;
- stay within response-size budgets.

Example shape:

```json
{
  "query": "tool to search confluence wiki pages for internal documentation",
  "decision": "use",
  "selectedToolRef": "atlassian.confluence_search",
  "confidence": "high",
  "reason": "Best indexed tool for searching Confluence/wiki/internal documentation.",
  "selected": {
    "toolRef": "atlassian.confluence_search",
    "title": "Search Confluence",
    "callableNow": true,
    "serverStatus": "online",
    "schemaPreview": {
      "required": ["query"],
      "properties": ["query", "limit"]
    }
  },
  "catalogStats": {
    "configuredServers": 3,
    "indexedTools": 42,
    "freshTools": 38,
    "staleTools": 4,
    "catalogAgeSeconds": 5400
  },
  "nextAction": {
    "tool": "describeTool",
    "arguments": {
      "toolRef": "atlassian.confluence_search"
    },
    "reason": "Inspect exact schema before invoking through callTool."
  },
  "likelyFollowupTools": [
    {
      "toolRef": "atlassian.confluence_get_page",
      "when": "Use after search returns page IDs."
    }
  ],
  "alternatives": [],
  "avoid": [
    "Do not invent page IDs; search first."
  ]
}
```

Decision values are explicit, and every listed value is emittable by the live broker — advertising unreachable states is the lying-interface failure this contract exists to prevent:

- `use`
- `no_good_match`
- `ambiguous`
- `catalog_empty`

`catalog_empty` means Ozy has no indexed tools to search. It must instruct the agent not to infer absence of capability and should direct the user or agent toward `ozy doctor`, indexing, or configuration repair.

Even ambiguous responses must be instructional — and self-consistent: an `ambiguous` response inlines the close candidates' schemas, so its instruction directs the agent to compare the inlined candidates and call the chosen one, never to re-fetch those same schemas with `describeTool`.

### 9.2 `describeTool`

Purpose:

> Return exact tool schema, usage guidance, examples, status, and recommended call shape for one known `toolRef`.

Input:

```json
{
  "toolRef": "atlassian.confluence_search"
}
```

Response requirements:

- include exact input schema;
- include usage hints;
- include examples;
- include related tools;
- include freshness/runtime status;
- include a recommended `callTool` shape;
- mark stale data clearly;
- avoid dumping unrelated tools.

Example shape:

```json
{
  "toolRef": "atlassian.confluence_search",
  "title": "Search Confluence",
  "description": "Searches Confluence pages.",
  "inputSchema": {
    "type": "object",
    "required": ["query"],
    "properties": {
      "query": {
        "type": "string",
        "description": "Plain text search query."
      },
      "limit": {
        "type": "integer",
        "default": 10
      }
    }
  },
  "usageHints": [
    "Use plain natural-language search unless the user explicitly asks for CQL.",
    "Use limit 5-10 for exploratory searches."
  ],
  "examples": [
    {
      "request": "Find onboarding docs about billing migration",
      "arguments": {
        "query": "billing migration onboarding",
        "limit": 5
      }
    }
  ],
  "recommendedCall": {
    "tool": "callTool",
    "arguments": {
      "toolRef": "atlassian.confluence_search",
      "arguments": {
        "query": "<search query>",
        "limit": 5
      }
    }
  },
  "relatedTools": [
    {
      "toolRef": "atlassian.confluence_get_page",
      "relationship": "Use after search to read a selected page."
    }
  ],
  "status": {
    "callableNow": true,
    "serverStatus": "online",
    "catalogFreshness": "fresh"
  }
}
```

### 9.3 `callTool`

Purpose:

> Invoke a selected downstream MCP tool through Ozy.

Input:

```json
{
  "toolRef": "atlassian.confluence_search",
  "arguments": {
    "query": "billing migration onboarding",
    "limit": 5
  }
}
```

Runtime requirements:

- resolve `toolRef` to downstream server and tool name;
- verify downstream server is reachable;
- validate arguments against current or acceptable schema state;
- reject stale unsafe invocation;
- call downstream MCP `tools/call`;
- normalize result;
- include next-action guidance when obvious;
- preserve downstream error detail where useful without leaking secrets.

Success response shape:

```json
{
  "ok": true,
  "toolRef": "atlassian.confluence_search",
  "result": {},
  "resultSummary": "Found relevant Confluence pages.",
  "notices": [
    "result truncated: showing 12 of 40 items (budgets.callTool.maxResultBytes=65536); narrow the call for the rest"
  ],
  "cachedAgeSeconds": 42,
  "nextActions": [
    {
      "recommended": true,
      "toolRef": "atlassian.confluence_get_page",
      "reason": "Search returned page IDs. Use this tool to read a selected page.",
      "exampleCall": {
        "tool": "callTool",
        "arguments": {
          "toolRef": "atlassian.confluence_get_page",
          "arguments": {
            "pageId": "<page id>"
          }
        }
      }
    }
  ]
}
```

Failure response shape:

```json
{
  "ok": false,
  "error": {
    "type": "DOWNSTREAM_SERVER_OFFLINE",
    "toolRef": "atlassian.confluence_search",
    "serverId": "atlassian",
    "retryable": true,
    "message": "The tool is known from the cached catalog, but the downstream MCP is not reachable.",
    "agentInstruction": "Do not retry immediately. Inform the user that the Confluence MCP is offline or choose another available tool if appropriate."
  }
}
```

`agentInstruction` on failures must explicitly state whether the agent should retry, avoid retrying, refresh/describe again, choose another tool, ask the user, or report the failure. If Ozy performs internal retries, the response must not cause retry amplification; it must clearly state whether additional agent-side retry is recommended. Failures caused by budgets Ozy itself imposed — the per-server `callTimeout`, the byte budget — are never marked `retryable: true`, because repeating the identical call cannot succeed.

`notices` carries actionable in-band messages (truncation recovery, staleness); `cachedAgeSeconds` is present only when the result was served from the result cache. Truncation is a labeled partial success delivered in-band, not an error type: arrays lose whole trailing elements (what remains is valid JSON, with an "N of M items" notice), text is cut at a line or word boundary. Adapters must deliver notices inside the response content the agent reads — never only in out-of-band metadata such as MCP `_meta`, which major clients do not surface to the model.

Common error types:

- `TOOL_NOT_FOUND`
- `DOWNSTREAM_SERVER_OFFLINE`
- `ARGUMENT_VALIDATION_FAILED`
- `TOOL_SCHEMA_CHANGED` — reserved: names the planned schema-drift failure (cataloged schema no longer matching the live tool). The eval corpus exercises its shape; the live broker does not emit it yet.
- `DOWNSTREAM_CALL_FAILED`
- `AUTH_UNAVAILABLE`
- `SEMANTIC_SEARCH_UNAVAILABLE`
- `CONFIG_ERROR`

---

## 10. Search behavior

### 10.1 Baseline requirement

Lexical search must work without embeddings.

Semantic search is valuable but must not be required for basic operation.

### 10.2 Indexed fields

Ozy should index:

- server id;
- server labels/tags;
- downstream tool name;
- title;
- description;
- input schema field names;
- input schema descriptions;
- annotations;
- examples;
- manually configured capability aliases;
- observed/curated usage hints where accepted into spec.

### 10.3 Hybrid search

When semantic search is enabled, Ozy combines the lexical and semantic
rankings with **Reciprocal Rank Fusion (RRF)**:

```text
score(tool) = Σ_lists  1 / (k + rank_in_list)      with k = 60
```

RRF is rank-based, so the BM25 lexical scores and the cosine similarities
from the sidecar do not need to be calibrated to a common scale. With no
semantic signal, RRF over the single lexical list reduces to the lexical
order, so the degraded path is identical to today's baseline.

The `use` vs. `ambiguous` decision is made on the RRF gap between the top two
entries; `no_good_match` is gated on an absolute component floor
(normalized lexical relevance OR cosine similarity) because RRF alone carries
no absolute relevance. The floors and the RRF `k` are named tunable constants
in `internal/search` and can be calibrated against the discovery evals
(§14.1).

`findTool` responses should be able to say why a tool matched, for example:

```text
Matched lexical term "confluence" and semantic intent "internal wiki search".
```

### 10.4 Embedding/indexing architecture

Ozy ships with an **integrated Python embedding sidecar** that the Go daemon
launches and supervises. The sidecar speaks newline-delimited JSON over its
standard input/output, with the daemon owning the lifecycle (provisioning,
spawn, health-check, shutdown) and stderr drained for logs.

Boundary:

- **Go** owns runtime, daemon, MCP, CLI, catalog authority, brokered calls,
  online search, and the lexical BM25 ranker.
- **Python sidecar** owns the embedding model, the embedding-metadata store
  (SQLite, the `toolRef ↔ vector_id` map, content hashes, model/version
  metadata, and raw float32 vectors), and the ANN index. It does NOT own
  Ozy's authoritative catalog of tools, schemas, or runtime status.
- **Vector index** is a derived artifact, rebuildable from SQLite. The default
  backend is **turbovec** (`IdMapIndex` with 4-bit quantization and kernel-level
  allowlist filtering); **FAISS** (`IndexIDMap` over `IndexFlatIP` with
  `IDSelectorBatch` allowlist) is an opt-in alternative selected before the
  first index is built. Switching backends after the first index requires a
  rebuild.

**Vector backend selection is fixed before the first index and immutable
after** — a mismatch between configured and recorded backend forces a
rebuild from SQLite rather than serving mixed artifacts.

**Facet-scoped search** resolves a facet (e.g. `server_id = "atlassian"`) in
SQLite to an allowlist of `vector_id`s, then passes that allowlist to the
index. Filtering happens inside the search kernel; results are not
over-fetched and post-filtered.

**Provisioning and supervision** are owned by the daemon. When semantic search
is enabled, the daemon resolves a Python interpreter, creates a pinned
isolated environment under XDG state via `uv` (with a `python -m venv` +
`pip` fallback), installs `fastembed` and `turbovec` (`faiss-cpu` only when
the FAISS backend is selected), caches the env, and starts the sidecar.
Provisioning never blocks daemon readiness: any failure (no `uv`, no
`python3`, pip install error) leaves the daemon ready in lexical-only mode
with a surfaced notice. The planned standalone bootstrap/installer change
will replace this interim on-demand provisioner.

The Go client in `internal/sidecar` speaks the JSONL protocol
(`health`/`upsert`/`delete`/`query`/`stats`), drains stderr to a logger,
enforces per-request timeouts, and returns "unavailable" (not an error) on
any failure so the engine degrades.

The full protocol shape and request/response payloads are documented in
`sidecar/README.md`.

---

## 11. Configuration model

Ozy configuration should be explicit, inspectable, and portable.

Illustrative shape:

```yaml
version: 1

servers:
  atlassian:
    enabled: true
    transport: http
    url: https://mcp.example.com/v1/mcp
    auth:
      type: env
      header: Authorization
      value: "Bearer ${ATLASSIAN_MCP_TOKEN}"

  opengrok:
    enabled: true
    transport: stdio
    command: opengrok-go-mcp
    args: []
    env:
      OPENGROK_URL: "https://opengrok.home"
    timeout: 5000        # discovery/connect budget (ms), used when indexing
    callTimeout: 60000   # per-callTool budget (ms): connect + execute; a call
                         # killed by this deadline reports retryable: false

embedding:
  provider: python-local
  required: false

search:
  lexical:
    enabled: true
  semantic:
    enabled: true
    required: false

budgets:
  findTool:
    maxResults: 5              # bounds selected + alternatives in a findTool response
    includeFullSchemas: false  # force schema inlining regardless of the fast-path size threshold
  describeTool:
    includeExamples: true
  callTool:
    maxResultBytes: 65536

cache:
  enabled: true       # on by default; false for a pure pass-through
  ttlSeconds: 300     # entry lifetime
  maxEntries: 1024    # bound on stored entries
```

The `cache` section toggles the broker result cache. It memoizes
`findTool`/`describeTool` results and read-only `callTool` results within the
TTL, keyed by a content hash of the request folded with the catalog generation
(findTool) or the target tool's schema hash (describe/call). `callTool` is cached
only for tools whose downstream `readOnlyHint` is true — write tools and tools of
unknown intent always invoke live, and failures are never cached.

Configuration requirements:

- support enabling/disabling servers;
- avoid secret literals where env references are possible;
- allow diagnostics for missing env vars;
- preserve cross-platform behavior;
- support future migration/versioning;
- avoid requiring users to duplicate config in each agent.

---

## 12. Refresh and freshness behavior

Ozy should refresh catalog data when:

- daemon starts;
- user runs explicit index/refresh command;
- downstream server reconnects;
- downstream tool-list change notification is received;
- Ozy config changes;
- scheduled refresh interval elapses, if configured.

Freshness must be visible in tool descriptions and search results.

Search may return stale known tools when marked. Invocation must verify live state and schema safety.

---

## 13. Token economy requirements

Ozy should maintain explicit response budgets.

Default expectations:

- `findTool` returns previews, not full schemas;
- `describeTool` returns one full schema at a time;
- `callTool` avoids unbounded result dumps;
- large downstream results are either paginated, summarized, or clearly marked as truncated;
- errors are structured and concise;
- alternatives are limited unless ambiguity requires more.

Required token-economy metrics for evals:

- startup tool-schema tokens;
- total input tokens to task success;
- tool definition tokens;
- tool result tokens;
- largest response bytes/tokens;
- number of broker calls;
- direct-MCP baseline comparison;
- Ozy broker comparison;
- success/failure outcome.

After the first representative eval cycle establishes direct-MCP and Ozy baselines, this section must be updated with concrete token-economy targets. Until measured baselines exist, Ozy may state expected directionality but must not claim quantified savings as a contract.

---

## 14. Eval framework

Ozy features should be evaluated through scenarios.

Grading approach:

- Discovery metrics should use curated gold sets mapping representative user intents to acceptable target tools and servers.
- Token and payload metrics should be measured deterministically from captured requests, responses, tool schemas, and profiler data where available.
- Judgment-heavy metrics, such as instruction quality and repair usefulness, should use human or review-board assessment against explicit rubrics until reliable automated graders exist.

### 14.1 Required scenario families

1. **Tool discovery**  
   Given an intent, does `findTool` identify the correct tool?

2. **Tool description**  
   Does `describeTool` give enough schema and examples for a model to form a valid call?

3. **Tool invocation**  
   Does `callTool` successfully route and validate downstream calls?

4. **Repair behavior**  
   Do structured errors lead the agent to correct action?

5. **Offline behavior**  
   Can Ozy search cached tools while accurately preventing unavailable invocation?

6. **Schema drift**  
   Does Ozy detect changed downstream schemas and instruct the agent to refresh/retry?

7. **Token economy**  
   Does Ozy reduce context/tool-schema burden compared with direct MCP configuration?

8. **Adapter parity**  
   Do MCP and CLI paths produce semantically equivalent broker behavior?

### 14.2 Core metrics

Discovery:

- top-1 accuracy;
- top-3 accuracy;
- mean reciprocal rank;
- wrong-server rate;
- no-match correctness.

Invocation:

- valid argument rate;
- first-call success rate;
- repair success rate;
- schema error rate;
- downstream error clarity.

Token economy:

- startup schema tokens;
- total tokens-to-success;
- largest response payload;
- extra broker calls;
- success adjusted for token cost.

### 14.3 ContextSpy integration

ContextSpy may be used as an optional measurement backend for real session/context profiling.

It should be used for evals and reports, not as a hard runtime dependency.

Preferred use:

```text
ozy eval run <scenario>
  -> direct MCP baseline
  -> Ozy broker path
  -> ContextSpy/session profiler data when available
  -> report success, cost, and regressions
```

---

## 15. CLI contract

The CLI mirrors the MCP broker operations.

Expected MVP commands:

```bash
ozy init
ozy daemon
ozy mcp
ozy index
ozy doctor
ozy list
ozy search "tool to search confluence wiki"
ozy describe atlassian.confluence_search
ozy call atlassian.confluence_search --json '{"query":"billing migration onboarding","limit":5}'
ozy eval run <scenario>
```

CLI output should support at least:

- human-readable format;
- JSON format for agents/evals;
- concise mode for token-sensitive use.

The CLI must not become a separate product with divergent behavior.

---

## 16. Security and privacy boundaries

Ozy handles tool discovery and invocation across potentially sensitive systems. It must treat secrets and private data carefully.

Requirements:

- prefer env references over literal secrets in config;
- avoid logging secrets;
- show redacted config in diagnostics;
- make external embedding providers explicit;
- allow semantic search to be disabled;
- do not send tool schemas or capability text to remote providers without clear configuration;
- preserve local-first behavior;
- keep downstream auth failures structured and non-leaky;
- distinguish read-only and write-capable tools when metadata allows.

Future write approval policies are out of MVP, but the data model should not make them impossible.

---

## 17. Observability and diagnostics

Ozy should make system state inspectable.

`ozy doctor` should eventually report:

- config validity;
- missing env vars;
- downstream server reachability;
- transport errors;
- tool counts by server;
- index freshness;
- embedding/indexing status;
- schema drift warnings;
- catalog storage status;
- MCP adapter readiness;
- common repair commands.

Agent-facing errors should include `agentInstruction` fields where possible.

---

## 18. Change governance with OpenSpec

OpenSpec-driven changes should follow this discipline:

1. **Proposal**  
   State the user/product problem, affected contracts, non-goals, risks, and expected evals.

2. **Spec delta**  
   Identify which parts of `SPEC.md` or subordinate specs change.

3. **Design**  
   Provide implementation approach only after the behavioral contract is clear.

4. **Tasks**  
   Break work into verifiable units.

5. **Implementation**  
   Keep code aligned with accepted spec and design.

6. **Evals and review**  
   Demonstrate behavior and token/cost impact.

7. **Spec update**  
   Update `SPEC.md` only when the change affects durable product behavior, architecture, contracts, or principles.

### 18.1 When to update `SPEC.md`

Update this document when a change affects:

- product mission;
- MVP boundary;
- agent-facing contracts;
- tool lifecycle;
- catalog model;
- invocation safety rules;
- search/index behavior;
- eval gates;
- adapter contracts;
- privacy/security guarantees;
- accepted architectural baselines.

Do not update this document for:

- small refactors;
- internal package moves;
- bug fixes that do not change behavior;
- transient implementation notes;
- task progress;
- TODOs better suited to issue trackers or OpenSpec tasks.

### 18.2 Proposal checklist

Every substantial proposal should answer:

```text
Problem:
Why now:
User-visible behavior:
Affected contracts:
Non-goals:
Token/context impact:
Security/privacy impact:
Offline/failure behavior:
Eval scenarios:
Migration/compatibility:
Rollback strategy:
Open questions:
```

---

## 19. Quality gates

A change is not complete until it satisfies the relevant gates below.

### 19.1 Contract gate

- Does the change preserve or intentionally update `findTool`, `describeTool`, or `callTool` behavior?
- Are response shapes still instructional?
- Do `agentInstruction`, `nextAction`, `recommendedCall`, and `avoid` fields meet the actionable, conditional, and grounded quality criteria?
- Are errors structured and repair-oriented?

### 19.2 Context gate

- Does the change increase agent-visible surface area?
- Does it increase response size?
- Does it dump schemas or results unnecessarily?
- Is the cost justified by evals or product need?

### 19.3 Runtime gate

- Does Ozy still function with semantic search disabled?
- Does Ozy still function when some downstream MCPs are offline?
- Does CLI behavior match MCP behavior?

### 19.4 Safety gate

- Are secrets protected?
- Are remote providers explicit?
- Are failed auth and offline states non-leaky and diagnosable?
- Does invocation remain live-gated?

### 19.5 Eval gate

- Is there a scenario proving success?
- Is there a regression check for core behavior?
- Is token economy measured when the change affects context?

---

## 20. Accepted architectural baseline

The current accepted baseline is:

- Go for trusted runtime, daemon, MCP adapter, CLI, broker, catalog authority, and online search.
- Optional Python worker for embeddings/indexing enrichment.
- Lexical search as mandatory baseline.
- Semantic search as optional enhancement.
- MCP adapter as required agent-agnostic interface.
- CLI adapter as required debugging/eval/shell-agent interface.
- Persistent local catalog as durable source of discovered tool capability.
- ContextSpy or similar profiler as optional eval/measurement integration, not runtime dependency.

These are current decisions, not eternal principles. Changes require proposal, tradeoff analysis, and eval plan.

---

## 21. Later extension candidates

Potential future areas:

- dynamic native downstream tool exposure for clients that handle tool list changes well;
- approval policies for write/destructive tools;
- richer auth flows;
- team/shared configuration;
- hosted or synchronized catalogs;
- usage-aware handoff;
- adaptive capability aliases learned from evals or usage;
- result caching;
- richer reranking;
- tool risk classification;
- UI/TUI for diagnostics.

Each extension must preserve the core broker model and prove that added complexity is worth the cost.

---

## 22. Anti-patterns

Avoid these unless a proposal explicitly justifies them:

- exposing all downstream tools directly by default;
- making Ozy unusable without embeddings;
- making Python required for basic search/call behavior;
- returning search results without next-action guidance;
- hiding offline/stale state;
- silently invoking changed schemas;
- logging secrets;
- building a policy engine before basic brokerage is proven;
- optimizing for demo magic over eval-proven behavior;
- letting CLI and MCP behavior drift;
- causing retry amplification by combining hidden internal retries with vague agent retry instructions;
- turning `SPEC.md` into a changelog or task tracker.

---

## 23. Glossary

**Agent-facing surface**  
The tools and instructions visible to an agent client at startup or during a task.

**Brokered invocation**  
Calling a downstream tool through Ozy using a stable `toolRef` and generic `callTool` contract.

**Capability catalog**  
Persistent Ozy registry of known downstream tool capabilities, schemas, metadata, freshness, and runtime status.

**Downstream MCP**  
An MCP server configured behind Ozy, such as Atlassian, OpenGrok, GitHub, filesystem, browser, or other tool providers.

**Instructional response**  
A response designed to tell the agent what to do next, what not to do, and how to recover from errors.

**Live-gated invocation**  
The rule that actual tool calls require reachable downstream runtime and safe schema/argument validation.

**Schema drift**  
A mismatch between cached tool schema and current downstream tool schema.

**ToolRef**  
Ozy's stable reference to a downstream tool, typically `<serverId>.<downstreamToolName>`.

---

## 24. Living document rule

`SPEC.md` should evolve deliberately.

When in doubt, keep this document focused on durable product behavior and decision constraints. Put transient planning into OpenSpec proposals/tasks. Put implementation details into design docs. Put historical rationale into ADRs. Put operational instructions into user docs.

The document is successful if a coding agent can read it and understand what Ozy is, what must not be broken, how changes should be proposed, and what evidence is required before claiming success.
