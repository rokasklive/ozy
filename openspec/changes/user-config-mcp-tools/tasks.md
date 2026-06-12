## 1. Config Home Resolution

- [x] 1.1 Add `internal/config` helpers for the Ozy config home and default config file path.
- [x] 1.2 Change `DefaultPath()` so only `OZY_CONFIG` overrides the user config home by default; remove implicit `./ozy.jsonc` and `./ozy.json` discovery.
- [x] 1.3 Implement Unix/XDG fallback behavior: `$XDG_CONFIG_HOME/ozy` when set, otherwise `$HOME/.config/ozy`.
- [x] 1.4 Implement Windows behavior using the OS user config directory plus `ozy`.
- [x] 1.5 Update config path tests for env override, XDG/default Unix path, Windows-style path behavior, and project-local files not being selected implicitly.

## 2. Init And Opencode MCP Compatibility

- [x] 2.1 Ensure `ozy init` writes `ozy.jsonc` under the default user config home and creates the parent directory with owner-private permissions.
- [x] 2.2 Keep `ozy init --config <path>` behavior for explicit local/test config creation and verify it refuses to overwrite existing files.
- [x] 2.3 Extend `config.ServerConfig` to model opencode-compatible local MCP fields: `type`, `command`, `cwd`, `environment`, `enabled`, and `timeout`.
- [x] 2.4 Extend `config.ServerConfig` to model opencode-compatible remote MCP fields: `type`, `url`, `headers`, `oauth`, `enabled`, and `timeout`.
- [x] 2.5 Implement tri-state `enabled` semantics: omitted and `true` mean enabled; `false` means disabled.
- [x] 2.6 Preserve `oauth` as either an object or `false` without implementing the OAuth browser/token flow in this change.
- [x] 2.7 Add config tests for local `cwd`, remote `headers`, remote `oauth` object, `oauth: false`, tri-state `enabled`, and default timeout.
- [x] 2.8 Add a config test that loads `examples/test_mcp_examples.jsonc` exactly and asserts the `searxng`, `javadoc`, and `opengrok` server entries parse with commands, environment maps, enabled flags, and timeout values preserved.

## 3. Downstream Timeout Handling

- [x] 3.1 Apply configured per-server timeout values as one total discovery budget covering connect/initialize and `tools/list`.
- [x] 3.2 Apply the opencode-compatible 5000ms timeout default when a server omits `timeout`.
- [x] 3.3 Add downstream/index tests proving timeout is honored and reported as a structured per-server failure when exceeded.
- [x] 3.4 Return a structured auth-unavailable diagnostic when OAuth is required at runtime but no OAuth flow is implemented.
- [x] 3.5 Apply configured `cwd` when launching local MCP server commands.
- [x] 3.6 Ensure timeout and auth-unavailable errors do not leak resolved environment values in messages or diagnostics.

## 4. CLI Tool Resolution From Config

- [x] 4.1 Add deterministic CLI acceptance coverage that loads an explicit opencode-shaped MCP config, indexes controlled test MCP tools, and verifies JSON summary counts.
- [x] 4.2 Verify `ozy --config <fixture> list --format json` returns discovered toolRefs with server ids and freshness after indexing.
- [x] 4.3 Verify `ozy --config <fixture> describe <toolRef> --format json` returns the discovered tool schema and status instead of `TOOL_NOT_FOUND`.
- [x] 4.4 Verify `ozy --config <fixture> search <query> --format json` uses the populated catalog rather than returning `catalog_empty`.
- [x] 4.5 Add an opt-in manual or env-gated integration check for running against `examples/test_mcp_examples.jsonc` when the real local MCP server commands are available.

## 5. Documentation And Verification

- [x] 5.1 Update README configuration docs to state the new default config-home path and explicit project-local override workflow.
- [x] 5.2 Update any generated starter config or example docs to mention `~/.config/ozy/ozy.jsonc` and the Windows user config equivalent.
- [x] 5.3 Run `gofmt` on changed Go files.
- [x] 5.4 Run `make test`.
- [x] 5.5 Run `openspec validate user-config-mcp-tools`.
- [x] 5.6 Run `graphify update .` after implementation changes.
