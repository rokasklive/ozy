## Why

Ozy currently exists only as `SPEC.md` — there is no buildable code, no module, and no place for subsequent OpenSpec changes to land implementation. Before any broker behavior (search, describe, call) can be built and evaluated, the project needs a compiling, runnable skeleton that establishes the architectural boundaries `SPEC.md` mandates: a single `ozy` binary, a daemon, an MCP adapter, a CLI mirror, configuration loading, and a catalog store seam. This change creates that skeleton so every later change has a stable frame to fill in.

## What Changes

- Initialize the Go module and standard repository layout (`cmd/ozy`, `internal/...`) producing a single `ozy` binary.
- Add build/test/lint tooling (`Makefile`, CI workflow, baseline `go test`) so future changes have a working quality gate.
- Wire the `ozy` CLI command surface from `SPEC.md` §15 (`init`, `daemon`, `mcp`, `index`, `doctor`, `list`, `search`, `describe`, `call`, `eval run`) with `--format` support for human / JSON / concise output. Commands whose behavior is future work return a structured, instructional `NOT_IMPLEMENTED` response rather than panicking.
- Define the configuration model from `SPEC.md` §11: file discovery, schema, `${ENV}` reference resolution, validation, and redaction for diagnostics.
- Stand up the daemon runtime lifecycle and an in-process broker seam (with a catalog store interface placeholder) shared by both the CLI and MCP adapter, satisfying the adapter-neutral-core principle (§4.9).
- Stand up the MCP adapter that registers the three stable agent-facing tools (`findTool`, `describeTool`, `callTool`) returning placeholder *instructional* responses that already conform to the §9 response shapes.
- Add a minimal `doctor` that reports config validity, missing env vars (redacted), and adapter/daemon readiness.

Non-goals (explicitly deferred to later changes): real lexical/semantic search, real downstream MCP connection and brokered invocation, the embedding/indexing worker, the eval harness, schema-drift detection, and ContextSpy integration. The skeleton only proves these seams exist and degrade gracefully.

## Capabilities

### New Capabilities
- `project-scaffold`: Go module, repository layout, build/test/lint tooling, and the single `ozy` binary entrypoint.
- `configuration`: configuration file discovery, schema, environment-reference resolution, validation, and redaction.
- `cli-interface`: the `ozy` subcommand surface, shared output formats (human / JSON / concise), and structured handling of not-yet-implemented operations.
- `daemon-runtime`: daemon process lifecycle, the in-process broker seam, and the catalog store interface placeholder shared by all adapters.
- `mcp-adapter`: the agent-facing MCP server registering `findTool`, `describeTool`, and `callTool` with instructional placeholder responses conforming to the §9 contracts.

### Modified Capabilities
<!-- None — this is the first change; openspec/specs/ is empty. -->

## Impact

- New code: `go.mod`, `cmd/ozy/`, `internal/cli/`, `internal/config/`, `internal/daemon/`, `internal/broker/`, `internal/catalog/`, `internal/mcp/`.
- New tooling: `Makefile`, `.github/workflows/ci.yml`, baseline unit tests.
- Dependencies introduced: a CLI framework, a YAML parser, and an MCP server library (selected in design).
- No existing behavior is changed (greenfield). Establishes contracts (§9 response shapes, §11 config, §15 CLI) that later changes must preserve or deliberately amend.
- Aligns with `SPEC.md` accepted architectural baseline (§20): Go runtime/daemon/adapters, lexical-first, semantic-optional, local-first.
