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

- [x] 3.1 Rework `applyCallBudget` in `internal/broker/live.go`: top-level array → drop whole trailing elements to fit + "showing N of M items" notice; text/other JSON → cut at last line (fallback word) boundary + partial-payload notice; notice carried in new `CallResult.Notices` (contract addition), not embedded in `resultSummary`
- [x] 3.2 Adapter trailer mechanism in `internal/mcp/adapter.go`: `CallResult.AllNotices()` (notices + cache stamp) rendered as one `[ozy] …` trailing `TextContent` block, payload block byte-identical; `_meta` keeps mirroring; CLI `Render` shows the same notices
- [x] 3.3 Tests: array truncation yields valid JSON + in-band notice; text truncation cuts at boundary + notice; un-truncated results have no trailer; payload block unchanged (`internal/broker/truncation_test.go`, `internal/mcp/trailer_test.go`)

## 4. findTool cost and credibility (D6, D7)

- [x] 4.1 Honor `budgets.findTool.maxResults` (default 5, `EffectiveMaxResults()`) in `live.FindTool`: alternatives = up to maxResults−1 runner-ups from the ranking with one-line reasons
- [x] 4.2 Fast path: canonical `inputSchema` ≤ 2 KiB (or `includeFullSchemas: true`, making that knob honest too) → inline full `inputSchema` + `recommendedCall` (typed skeleton from required fields), `nextAction: callTool`; larger schemas keep preview + describe-first
- [x] 4.3 Match reasons: at most the 4 highest-IDF matched terms (`topSignalTerms` in `internal/search/lexical.go`); no stopword list — the corpus decides
- [x] 4.4 Ambiguous branch: compare-inlined-candidates-and-call (`nextAction: callTool`, no auto-picked toolRef); `describeTool` no longer prescribed for delivered schemas
- [x] 4.5 Tests: maxResults bounds candidates; small schema → inline + callTool nextAction; large schema → preview + describeTool; reason drops stopwords (`find_fastpath_test.go`, `search/reason_test.go`); ambiguous self-consistency in `find_nextaction_test.go`

## 5. Adapter honesty (D8, D9)

- [x] 5.1 Fix `handleFind`: broker error → §9.3 error envelope with `isError: true`; non-contract failures synthesized as `CONFIG_ERROR` (a find failure is local, not downstream); test asserts never-`null`
- [x] 5.2 Set MCP `ServerOptions.Instructions` (`OzyServerInstructions` + breadcrumb when enabled); tests cover presence and breadcrumb toggle (`internal/mcp/honesty_test.go`)
- [x] 5.3 Populate `DescribeResult.RecommendedCall` (typed skeleton from required fields); trimmed `OzyDescribeDescription` to promise only delivered fields (`describe_recommended_test.go`)
- [x] 5.4 Removed `DecisionChooseFromCandidates`, `DecisionKnownButUnavailable`, `ErrTypeResultTruncated` from `internal/contract` (+ eval `knownDecision` trimmed); `TOOL_SCHEMA_CHANGED` marked reserved in the constant doc (SPEC §9 pass lands in 8.2); fixed stale `ConnectAll powers FindTool` comment

## 6. Cache visibility (D10)

- [x] 6.1 Add additive `cachedAgeSeconds` (omitempty) to `contract.CallResult` (landed with 3.x); caching broker stamps a shallow copy on hit via `stampedCallResult` (entry gains `produced`; stored value never mutated)
- [x] 6.2 Adapter renders the cache stamp as an in-band trailer via `AllNotices()` (landed with 3.2; covered by `TestCallResult_CachedAgeRendersInBandStamp`)
- [x] 6.3 Tests: hit stamped, live call unstamped, stored entry unmutated, hits are distinct copies (`internal/broker/cache_stamp_test.go`)

## 7. Doctor secret scan (D11)

- [x] 7.1 Add inline-secret check to `internal/cli/doctor.go` (`secretHygieneChecks` on the RAW config — resolved values would false-flag proper `{env:}` refs): field-prefix matching for `ghp_`/`gho_`/`ghs_`, `github_pat_`, `sk-`, `AKIA`, `xox[bpas]-`, plus `Bearer ` literals; WARN names server + field + key + kind + rotate advice, never the value
- [x] 7.2 Tests: literal `ghp_…` flagged without leaking, `{env:…}` + sk- inside words pass, bearer literal flagged (`internal/cli/secret_hygiene_test.go`)

## 8. Docs, SPEC, and verification

- [ ] 8.1 README honesty pass: correct "Use Ozy as an MCP server" (findTool ranks the indexed catalog; index runs per the zero-touch lifecycle), document `callTimeout`, remove/adjust claims for removed contract states
- [ ] 8.2 SPEC.md §9/§13 pass: remove deleted decisions/error type, add `catalogAgeSeconds`, `cachedAgeSeconds`, fast-path and trailer contracts, reserved note for `TOOL_SCHEMA_CHANGED`
- [ ] 8.3 Full `go test ./...`, `golangci-lint`, `ozy eval run` green; update eval fixtures/corpus only where contract fields were added
- [ ] 8.4 Live smoke over stdio (mcp probe): trailer on truncation, cache stamp, instructions at initialize, fast-path findTool, non-null find error; then `graphify update .`
