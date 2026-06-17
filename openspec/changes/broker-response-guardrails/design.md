## Context

Four independent improvements to the live broker and MCP adapter, grouped because they all touch the agent-facing response path and share one theme: make Ozy's responses correct and self-directing without enlarging the three-tool surface.

Current state grounded in code:
- `live.CallTool` ([internal/broker/live.go:209](../../../internal/broker/live.go)) resolves the `toolRef` against config and forwards `args` straight to the downstream `tools/call` — no local schema check. The eval suite compensates with `callWithModeling` ([internal/eval/invocation.go:107](../../../internal/eval/invocation.go)), which explicitly "models the agent-side schema checks Ozy's broker does not yet perform" using a self-contained `validateArgs` ([internal/eval/schema.go:13](../../../internal/eval/schema.go)).
- `live.FindTool` sets a structured `NextAction` only in the `DecisionUse` branch; `ambiguous` / `no_good_match` get `AgentInstruction` prose only.
- The `findTool` description is a static const ([internal/mcp/adapter.go:27](../../../internal/mcp/adapter.go)); the adapter is built with only `broker + version` ([internal/cli/commands.go:64](../../../internal/cli/commands.go)).
- `jsonResult` sets both `Content` (indented JSON) and `StructuredContent` to the same payload; the three tools declare no `outputSchema`, so the SDK's de-dup path (server.go:384-393) never runs and both copies go on the wire.

## Goals / Non-Goals

**Goals:**
- Reject malformed `callTool` arguments locally, before any downstream contact, with the existing `ARGUMENT_VALIDATION_FAILED` envelope.
- Give `ambiguous` and `no_good_match` a structured `nextAction`.
- Advertise a bounded list of reachable downstream servers in the `findTool` description, default on, opt-out via `ozy.jsonc`.
- Stop emitting each payload twice; shrink per-response tokens.

**Non-Goals:**
- Live-schema drift detection (`TOOL_SCHEMA_CHANGED`) in the broker — deferred (see Decisions).
- Session-state tracking / forcing `describeTool` before `callTool` (the rejected "bouncer"; schema validation is the stateless equivalent).
- A `nextAction` for `catalog_empty` (its remedy is `ozy index`, a CLI command, not one of the three Ozy tools).
- Renaming the three tools or changing `toolRef` format.

## Decisions

**1. Lift `validateArgs` into a new `internal/schema` package.**
It is pure (`map[string]any` → `[]string`, no eval types). `eval` imports `broker`, so `broker` cannot import `eval` (cycle). A neutral `internal/schema` package is imported by both. *Alternatives:* `internal/contract` (rejected — keep it types-only) and `internal/catalog` (rejected — it is the storage seam, not validation logic).

**2. `CallTool` reads the cataloged schema via the store it already holds.**
`live` embeds `skeleton{store}`. At the top of `CallTool`, look up `store.GetTool(ctx, toolRef)`; if found with a non-empty `InputSchema`, run `schema.Validate` and return `ARGUMENT_VALIDATION_FAILED` on problems. If the tool is absent or has no schema, skip and proceed — this preserves the `tool-invocation` guarantee that `callTool` works without a prior `ozy index`.

**3. Defer drift detection.**
`TOOL_SCHEMA_CHANGED` requires comparing cataloged vs *live* schema, i.e. a `tools/list` round trip on every call. Cataloged-schema validation already catches the dominant failure (an agent hallucinating or omitting args). Drift is rarer and pays latency on the happy path. Revisit only if the bench shows drift-caused failures matter.

**4. `nextAction` in the two missing branches.**
`ambiguous` → `{tool: "describeTool", toolRef: <top candidate>, arguments: {toolRef}}`. `no_good_match` → `{tool: "findTool", reason: "retry with a more specific query"}` (no auto-refined query — the reason carries the instruction). Prose `AgentInstruction` stays as the human-readable mirror.

**5. Breadcrumb is computed by the caller, passed as a string.**
Change the constructor to `mcp.New(broker, version, breadcrumb string)`. `commands.go` builds the breadcrumb from the daemon's catalog/config (it has `d.Store()` and `d.Config()`), honoring `surface.capabilityBreadcrumb`, and the adapter just appends it to the base description. Keeps the adapter free of catalog/config imports. *Source:* prefer catalog servers (`Store.Servers`) when populated, else configured server ids from `config.MCP`. *Bound:* sorted, capped (~12), with a "+N more" overflow tail. *Lifecycle:* built once at construction (static for the stdio session); refreshed on restart/reindex. *Alternative:* live `listChanged` notifications (rejected — YAGNI; restart refresh is enough). Wording avoids implying liveness: "Available downstream servers: …", not "Connected".

**6. Single representation + compact JSON.**
`jsonResult` (findTool/describeTool/errors): emit compact `json.Marshal` in `Content`, drop `StructuredContent`. `callResult` (success): carry the downstream result once in `Content` (compact JSON for structured, raw text for string) and keep Ozy's metadata in `_meta`; drop the duplicate `StructuredContent`. Switching `MarshalIndent` → `Marshal` is a free token win. Validate the token delta with ContextSpy on the bench.

## Risks / Trade-offs

- **Dropping `structuredContent` could break a client that reads only that field** → MCP requires clients to handle `content`; `structuredContent` is supplementary and tied to an `outputSchema` Ozy does not declare. Bench client (opencode) reads `content`. Confirm on the bench before merge.
- **Validation false-positives on exotic schemas** → the validator only checks `required` presence and declared scalar/array/object types and allows extra fields; unknown/unconstrained types pass. It will not reject anything the eval suite's modeled check wouldn't already reject.
- **Stale breadcrumb** (server added/removed mid-session) → description refreshes on restart/reindex; acceptable for a stdio multiplexer. Documented, not mitigated further.
- **Breadcrumb leaking internal server ids** → ids are operator-chosen labels already visible in `toolRef`s; no secrets. Bound the count to avoid bloating the always-loaded description.

## Migration Plan

No data migration. Backward compatible: configs without a `surface` section get the breadcrumb on by default. The one wire-shape change is removing the duplicate `structuredContent`; roll back by reverting the adapter if any client regresses.

## Open Questions

- Final breadcrumb source and exact cap/wording — tune against the bench. **Resolved during apply:** breadcrumb is built from enabled `config.MCP` ids (no catalog/index dependency), capped at 12 with a "+N more" tail; catalog-derived capability categories deferred until the bench shows they help.
- `callResult`: content-only vs. retaining `structuredContent` for structured downstream results. **Resolved during apply:** content-only across all three tools (compact JSON in `content`, no `structuredContent`), honoring the "carried once" spec scenario; the prior `TestAdapter_CallToolPreservesStructuredContent` was updated accordingly. Revisit only if a structured-content-only client regresses on the bench.
