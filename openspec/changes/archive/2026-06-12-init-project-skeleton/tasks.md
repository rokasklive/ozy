## 1. Module and scaffold

- [x] 1.1 Initialize the Go module (`go.mod`) pinned to a recent stable Go version
- [x] 1.2 Create the repository layout: `cmd/ozy/main.go` plus `internal/` packages (`cli`, `config`, `broker`, `catalog`, `daemon`, `mcp`, `render`)
- [x] 1.3 Add `cmd/ozy/main.go` that builds, prints `--version`, and delegates to the CLI root command
- [x] 1.4 Verify `go build ./...` produces a single `ozy` binary that runs `--help` and `--version`

## 2. Configuration

- [x] 2.1 Define the typed config model mirroring `SPEC.md` §11 (`version`, `servers`, `embedding`, `search`, `budgets`)
- [x] 2.2 Implement config discovery: documented default path plus override via flag/env
- [x] 2.3 Implement `${ENV}` reference resolution at load time; record unresolved references as structured diagnostics naming the variable and owning server
- [x] 2.4 Implement validation returning structured `CONFIG_ERROR` that names the offending field; missing file returns repair guidance (`ozy init`)
- [x] 2.5 Implement a `Redacted()` view that masks secret-sourced values for diagnostics/logging
- [x] 2.6 Unit tests: valid load, override path, missing file, missing env var, invalid field, redaction

## 3. Catalog store

- [x] 3.1 Define the `catalog.Store` interface over servers/tools/schemas/freshness/runtime status (`SPEC.md` §8)
- [x] 3.2 Implement an in-memory placeholder `Store` (empty by default)
- [x] 3.3 Unit test: empty-store queries return cleanly (no panic) to support the `catalog_empty` path

## 4. Broker seam and contract models

- [x] 4.1 Define result/error structs that JSON-marshal to the §9 shapes (`decision`, `confidence`, `nextAction`, `agentInstruction`, `ok`, `error.type`, `error.retryable`)
- [x] 4.2 Add error-type constants from §9.3 plus the skeleton-only `NOT_IMPLEMENTED` marker
- [x] 4.3 Define the `broker.Broker` interface (`FindTool`, `DescribeTool`, `CallTool`)
- [x] 4.4 Implement the skeleton broker: `FindTool` returns `catalog_empty`/`no_good_match` with instruction; `describeTool` returns `TOOL_NOT_FOUND`-shaped result; `callTool` returns a structured not-yet-callable failure with grounded `agentInstruction`
- [x] 4.5 Unit tests asserting each stub response conforms to its §9 contract shape

## 5. CLI interface

- [x] 5.1 Add the cobra root command with global `--config` and `--format` (human/json/concise) flags
- [x] 5.2 Register all §15 subcommands: `init`, `daemon`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, `eval run`, each with help text
- [x] 5.3 Implement the `render` package: human (default), `json` (single document), and concise renderers over the typed results
- [x] 5.4 Wire broker-backed commands (`search`/`describe`/`call`/`list`) to the shared `broker.Broker`; unimplemented operations emit a structured `NOT_IMPLEMENTED` result and exit non-zero
- [x] 5.5 Implement `ozy init` to scaffold a starter config at the default location
- [x] 5.6 Tests: `--help` lists all commands; `--format json` yields one valid JSON document; unimplemented command returns `NOT_IMPLEMENTED` and non-zero exit

## 6. MCP adapter

- [x] 6.1 Add `internal/mcp` wrapping the chosen MCP Go SDK behind an internal interface (broker never imports the SDK)
- [x] 6.2 Register exactly `findTool`, `describeTool`, `callTool` with input schemas; expose no downstream tools
- [x] 6.3 Route each tool invocation through the shared `broker.Broker`
- [x] 6.4 Implement `ozy mcp` to serve the protocol over its transport
- [x] 6.5 Tests: tool list contains exactly the three tools; `findTool` returns a §9.1-shaped result (incl. `catalog_empty`); `callTool` returns a §9.3-shaped structured failure

## 7. Daemon runtime

- [x] 7.1 Implement `ozy daemon`: load config, construct catalog store and broker, report ready state
- [x] 7.2 Handle interrupt/termination signals for clean shutdown
- [x] 7.3 Refuse to start on invalid config (structured `CONFIG_ERROR`, non-zero exit)
- [x] 7.4 Ensure startup succeeds with semantic search / embedding worker disabled or absent (graceful degradation)
- [x] 7.5 Tests: ready-state on valid config; non-zero exit on invalid config; starts with semantic search disabled

## 8. Doctor and diagnostics

- [x] 8.1 Implement minimal `ozy doctor`: config validity, missing env vars (redacted), and adapter/daemon readiness
- [x] 8.2 Support `--format json` output for `doctor`
- [x] 8.3 Test: `doctor` reports a missing env var without leaking secret values

## 9. Tooling and CI

- [x] 9.1 Add a `Makefile` with `build`, `test`, and `lint`/`vet` targets
- [x] 9.2 Add a CI workflow (`.github/workflows/ci.yml`) running build, test, and lint; fail on any step failure
- [x] 9.3 Add a `README`/usage note documenting build, run, and the default config path

## 10. Verification

- [x] 10.1 Run `make build test lint` clean on a fresh checkout
- [x] 10.2 Manually verify CLI and MCP paths return semantically equivalent results for `findTool` against the empty catalog (adapter parity)
- [x] 10.3 Run `openspec validate init-project-skeleton` and confirm the change satisfies its specs
