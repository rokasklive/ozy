## Why

On a clean second device, semantic search silently never worked: the installer reported success but the embedding model was only partially downloaded, the sidecar could not start, and nothing told the user what went wrong or how to recover. Even after starting the sidecar by hand, `ozy index` claimed to index ~160 tools while embedding **zero** of them, yet `ozy doctor` reported "semantic available" — so the catalog filled but vector storage stayed empty. The net effect is lexical-only search with no signal, which resolves the wrong tools, forces the agent to work around Ozy, and erodes trust in the broker. Search quality cannot improve until install, feedback, and the semantic layer are robust and honest, so this change makes that foundation reliable.

## What Changes

- **Install actually provisions the model.** The `DownloadEmbeddingAssets` step pre-downloads and verifies the embedding model during install (today it is a no-op that defers the fetch to first query), with a generous timeout, partial-download detection, and a clean retry that clears a corrupt cache instead of failing opaquely.
- **Stop killing the model download mid-flight.** Sidecar startup separates a fast liveness check from a one-time model warm-up, so a cold model fetch is no longer aborted by the 10s/15s health timeout that currently corrupts the on-disk cache (the likely cause of "sidecar won't start").
- **`ozy index` embeds when semantic is enabled.** The standalone index path provisions the sidecar and attaches the embedding sink (today it is hardwired lexical-only via `index.New(store, nil)`), so indexed tools are actually written to vector storage.
- **Honest index + doctor reporting.** `ozy index` reports embedded/vector counts (not just `toolsIndexed`) and fails loudly when semantic is enabled but nothing was embedded; `ozy doctor`, `ozy index`, and `findTool` agree on a single definition of "semantic available" = vectors are queryable, and every degraded surface states the reason and the next command to run.
- **BREAKING: `callTool` responses are no longer double-wrapped.** The downstream tool payload is surfaced directly in the MCP content channel instead of being JSON-stringified inside an Ozy envelope and duplicated into structured content, so agents read the actual result without unwrapping two layers.

## Capabilities

### New Capabilities
<!-- None — this change hardens existing behavior; no new capability is introduced. -->

### Modified Capabilities
- `installer`: the embedding-assets step SHALL download and verify the model during install, recover from a partial/corrupt download, and surface an actionable failure instead of reporting silent success.
- `embedding-sidecar`: provisioning/health SHALL distinguish liveness from model warm-up and SHALL NOT abort an in-progress model download; "available" SHALL mean the model is loaded and vectors are queryable, and the reported reason SHALL be actionable when unavailable.
- `tool-discovery`: `ozy index` SHALL provision the sidecar and embed indexed tools when semantic is enabled, SHALL report embedded/vector counts, and SHALL fail loudly when semantic is enabled but no tools were embedded.
- `mcp-adapter`: `callTool` SHALL return the downstream payload directly in the response content without an extra Ozy envelope wrapped and JSON-stringified around it.

## Impact

- Code: `internal/installer/steps.go` (`stepDownloadEmbeddingAssets`, verification), `internal/sidecar/provision.go` + `internal/sidecar/client.go` (model warm-up vs. liveness, partial-cache recovery), `internal/daemon/daemon.go` (health/warm-up timeouts), `internal/cli/commands.go` (`indexCmd` sink wiring) + `internal/cli/doctor.go` (shared availability definition), `internal/index/index.go` (summary counts, loud-fail guard), `internal/mcp/adapter.go` (`handleCall`/`jsonResult` response shape).
- Behavior: first-run installs take longer (model is fetched up front) but semantic search works without manual intervention; degraded states are always explained.
- Compatibility: `callTool` response shape changes for MCP clients/agents (BREAKING); `findTool`/`describeTool` shapes are unchanged.
- No new third-party dependencies; model/package versions stay pinned as today.
