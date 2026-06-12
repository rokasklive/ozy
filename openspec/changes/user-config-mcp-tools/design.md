## Context

`downstream-discovery` moved Ozy toward JSONC config and real MCP tool
discovery, but the default path still allows repo-local files to win before the
user config directory. That keeps Ozy feeling project-scoped even though the
product mission is one durable user-level MCP registry. The checked-in
`examples/test_mcp_examples.jsonc` also captures the kind of opencode-shaped
local MCP config Ozy must accept in practice: JSONC comments and trailing
commas, command arrays, environment maps, enabled flags, and a server timeout.
The broader target is full compatibility with opencode's top-level `mcp`
section, because MCP server README snippets commonly document copy-paste config
for specific agents and opencode's shape is the least opinionated target for
Ozy's use case.

This change makes the user config home the default source of truth and adds a
CLI-level acceptance path for using a real config file to index and inspect
downstream tools.

## Goals / Non-Goals

**Goals:**

- Default to the OS user config home for Ozy config:
  `$XDG_CONFIG_HOME/ozy` or `~/.config/ozy` on Unix-like systems, and the
  roaming user config equivalent such as `%AppData%\ozy` on Windows.
- Make `ozy init` write `ozy.jsonc` into that config home by default.
- Preserve explicit overrides through `--config` and `OZY_CONFIG` for
  project-local configs, tests, and one-off runs.
- Support full opencode compatibility for the `mcp` section only, preserving
  local server fields (`type`, `command`, `cwd`, `environment`, `enabled`,
  `timeout`) and remote server fields (`type`, `url`, `headers`, `oauth`,
  `enabled`, `timeout`).
- Load `examples/test_mcp_examples.jsonc` without syntax or schema errors and
  preserve the fields needed to launch enabled local MCP servers.
- Prove the CLI can use a real config path to run `index`, then expose the
  discovered catalog through `list`, `describe`, and broker-backed search.

**Non-Goals:**

- Importing configs from opencode automatically.
- Supporting unrelated top-level opencode config sections such as providers,
  agents, tools, themes, permissions, or keybinds.
- Implementing remote OAuth browser/token flows in this change; OAuth config is
  parsed and preserved, but authenticated runtime flow can be proposed later.
- Moving catalog state into the config directory; catalog data remains runtime
  state and should stay under the state path.
- Implementing brokered `callTool`; this change only requires discovery and CLI
  inspection of tools.
- Making private or network-dependent example servers mandatory in normal CI.

## Decisions

### D1: Make config-home resolution explicit in `internal/config`

Add a small path API around the existing default path logic:

- `ConfigHome()` returns the Ozy config directory.
- `DefaultPath()` returns `ConfigHome()/ozy.jsonc` unless `OZY_CONFIG` is set.
- The CLI `--config` default uses `DefaultPath()`.

On Unix-like systems, `ConfigHome()` uses `$XDG_CONFIG_HOME/ozy` when set and
`$HOME/.config/ozy` otherwise. On Windows, it uses `os.UserConfigDir()` and
appends `ozy`, yielding the roaming user config location in normal Go builds.

Alternative: continue checking `./ozy.jsonc` before the user config home. That
keeps old developer convenience but contradicts the desired single global MCP
configuration. Project-local files remain supported by `--config ./ozy.jsonc` or
`OZY_CONFIG=./ozy.jsonc`.

### D2: `ozy init` writes only to the resolved config path

`ozy init` should create the parent config directory with owner-private
permissions and write the starter `ozy.jsonc` there unless an explicit config
path was supplied. It should still refuse to overwrite an existing file.

Alternative: add a separate `--global` or `--local` mode. That adds CLI surface
before there is a real workflow split; explicit `--config` already covers local
or test configs.

### D3: Match opencode `mcp` section shape, not full opencode config

The config model should accept opencode-compatible `mcp` entries so users can
copy MCP snippets from server documentation into Ozy with minimal editing. The
supported `mcp` section fields are:

- `mcp.<server>.type`
- `mcp.<server>.command`
- `mcp.<server>.cwd`
- `mcp.<server>.enabled`
- `mcp.<server>.environment`
- `mcp.<server>.url`
- `mcp.<server>.headers`
- `mcp.<server>.oauth`
- `mcp.<server>.timeout`

The `timeout` value is milliseconds and should be applied as one total
per-server discovery budget covering connect/initialize and `tools/list`, with a
default of 5000ms when omitted. `enabled` should be modeled as a tri-state:
omitted means enabled, `true` means enabled, and `false` means disabled. This is
important because opencode examples often omit `enabled`.

Remote `oauth` config should be parsed and preserved as either an object or
`false`. Runtime OAuth is not part of this change. If a remote server requires
OAuth and Ozy cannot authenticate yet, Ozy should surface a structured
auth-unavailable result instead of rejecting the config file.

The exact `examples/test_mcp_examples.jsonc` file remains a compatibility
fixture for local server snippets. Unknown forward fields inside each MCP entry
can be preserved or ignored if they do not affect launch semantics, but fields
documented by opencode for the `mcp` section should be modeled deliberately.

Alternative: support only the fields in the current fixture. That is too narrow:
it would parse today's local examples but fail the real copy-paste goal for
common remote snippets with `headers`, `oauth`, or `cwd`.

### D4: Add deterministic CLI acceptance coverage plus opt-in real-server check

Normal tests should not require private paths, `npx`, network access, or home
services. Use a deterministic CLI/integration seam that exercises the same
config loading and broker/catalog path while controlling the MCP server side
with a local test MCP server or fake connector. Separately, document an opt-in
manual or env-gated integration command that runs against
`examples/test_mcp_examples.jsonc` when the user's real servers are available.

Alternative: make CI run the literal example commands. That would be brittle and
environment-specific, and failures would not distinguish Ozy bugs from missing
private services.

## Risks / Trade-offs

- Existing developer workflows relying on implicit `./ozy.jsonc` discovery break
  by default -> keep explicit `--config ./ozy.jsonc` and document the migration.
- Platform config directories can surprise users -> tests pin Unix/XDG and
  Windows-style behavior, and `ozy doctor`/errors print the resolved path.
- The real example fixture references private commands/services -> automated
  coverage uses controlled MCP servers; the literal fixture remains an opt-in
  acceptance check for the developer environment.
- Accepting opencode-shaped `mcp` fields creates compatibility pressure ->
  isolate the parsing model in `internal/config` and explicitly scope parity to
  the `mcp` section only.
- OAuth config can imply runtime behavior that Ozy does not implement yet ->
  parse and preserve it, then return structured auth-unavailable diagnostics
  when runtime authentication is required.

## Migration Plan

Greenfield migration. Users with repo-local configs can either move the file to
the new default location:

```bash
mkdir -p ~/.config/ozy
mv ./ozy.jsonc ~/.config/ozy/ozy.jsonc
```

or keep using the local file explicitly:

```bash
ozy --config ./ozy.jsonc index
```

Rollback is limited to restoring the old default-path precedence. No catalog
migration is required because catalog state remains under the state path.

## Open Questions

None for this change. Keep the file name `ozy.jsonc`, scope opencode parity to
the `mcp` section only, and treat `timeout` as one total per-server discovery
budget.
