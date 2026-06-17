## 1. Sidecar readiness: liveness vs. warm-up

- [x] 1.1 Add a readiness/warm-up probe to the sidecar client (`internal/sidecar/client.go`) distinct from the fast liveness `Health`: it loads the model and runs one probe query under a caller-supplied generous timeout, returning model/dim/backend/vector-count on success.
- [x] 1.2 Define a single "available" predicate = readiness probe succeeded (model loaded + probe query returns); a bare liveness ping is NOT "available".
- [x] 1.3 Ensure a liveness/health timeout never triggers `Close()` while a warm-up is in progress — give the warm-up its own deadline so a slow cold model download is not aborted (`internal/sidecar/client.go`, `internal/daemon/daemon.go` `sidecarSidecarHealthTimeout`).

## 2. Partial/corrupt model cache self-heal (Python sidecar)

- [x] 2.1 In the sidecar model-load path (`sidecar/`), detect an incomplete/corrupt model load, clear the model cache directory, and re-fetch the model exactly once before giving up.
- [x] 2.2 Return a structured "model download incomplete" result the Go client maps to an actionable reason, instead of an opaque start failure.

## 3. Install warms up and verifies the model

- [x] 3.1 Replace the no-op `stepDownloadEmbeddingAssets` (`internal/installer/steps.go`) with a real warm-up: start the sidecar and run the readiness probe under `sidecar.DefaultProvisionTimeout`, setting `semanticOK` only on a verified success.
- [x] 3.2 On warm-up failure with Python present, return an actionable `StepError` (failed step, cause, retry-safe, next command, log path) while still letting the install complete in lexical-only mode.
- [x] 3.3 Keep the no-Python path degrading to lexical-only unchanged, and stop the step from reporting "model present" without a verified load.

## 4. `ozy index` provisions and embeds

- [x] 4.1 Add `Daemon.Index(ctx, status) *index.Summary` (`internal/daemon/daemon.go`) that reuses `provisionSidecar`, then runs the indexer with the embedding sink attached when `semanticAvailable`; collapse the duplicated wiring in `runStartupIndex` to call it.
- [x] 4.2 Rewire `indexCmd` (`internal/cli/commands.go:88`) to call `d.Index(...)` instead of constructing a bare `ozyindex.New(d.Store(), nil)`.
- [x] 4.3 Add `EmbeddedCount` and `VectorCount` to `index.Summary`, populate them in `flushEmbeddings`, and render them in `Summary.Render`.
- [x] 4.4 Add the loud-fail guard in `Indexer.Run`: when the sink is available and `ToolsIndexed > 0` but zero tools embedded, set `OK=false` with a `SEMANTIC_SEARCH_UNAVAILABLE` reason and next step.

## 5. Honest, consistent degraded reporting

- [x] 5.1 Make `ozy doctor`'s embedding check use the same readiness predicate (`internal/cli/doctor.go` `newSidecarProbe`/`embeddingCheck`) so it reports available only when vectors are queryable, and include the specific reason when unavailable.
- [x] 5.2 Ensure the daemon degraded notice and the `findTool` degraded surface name the specific reason and the next command (e.g. `ozy doctor` / `ozy index`), not just "lexical-only" (`internal/daemon/daemon.go`, `internal/broker/live.go`).

## 6. callTool returns the payload directly

- [x] 6.1 Add `callResult(*contract.CallResult)` in `internal/mcp/adapter.go`: a string result becomes `TextContent`, a structured result becomes `StructuredContent`, and `toolRef`/`resultSummary` are carried once as metadata — no second stringified copy of the whole envelope.
- [x] 6.2 Switch the `handleCall` success path to `callResult`; keep `jsonResult` for `findTool`/`describeTool` and for the error envelope.

## 7. Tests and verification

- [x] 7.1 Sidecar client test: readiness probe is distinct from liveness, and a warm-up is not aborted by a short liveness timeout (fake/process double).
- [x] 7.2 Python sidecar test: corrupt-cache detection clears the cache and re-fetches exactly once.
- [x] 7.3 Installer test (`internal/installer/steps_test.go`): `stepDownloadEmbeddingAssets` warms up on success, returns an actionable error on failure, and degrades cleanly without Python.
- [x] 7.4 Index/daemon tests: a semantic-enabled run embeds and reports `EmbeddedCount`/`VectorCount`; indexed-but-not-embedded sets `OK=false` (`internal/index/index_test.go`); `Daemon.Index` provisions and wires the sink (`internal/daemon/daemon_test.go`).
- [x] 7.5 Doctor test: reports semantic available only when the readiness probe succeeds, and surfaces the reason when degraded.
- [x] 7.6 Adapter test (`internal/mcp/`): a successful `callTool` surfaces the downstream payload directly (text and structured cases) and errors stay §9.3 structured failures.
- [x] 7.7 `gofmt`, `go vet ./...`, `go test ./...`, then an end-to-end check from a clean state dir: install → `ozy doctor` (semantic available) → `ozy index` (embedded > 0, vector count > 0) → semantic `findTool` hit → `callTool` payload not double-wrapped. Run `graphify update .` to refresh the graph.
