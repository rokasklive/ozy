## Context

Semantic search is wired in three independent places that do not agree, which is
why it silently failed on a clean device:

- **Install** — `stepDownloadEmbeddingAssets` (`internal/installer/steps.go`) is a
  no-op: it prints "model is fetched on first semantic query" and never downloads
  the model. The model is therefore fetched lazily by whichever path first calls
  the sidecar.
- **Startup / doctor** — the daemon's `provisionSidecar` health-checks the sidecar
  under a 10s timeout (`sidecarSidecarHealthTimeout`) and calls `sc.Close()` on
  failure; `ozy doctor`'s `newSidecarProbe` uses a 15s timeout. A cold FastEmbed
  download easily exceeds both, so the first warm-up is killed mid-download,
  leaving a partial/corrupt model cache that makes every later start fail.
- **`ozy index`** — `indexCmd` builds `ozyindex.New(d.Store(), nil)` with no sink,
  and `app.load()`/`daemon.New` never provisions the sidecar. The standalone index
  path is structurally lexical-only: it persists the catalog (`toolsIndexed=160`)
  but `flushEmbeddings` is never reached, so vector storage stays empty — while
  `ozy doctor` independently provisions its own probe and reports "available."

Separately, `handleCall` → `jsonResult` (`internal/mcp/adapter.go`) serializes the
whole §9.3 `CallResult` envelope into a `TextContent` string *and* duplicates it
into `StructuredContent`, burying the downstream payload inside a stringified
envelope inside the MCP result — the "double-wrap" agents hit.

Constraints: the sidecar is optional and must degrade to lexical-only without
failing the daemon; no new third-party dependencies; model/package versions stay
pinned; CLI and MCP must stay behavior-equivalent through the shared broker.

## Goals / Non-Goals

**Goals:**
- A clean install ends with a loaded, queryable embedding model — or an actionable
  reason why not — with no manual sidecar start required.
- `ozy index`, `ozy doctor`, and `findTool` agree on one definition of
  "semantic available" = model loaded and a probe query returns.
- `ozy index` actually embeds indexed tools into vector storage when semantic is
  enabled, and its summary cannot claim success on an empty vector store.
- A cold or partial model download is never aborted by a liveness timeout, and a
  corrupt cache self-heals on the next run.
- `callTool` returns the downstream payload directly, no envelope-in-text wrap.

**Non-Goals:**
- Improving ranking/search *quality* itself (re-rankers, better embed text,
  hybrid fusion tuning) — this change only makes the semantic layer actually run;
  quality work builds on it.
- Bundling or vendoring the model offline; it is still fetched from the network on
  first install (behind the existing consent gate).
- Changing the `findTool`/`describeTool` response encoding.

## Decisions

### 1. Model is warmed up and verified during install
`stepDownloadEmbeddingAssets` stops being a no-op. After the venv is provisioned it
starts the sidecar and issues a **warm-up** that loads the model and runs one probe
embed/query, under a generous timeout (reuse `sidecar.DefaultProvisionTimeout`, 5m,
not the 10–15s liveness budget). Success sets `semanticOK` and reports "semantic
available"; failure produces a `StepError` (actionable, per the installer's
existing error contract) and the install still completes lexical-only.

