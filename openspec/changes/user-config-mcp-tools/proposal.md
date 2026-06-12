## Why

Ozy is meant to be the one durable place where users keep MCP configuration, but
the current discovery order still treats project-local config as the primary
default. Moving the default configuration home to the OS user config directory
and validating a real MCP config fixture makes Ozy usable as a global tool
broker rather than only a repo-local prototype.

## What Changes

- Make the user config directory the default Ozy configuration home: on Unix-like
  systems `$XDG_CONFIG_HOME/ozy` or `~/.config/ozy`, and on Windows the
  comparable roaming user config directory (for example `%AppData%\ozy`).
- Store Ozy configuration files under that directory moving forward, with
  `ozy.jsonc` as the default file and `ozy init` creating the parent directory
  and starter config there unless `--config`/`OZY_CONFIG` overrides it.
- Keep explicit config path overrides for project-local or test configs, but do
  not silently prefer `./ozy.jsonc` or `./ozy.json` over the user config home.
- Support full opencode compatibility for the top-level `mcp` section only, so
  MCP snippets from project docs can be copied into Ozy config without reshaping:
  local server fields (`type`, `command`, `cwd`, `environment`, `enabled`,
  `timeout`) and remote server fields (`type`, `url`, `headers`, `oauth`,
  `enabled`, `timeout`).
- Treat `examples/test_mcp_examples.jsonc` as a real compatibility fixture for
  opencode-shaped local MCP server config, including command arrays, enabled
  flags, environment maps, timeouts, comments, and trailing commas.
- Require the CLI path to load that fixture with `--config`, index the configured
  MCP servers, and expose discovered tools through `ozy list`, `ozy describe`,
  and the broker-backed search flow.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `configuration`: default discovery, initialization, and compatibility parsing
  change to make the OS user config directory the moving-forward config home
  while preserving explicit overrides and full JSONC/opencode `mcp` section
  compatibility.
- `cli-interface`: broker-backed CLI commands must prove that tools discovered
  from a real config file are visible and inspectable through the CLI.

## Impact

- Changed code: `internal/config/` default path resolution, init/scaffold
  behavior, JSONC compatibility coverage for the opencode `mcp` section and
  example fixture, and docs.
- Changed code: `internal/cli/` tests or command wiring where needed to prove
  `--config examples/test_mcp_examples.jsonc` can drive `index`, `list`,
  `describe`, and `search` against discovered tools.
- Affected user contract: users now keep Ozy configuration in the user config
  home by default instead of relying on repo-local discovery.
- Affected tests/evals: add cross-platform path tests and an end-to-end CLI
  fixture test using the real example config shape with mocked or controlled MCP
  servers so acceptance does not depend on private network services.
