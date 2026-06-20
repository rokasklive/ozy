## 1. Shared validator package

- [x] 1.1 Create `internal/schema` with `Validate(schema, args map[string]any) []string` plus the `required`/`properties`/type-match helpers, moved verbatim from `internal/eval/schema.go`
- [x] 1.2 Re-point `internal/eval` (`schema.go`, `invocation.go`) at `internal/schema` and delete the duplicated functions; confirm no `broker → eval` import cycle is introduced
- [x] 1.3 `go build ./...` and `go test ./internal/eval/...` stay green after the move

## 2. callTool argument validation

- [x] 2.1 In `live.CallTool` ([internal/broker/live.go](../../../internal/broker/live.go)), after resolving `toolRef`, look up the cataloged tool via `store.GetTool`; when found with a non-empty `InputSchema`, run `schema.Validate` before connecting downstream
- [x] 2.2 On validation problems, return a non-retryable `ARGUMENT_VALIDATION_FAILED` `*contract.Error` naming the offending fields and instructing the agent to fix args and confirm via `describeTool`
- [x] 2.3 When the tool is absent from the catalog or has no schema, skip validation and proceed to invoke (index-free invocation preserved)
- [x] 2.4 Tests: missing-required and wrong-type are rejected before any downstream connection; valid args reach the connector; missing cataloged schema still invokes

## 3. findTool structured nextAction

- [x] 3.1 In `live.FindTool`, set `NextAction` for `DecisionAmbiguous` → `describeTool` on the top candidate
- [x] 3.2 Set `NextAction` for `DecisionNoGoodMatch` → `findTool` with a "refine the query" reason
- [x] 3.3 Tests: ambiguous and no_good_match results both carry a structured `nextAction` alongside the existing `agentInstruction`

## 4. Capability breadcrumb + config opt-out

- [x] 4.1 Add a `surface` config section with `capabilityBreadcrumb` (raw `*bool`, resolved `bool` defaulting true), following the `SemanticSearch` raw/resolved pattern in `internal/config`
- [x] 4.2 Add a breadcrumb builder (`config.MCP` enabled ids): sorted, capped (12) with a "+N more" tail, returning "" when empty or disabled
- [x] 4.3 Change `mcp.New` to accept a `breadcrumb string` and append it to the base `findTool` description; leave the description unchanged when empty
- [x] 4.4 In `internal/cli/commands.go`, compute the breadcrumb from `d.Config()` honoring `surface.capabilityBreadcrumb` and pass it into `mcp.New`
- [x] 4.5 Update `ozy init` scaffold / config docs to mention `surface.capabilityBreadcrumb`
- [x] 4.6 Tests: breadcrumb present by default, bounded over the cap, omitted when disabled or no servers; config defaults to enabled when `surface` omitted

## 5. Single-representation responses

- [x] 5.1 `jsonResult` ([internal/mcp/adapter.go](../../../internal/mcp/adapter.go)): emit compact `json.Marshal` in `Content` and stop setting `StructuredContent`
- [x] 5.2 `callResult`: carry the downstream result once in `Content` (compact JSON for structured, raw text for string), keep metadata in `_meta`, drop the duplicate `StructuredContent`
- [x] 5.3 Update adapter tests to assert each payload appears once and `structuredContent` is unset for the three tools

## 6. Verification

- [x] 6.1 `go build ./...`, `go test ./...`, and `golangci-lint run` pass
- [x] 6.2 Run `openspec validate broker-response-guardrails --strict` clean
- [x] 6.3 Bench run (Docker Desktop + ContextSpy, mode=ozy, deepseek-chat): `pass=true`, 5/5 grading criteria, broker drove findTool→describeTool→callTool — **no regression**. Breadcrumb cost is small/bounded (findTool def ~185→218 tok/req for 7 servers). Single-representation token drop is **not observable via OpenCode**: OpenCode re-serializes every tool result to compact JSON before the model, so Ozy's old indentation + duplicate `structuredContent` never reached the model (old transcript findTool output was already compact, ~same size as new). The de-double-wrap is still correct wire/protocol hygiene and benefits clients that don't normalize.