*Why:* the step is named and verified for exactly this ("model present", "doctor
reports semantic available"); making it real is the smallest change that removes
the lazy-first-query landmine. *Alternative rejected:* keep lazy download but raise
every caller's timeout — leaves three callers to keep in sync and still races the
first real query.

### 2. Liveness and warm-up are separate operations with separate timeouts
Split the single health call into (a) a fast **liveness** probe (process answered,
short timeout — fine to fail fast) and (b) a **warm-up/readiness** probe that may
trigger a model download (long timeout, must not kill the process on timeout).
`provisionSidecar` and `newSidecarProbe` both use this split, so neither calls
`Close()` on a process that is still downloading. "Available" is defined as a
successful readiness probe (model loaded + probe query returns), not a bare health
ping.

*Why:* the root cause of "sidecar won't start" is a short deadline killing a long
download. *Alternative rejected:* one timeout sized for downloads — makes a
genuinely-dead sidecar hang startup for minutes.

### 3. Partial/corrupt model cache self-heals in the sidecar
Recovery lives in the Python sidecar, which owns model loading: on a model-load
failure that looks like an incomplete/corrupt download, it clears the model cache
directory and re-fetches **once** before returning a structured error. The Go side
stays cache-location-agnostic.

*Why:* the sidecar already knows the FastEmbed cache path and load semantics; doing
it in Go would hard-code cache internals across the language boundary. *Open:* exact
detection signal (load exception vs. checksum) — settle during apply.

### 4. One index path that provisions and embeds
Add `Daemon.Index(ctx, status) *index.Summary` that reuses `provisionSidecar`, then
runs the indexer with the sink attached when `semanticAvailable` (the same wiring
`runStartupIndex` already does). `indexCmd` calls `d.Index(...)` instead of
constructing a bare `index.New(store, nil)`. This deletes the divergent CLI wiring
and guarantees the CLI and daemon index identically.

`index.Summary` gains `EmbeddedCount` and `VectorCount`. `Indexer.Run` adds a
guard: when the sink is available and `ToolsIndexed > 0` but nothing was embedded,
set `OK=false` with a `SEMANTIC_SEARCH_UNAVAILABLE` reason and next step — the
"indexed-but-not-embedded" loud failure.

*Why:* the bug is duplicated, drifting wiring; collapsing to one daemon method is
the lazy fix and the loud-fail guard makes the class of bug self-reporting.
*Alternative rejected:* have `indexCmd` provision+wire the sink itself — re-creates
the duplication this is removing.

### 5. `callTool` success returns the payload directly
Give the adapter a `callResult(res *contract.CallResult)` helper separate from
`jsonResult`. On success: if `res.Result` is a string, it becomes the `TextContent`;
if structured, it becomes `StructuredContent`; `toolRef`/`resultSummary` ride along
as metadata (a thin structured field set / `_meta`) — carried once, never a second
stringified copy of the whole envelope. `findTool`/`describeTool` keep `jsonResult`.
Errors keep the §9.3 envelope via the existing error path.

*Why:* agents consume `callTool` output as the tool's result; everything else is a
decision payload meant to be read whole. *Alternative rejected:* stop double-emit
globally (text XOR structured) — risks breaking MCP clients that only read one
channel, for no benefit on find/describe.

## Risks / Trade-offs

- **Longer first install (model download up front)** → mitigated: behind the
  existing consent gate, marker-cached so reruns skip, and failure degrades to
  lexical-only with an actionable error rather than blocking.
- **Warm-up could hang if the sidecar is genuinely wedged** → mitigated: liveness
  probe fails fast and bounds the warm-up; warm-up timeout (5m) is a ceiling, and a
  timeout reports unavailable rather than hanging the daemon forever.
- **Self-heal could loop on a persistently bad network** → mitigated: re-fetch is
  capped at one retry, then a structured "unavailable" with reason.
- **BREAKING `callTool` shape** → mitigated: the new shape is strictly easier to
  consume (raw payload vs. stringified nested envelope); called out as BREAKING and
  covered by adapter tests asserting the downstream payload is directly readable.
- **Three call sites must adopt the liveness/warm-up split** (installer, daemon,
  doctor) → mitigated: the split is a single sidecar-client method they all call, so
  the readiness definition has one source of truth.

## Migration Plan

No data migration. On upgrade, the first `ozy index` (or daemon start) with semantic
enabled warms up and embeds; a previously-corrupt model cache self-heals on that
run. Rollback is reverting the change — the catalog is untouched and lexical search
is unaffected. Agents consuming `callTool` see the new (simpler) response shape
immediately; no opt-in flag.

## Open Questions

- Exact partial-download detection signal in the sidecar (load exception class vs.
  explicit size/checksum check) — decide during apply against FastEmbed behavior.
- Final carrier for `callTool` metadata: structured field set vs. MCP `_meta` —
  pick whichever the go-sdk surfaces most cleanly to clients during apply.
