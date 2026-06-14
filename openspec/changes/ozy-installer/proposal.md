## Why

Ozy is an agent-facing MCP capability broker, but getting it running today is a
developer chore: clone, `go build`, guess a config location, run `ozy init`,
hope a Python toolchain exists for the embedding sidecar, and manually wire it
into an agent harness. That is fine for contributors and hostile to everyone
else. Ozy's first product boundary should be a single boring, transparent,
resumable command that takes a user from zero to "Ozy is ready to add to my
agent harness" — and an equally careful command to remove it. The installer is
where users decide whether to trust Ozy; it must feel safe, never surprise them,
and always be honest about what it changed.

## What Changes

- Add a **Go bootstrap binary** at `cmd/ozy-install`, runnable in one line with
  `go run github.com/rokasklive/ozy/cmd/ozy-install@<ver>` (and, secondarily,
  `go install` + `ozy-install`). No separate repo — it ships inside the main
  module and reuses Ozy's own packages.
- **Dry-run-first by default.** The bare command inspects the machine, prints a
  full install plan and dependency report, states "Nothing has changed yet," and
  asks `Proceed? [Y/n]` before mutating anything. Flags: `--dry-run` / `--plan`
  (never mutate), `--yes` (skip ordinary confirmations but never risky ones),
  `--manual` (guided checklist), `--verbose`, `--no-color`.
- **Consent boundaries are load-bearing.** Even in auto mode and even with
  `--yes`, the installer asks before downloads, PATH/shell-profile edits,
  dependency installs, creating managed runtimes, deleting config, or any change
  outside the Ozy-managed directory.
- **Resumable step state machine**, not a one-shot script. Steps
  (DetectPlatform → ResolveInstallDirs → CheckExistingInstall → dependency
  checks → CreateInstallRoot → InstallOzyBinary → CreateOrUpdateConfig →
  SetupPythonEnvironment → DownloadEmbeddingAssets → BuildOrLoadToolCatalog →
  ConfigurePath → RunDoctor → PrintNextSteps) persist to a small JSON state file
  and are individually idempotent, so a rerun continues from the last safe point.
- **Single platform-paths abstraction** for binary / config / data / cache /
  logs / venv / asset locations (XDG on Linux, OS conventions on macOS/Windows),
  reusing the resolution already in `internal/config` and `internal/sidecar`
  instead of hardcoding paths anywhere.
- **Dependency report** before work: required (Go toolchain context, Git,
  Python 3.11+) and optional (semantic backend, SQLite) with version, status,
  why it's needed, whether Ozy can manage it, and the fallback if missing.
  Python/embedding setup **reuses `internal/sidecar.Provision`** (uv → python3
  venv, pinned deps, marker-cached) and degrades to **lexical-only** when no
  toolchain exists — matching Ozy's semantic-on-by-default-with-fallback model.
- **Config handling that never clobbers.** Create a default `ozy.jsonc` if
  missing (reusing `config.WriteStarter`); if one exists, validate it, back it
  up before any change, apply only minimal safe edits, and report what changed.
- **PATH handling is detection-first and consent-based**, with detected-shell
  rc/profile editing only on approval and exact copy-paste fallback when
  automatic modification is unsafe.
- **Persistent progress dashboard**: every planned step visible from the start,
  completed steps stay at 100%, the running step shows honest progress (real
  bars for downloads/installs, spinner for unknown-duration checks), failures
  expand inline with an actionable error. Degrades to plain static lines for
  non-TTY / CI / `--no-color`.
- **Durable, redacted logging** for every install and uninstall run (timestamps,
  versions, paths, per-step results, commands, URLs/checksums, config/PATH
  mutations, final status), never logging secrets or config contents. Every run
  ends by printing the log path; failures also print the safe-retry command.
- **First-class uninstall**: `ozy uninstall` (and
  `go run …/cmd/ozy-install@<ver> uninstall` before Ozy is installed) with the
  same plan-first/consent/progress/logging discipline. Conservative by default
  (removes binary + Ozy-managed runtime files, asks before config / caches /
  models / vector stores / PATH edits / user MCP definitions); `--dry-run`,
  `--keep-config`, `--keep-data`, and an explicit-confirmation `--purge`.
- First version targets **Linux well and macOS solidly**, with structured,
  tested stubs for **Windows** (path rules, PATH, shell behavior).

## Capabilities

### New Capabilities
- `install-paths`: Platform-appropriate resolution of every Ozy location
  (binary, config, data, cache, logs, Python venv, vector/model assets, user
  bin dir) behind one abstraction, plus PATH-reachability detection. Reused by
  both the install and uninstall flows so locations are never hardcoded.
- `installer`: The `ozy-install` bootstrap — intro, install modes, dry-run-first
  planning, dependency report, consent boundaries, the resumable install state
  machine, config create-or-update, Python/asset provisioning via the existing
  sidecar provisioner, PATH configuration, progress dashboard, durable redacted
  logging, actionable error conventions, and the MCP-harness handoff/next-steps.
- `uninstaller`: The uninstall flow — discovery of Ozy-managed locations,
  plan-first removal, conservative-vs-`--purge` policy, per-category consent,
  preservation of user-authored config and downstream MCP definitions, the
  resumable uninstall state machine, and uninstall logging.

### Modified Capabilities
- `cli-interface`: Adds the `ozy uninstall` subcommand (with its modes/flags) to
  the main `ozy` binary; documents `ozy-install` as the separate bootstrap
  entrypoint alongside the existing commands.

## Impact

- **New code**: `cmd/ozy-install` (bootstrap + `uninstall` subcommand), a new
  `internal/installer` package (planner, step state machine, dependency checker,
  PATH manager, downloader, progress renderer, logger, state store — all behind
  interfaces for fakeable tests), and a shared `internal/paths` (or extension of
  `internal/config`) for platform locations.
- **Reused, not rebuilt**: `internal/config` (`Home`, `DefaultPath`,
  `WriteStarter`, validation, redaction), `internal/sidecar` (`Provision`,
  marker cache, `ErrNoToolchain` soft-fail), and `ozy doctor` (run as the final
  verification step). The installer must align config defaults with
  semantic-on-by-default rather than contradict it.
- **CLI**: `internal/cli` gains an `uninstall` command wired through the same
  app/render plumbing as existing commands.
- **Dependencies**: prefer stdlib; at most one small terminal-UI dependency for
  the progress dashboard, isolated behind the progress-renderer interface.
- **Docs**: README / CONTRIBUTING install sections point at the one-line
  bootstrap; `go.mod` module path (`github.com/rokasklive/ozy`) must match the
  documented `go run` path.
- **Testing/CI**: most flow tested with fakes (no real installs); Linux via
  containers; native macOS/Windows runners for PATH, shell-profile, and
  filesystem/permission behavior.
