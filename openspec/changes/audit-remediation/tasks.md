# Tasks: audit-remediation

Prerequisite (separate change, do first): recover `zero-touch-lifecycle` from `git stash@{0}` onto a branch, verify tests, land it. This change assumes its `Daemon.Start`/broker-provider wiring exists.

## 1. Invocation clock (D1)

- [x] 1.1 Add `callTimeout` to `internal/config` server config (raw + resolved, ms, default 60000, `CallTimeout() time.Duration` accessor); document both knobs in `internal/config/scaffold.go` and `examples/ozy.jsonc` (accessor named `InvocationTimeout()`)
- [x] 1.2 Use `CallTimeout()` for the `callCtx` in `live.CallTool`; on our own deadline (`context.DeadlineExceeded` from `callCtx`), return `DOWNSTREAM_CALL_FAILED` with `retryable: false`, message naming `callTimeout`, instruction to narrow the call or raise the budget
- [x] 1.3 Tests: slow fake server succeeds past 5s under default `callTimeout`; deadline exceeded → non-retryable with `callTimeout` named; explicit override honored (`internal/broker/call_timeout_test.go`)

## 2. Catalog reconciliation (D2, D3)

- [x] 2.1 Add `DeleteTools(ctx, refs []string) error` to `catalog.Store`; implement in `memory.go` and `file.go` (persisted); tests for delete + survives restart (`internal/catalog/delete_test.go`)
- [x] 2.2 Indexer reconciliation in `internal/index`: per server with successful `tools/list`, delete cataloged tools absent from the listing; delete tools of servers absent from config; degrade (stale, not callable, offline) tools of unreachable/disabled servers; never delete on a failed listing (note: a successful listing outranks config absence, so fixture configs can't self-delete)
- [x] 2.3 Push the run's deletions to the embedding sink via `sink.Delete`; remove the no-op `List`-based `reconcileStaleEmbeddings` path and the dead `List` method from the sink seam
- [x] 2.4 Add catalog age: expose age-since-last-index (never-indexed distinct → nil/omitted), add `catalogAgeSeconds` to `contract.CatalogStats`, populate in broker responses
- [x] 2.5 Tests: vanished tool deleted (catalog + sink), removed-server tools deleted, flaky server degrades but keeps tools, failed listing deletes nothing (`internal/index/reconcile_test.go`); age assertion folded into group 4/8 response tests

## 3. In-band guidance and structural truncation (D4, D5)

- [ ] 3.1 Rework `applyCallBudget` in `internal/broker/live.go`: top-level array → drop whole trailing elements to fit + "showing N of M items" notice; text/other JSON → cut at last line (fallback word) boundary + partial-payload notice; notice text returned alongside the result, not embedded in `resultSummary` only
- [ ] 3.2 Adapter trailer mechanism in `internal/mcp/adapter.go`: render actionable notices (truncation, cache stamp) as one short `[ozy] …` trailing `TextContent` block, payload block byte-identical; `_meta` keeps mirroring `toolRef`/`resultSummary`
- [ ] 3.3 Tests: array truncation yields valid JSON + in-band notice; text truncation cuts at boundary + notice; un-truncated results have no trailer; payload block unchanged by trailers

## 4. findTool cost and credibility (D6, D7)

- [ ] 4.1 Honor `budgets.findTool.maxResults` (default 5) in `live.FindTool`: alternatives = up to maxResults−1 runner-ups with one-line reasons
- [ ] 4.2 Fast path: when selected tool's canonical `inputSchema` ≤ 2 KiB constant, inline full `inputSchema` + `recommendedCall` (skeleton from required fields) in `selected`, set `nextAction: callTool`, adjust `agentInstruction`; larger schemas keep preview + describe-first
- [ ] 4.3 Match reasons: list at most the 4 highest-IDF matched terms in `internal/search/lexical.go` reason strings (drops stopwords adaptively)
- [ ] 4.4 Ambiguous branch: instruction becomes compare-inlined-candidates-and-call; stop prescribing `describeTool` for inlined schemas
- [ ] 4.5 Tests: maxResults bounds candidates; small schema → inline + callTool nextAction; large schema → preview + describeTool; reason has no stopwords for a stopword-heavy query; ambiguous instruction self-consistent

## 5. Adapter honesty (D8, D9)

- [ ] 5.1 Fix `handleFind`: broker error → §9.3 error envelope with `isError: true` (mirror `handleDescribe`); test that content is never `null`
- [ ] 5.2 Set MCP `ServerOptions.Instructions` at server construction: when-to-use guidance + breadcrumb (honor `surface.capabilityBreadcrumb`); test presence and breadcrumb toggle
- [ ] 5.3 Populate `DescribeResult.RecommendedCall` from the cataloged schema's required fields; trim `OzyDescribeDescription` to promise only delivered fields; test recommendedCall present
- [ ] 5.4 Remove never-emitted surface: `DecisionChooseFromCandidates`, `DecisionKnownButUnavailable`, `ErrTypeResultTruncated` from `internal/contract`; mark `TOOL_SCHEMA_CHANGED` as reserved (not yet emitted) in SPEC §9; fix the stale `ConnectAll powers FindTool` comment in `internal/broker/live.go`

## 6. Cache visibility (D10)

- [ ] 6.1 Add additive `cachedAgeSeconds` (omitempty) to `contract.CallResult`; caching broker stamps a shallow copy on hit (store entry gains a produced-at time; never mutate the stored value)
- [ ] 6.2 Adapter renders the cache stamp as an in-band trailer (`[ozy] cached result from Ns ago`) via 3.2
- [ ] 6.3 Tests: cached hit stamped with growing age across serves, live call unstamped, stored entry unmutated

## 7. Doctor secret scan (D11)

- [ ] 7.1 Add inline-secret check to `internal/cli/doctor.go`: patterns `ghp_`, `github_pat_`, `sk-`, `AKIA`, `xox[a-z]-`, `Bearer ` on header/environment values without `{env:`; WARN names server + key + pattern kind, never the value
- [ ] 7.2 Tests: literal `ghp_…` flagged, `{env:…}` not flagged, value absent from all output formats

## 8. Docs, SPEC, and verification

- [ ] 8.1 README honesty pass: correct "Use Ozy as an MCP server" (findTool ranks the indexed catalog; index runs per the zero-touch lifecycle), document `callTimeout`, remove/adjust claims for removed contract states
- [ ] 8.2 SPEC.md §9/§13 pass: remove deleted decisions/error type, add `catalogAgeSeconds`, `cachedAgeSeconds`, fast-path and trailer contracts, reserved note for `TOOL_SCHEMA_CHANGED`
- [ ] 8.3 Full `go test ./...`, `golangci-lint`, `ozy eval run` green; update eval fixtures/corpus only where contract fields were added
- [ ] 8.4 Live smoke over stdio (mcp probe): trailer on truncation, cache stamp, instructions at initialize, fast-path findTool, non-null find error; then `graphify update .`
