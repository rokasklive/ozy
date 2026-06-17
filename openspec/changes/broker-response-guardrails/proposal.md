## Why

Ozy's live broker forwards `callTool` arguments straight to the downstream server with no local schema check ([internal/broker/live.go:243](../../../internal/broker/live.go)), so an agent that omits or mistypes an argument only finds out through a confusing downstream error — the eval suite already *models* the missing guardrail in [internal/eval/invocation.go:107](../../../internal/eval/invocation.go) ("the agent-side schema checks Ozy's broker does not yet perform"). Three smaller ergonomics gaps compound it: `findTool` returns a structured `nextAction` only on the confident `use` path (ambiguous / no-match agents get prose only), the agent-facing tool descriptions never name which downstream servers are actually reachable, and every control response is emitted twice on the wire — the full payload as JSON text in `content` **and** as `structuredContent` — with no declared `outputSchema` to justify the copy.

## What Changes

- **Argument validation in `callTool`**: before contacting a downstream server, validate agent-supplied arguments against the tool's cataloged input schema (when one is cataloged) and return the already-defined `ARGUMENT_VALIDATION_FAILED` structured failure on a mismatch. Lift the self-contained validator from `internal/eval` into a shared package so broker and eval share one implementation.
- **Structured `nextAction` on more `findTool` decisions**: add a concrete next-call shape to the `ambiguous` and `no_good_match` decisions, not just `use`.
- **Dynamic capability breadcrumb**: append a bounded summary of available downstream servers to the `findTool` description so the agent sees what Ozy can reach before its first call. Default **on**, opt-out via `ozy.jsonc`.
- **Single-representation responses**: emit each agent-facing payload once (compact JSON in `content`) instead of duplicating it across `content` and `structuredContent`.

No breaking changes to the three-tool surface (`findTool`/`describeTool`/`callTool`) or the `toolRef` format.

## Capabilities

### New Capabilities

_(none — all changes modify existing capabilities)_

### Modified Capabilities

- `tool-invocation`: `callTool` validates arguments against the cataloged schema before invoking, returning `ARGUMENT_VALIDATION_FAILED` instead of forwarding bad arguments to the downstream server.
- `tool-search`: `findTool` returns a structured `nextAction` for the `ambiguous` and `no_good_match` decisions, not only `use`.
- `mcp-adapter`: `findTool`'s advertised description carries a bounded breadcrumb of available downstream servers; agent-facing tool responses emit a single representation per payload rather than duplicating it across `content` and `structuredContent`.
- `configuration`: a new opt-out setting disables the capability breadcrumb.

## Impact

- **Code**: `internal/broker/live.go` (validation + nextAction), `internal/mcp/adapter.go` (breadcrumb + single-representation), `internal/cli/commands.go` (compute and pass the breadcrumb), `internal/config` (opt-out flag), a new shared `internal/schema` package (the lifted validator), and `internal/eval/schema.go` (re-point to the shared validator).
- **Behavior**: agents get an earlier, structured correction loop on bad arguments; fewer tokens per agent-facing response; richer pre-call context in the `findTool` description.
- **Constraints / risk**: validation is best-effort — it is skipped when no schema is cataloged, because `callTool` must keep working without a prior `ozy index` (per `tool-invocation`). Live-schema drift detection (`TOOL_SCHEMA_CHANGED`) is explicitly **out of scope** here: it needs a per-call `tools/list` round trip and is deferred (see design.md).
