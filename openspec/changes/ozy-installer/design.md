## Context

Ozy already has the pieces an installer needs, just not assembled for a
non-contributor: `internal/config` resolves the config home and `ozy.jsonc`
path and scaffolds a starter file (`config.Home`, `config.DefaultPath`,
`config.WriteStarter`); `internal/sidecar` provisions a managed Python venv
(uv → python3), installs pinned embedding deps, caches via a marker file, and
soft-fails with `ErrNoToolchain`; `ozy doctor` already reports config validity,
catalog, server health, and semantic-vs-lexical mode. The gap is the front
door: there is no single command that inspects the machine, shows a plan, gets
consent, and drives these pieces to a working install — or removes them safely.

This change adds that front door as a Go bootstrap (`cmd/ozy-install`) plus an
`internal/installer` package, reusing the existing packages rather than
duplicating their logic. The module path is `github.com/rokasklive/ozy`
(Go 1.26), so the documented `go run github.com/rokasklive/ozy/cmd/ozy-install`
path is correct as-is.

## Goals / Non-Goals

**Goals:**
- One-command, dry-run-first bootstrap that is boring, transparent, resumable,
  and safe to rerun.
- A resumable step state machine with durable JSON state; each step idempotent
  and individually retryable.
- A single platform-paths abstraction reused by install and uninstall.
- Consent before every download, dependency install, PATH/shell edit, managed
  runtime, or change outside the Ozy-managed directory — even under `--yes`.
- Graceful degradation to lexical-only when Python is absent (install still
  succeeds).
- First-class, equally careful uninstall (conservative default, explicit purge).
- Durable redacted logging and a persistent honest progress dashboard, both with
  non-TTY fallback.
- Linux + macOS working well; Windows structured and tested via fakes/stubs.

**Non-Goals:**
- Installing a system Python, editing machine-level PATH, or any sudo/admin path.
- A GUI or TUI beyond the line-based dashboard.
- Splitting `ozy-install` into its own repo (revisit only if an independent
  release cadence is ever needed).
- Self-update / version channel management beyond installing a requested version.
- Re-implementing embedding provisioning — the sidecar provisioner owns that.

## Decisions

### Architecture: thin bootstrap over a fakeable engine
`cmd/ozy-install/main.go` parses flags and calls into `internal/installer`,
which holds the engine. The engine is built from small interfaces so the whole
flow is testable with fakes and no real installs:

- `Platform` — OS/arch + capability detection (reused/extended from runtime).
- `Paths` — the `install-paths` abstraction (see below).
- `Runner` — command execution (mirror of the sidecar's `Runner` seam).
- `Downloader` — fetch + checksum verify (stdlib `net/http`).
- `DepChecker` — dependency detection (Go/Git/Python/SQLite/semantic).
- `PathManager` — PATH detection + consent-based rc/profile + Windows user PATH.
- `ConfigManager` — wraps `config` (create/validate/backup/minimal-edit).
- `StateStore` — read/write the JSON state file.
- `Logger` — durable redacted file log + friendly terminal stream.
- `Progress` — the dashboard renderer (TTY + plain fallback).
- `Planner` / `UninstallPlanner` — build the plan from detection.
- `Prompter` — consent (auto-accept ordinary under `--yes`, never risky).

Reuse over rebuild: `ConfigManager` delegates to `internal/config`;
SetupPythonEnvironment/DownloadEmbeddingAssets call `sidecar.Provision`;
RunDoctor calls the existing doctor path. The dozen interfaces are the test
seams, not twelve new subsystems.

### Platform-paths: standardize on XDG on Unix (match existing code)
`internal/config.Home()` already uses XDG/`~/.config/ozy` on both Linux and
macOS, and the sidecar venv lives under `~/.local/state/ozy`. The paths package
follows that exact convention rather than introducing macOS `~/Library/...`,
because contradicting the running code would split where files live.

| Location | Linux/macOS | Windows |
|----------|-------------|---------|
| Config   | `$XDG_CONFIG_HOME/ozy` or `~/.config/ozy` | `%APPDATA%\Ozy` |
| Data/models | `$XDG_DATA_HOME/ozy` or `~/.local/share/ozy` | `%LOCALAPPDATA%\Ozy` |
| Cache    | `$XDG_CACHE_HOME/ozy` or `~/.cache/ozy` | `%LOCALAPPDATA%\Ozy\Cache` |
| State/logs/venv | `$XDG_STATE_HOME/ozy` or `~/.local/state/ozy` | `%LOCALAPPDATA%\Ozy\state` |
| User bin | `~/.local/bin` | `%LOCALAPPDATA%\Ozy\bin` |

Documented env overrides (`OZY_CONFIG`, XDG vars) win. *Alternative considered:*
native macOS `~/Library/Application Support/Ozy` — rejected to stay consistent
with current resolution; can be revisited as a deliberate, separate change.

### Install state model
A JSON file at `<state>/ozy/install-state.json` records per-step status. Steps
and their done-validation:

| Step | Done when | Idempotency / validation on rerun |
|------|-----------|-----------------------------------|
| DetectPlatform | OS/arch captured | pure; always recompute |
| ResolveInstallDirs | paths resolved | pure; recompute |
| CheckExistingInstall | existing binary/config recorded | recompute |
| CheckGo / CheckGit / CheckPython / CheckOptionalDependencies | detection recorded | recompute (cheap) |
| CreateInstallRoot | dirs exist | `MkdirAll` is idempotent |
| InstallOzyBinary | binary at path, version matches | re-install if missing/older |
| CreateOrUpdateConfig | valid config at path | reuse if valid; backup before edit |
| SetupPythonEnvironment | venv marker matches | sidecar marker cache (already idempotent) |
| DownloadEmbeddingAssets | assets present + checksum | re-download if missing/mismatch |
| BuildOrLoadToolCatalog | catalog present | rebuild if missing |
| ConfigurePath | reachable or instructions shown | re-detect; no duplicate rc block |
| RunDoctor | doctor ran | always re-run (verification) |
| PrintNextSteps | n/a | always print |

State records `version`, `schemaVersion`, timestamps, and per-step result so a
rerun skips completed-and-valid steps. Detection/verification steps recompute
freely (cheap, no mutation). Mutation steps validate their own output before
skipping.

### Uninstall state model
A separate `<state>/ozy/uninstall-state.json`. Steps: DetectInstall →
PlanRemovals → RemoveBinary → RemoveManagedData → CleanPath → WriteSummary.
Removals are keyed by category (binary, managed-runtime, cache, data/models,
config, mcp-defs, path-block) so `--keep-*` and conservative defaults drop
categories from the plan. Each removal is idempotent: a missing target is a
no-op success, making partial-uninstall reruns safe.

### Consent model
`Prompter` distinguishes *ordinary* (create managed dir, write fresh config) from
*risky* (download, dep install, shell-profile edit, deleting user config/data,
purge, anything outside the managed dir). `--yes` auto-accepts ordinary only;
risky always prompts or, when non-interactive, is skipped with a printed manual
instruction. `--dry-run`/`--plan` short-circuit before any mutation regardless of
flags.

### Progress + logging
`Progress` keeps a fixed slice of steps and repaints in place on a TTY (carriage
return / cursor up), or emits one static line per state transition for
non-TTY/CI/`--no-color`. Bars are real only when a total is known (download
bytes, package count); otherwise a spinner. `Logger` writes a structured file
log under `<state>/ozy/logs/` and a friendly stream to the terminal; a redactor
strips secret-shaped values and never logs config contents or env values (Ozy
already has `config/redact.go` to lean on). Every run prints the log path; failed
runs also print the retry command.

### Dependency for the dashboard
Prefer stdlib (ANSI + `golang.org/x/term` for width/TTY, already transitively
common). If a richer dashboard is wanted, isolate exactly one small lib behind
`Progress` so it never leaks into the engine. *Default: stdlib first.*

## Risks / Trade-offs

- **`go run @version` requires a published, buildable tag** → keep
  `cmd/ozy-install` dependency-light so the bootstrap compiles fast and rarely
  breaks; document a fallback (clone + `go run ./cmd/ozy-install`).
- **Binary acquisition for InstallOzyBinary** (build-from-source via the Go
  toolchain the user already has vs. download a release asset) → start with
  build-from-source (the user is running `go run`, so Go is present); add release
  downloads behind `Downloader` later.
- **Shell-profile edits are the riskiest mutation** → marked, fenced block;
  consent always; idempotent (no duplicate block); uninstall removes only that
  block; copy-paste fallback when unsure.
- **Cross-platform PATH/shell/permissions can't be fully unit-tested** → fakes
  cover the state machine and Linux; native macOS/Windows CI runners cover PATH,
  rc edits, and filesystem/permission behavior. Docker is not treated as
  sufficient for platform-specific behavior.
- **Progress repaint corrupting scrollback / dumb terminals** → strict non-TTY
  detection with plain fallback; never assume ANSI.
- **Config/scaffold default drift**: `config.WriteStarter` currently writes
  `semantic.enabled:false` while the loader defaults semantic ON → the installer
  must write a default-on-consistent config; reconcile the scaffold or have the
  installer supply its own template. Tracked as an open question.
- **Estimated disk usage is approximate** → label it an estimate; never block on
  it; avoid fake precision.

## Migration Plan

Purely additive: new `cmd/ozy-install`, new `internal/installer`, new paths
package, and an `ozy uninstall` command. No existing command changes behavior.
Rollback = remove the new entrypoints; nothing else depends on them. Ship Linux +
macOS first; Windows lands behind the same interfaces with native-runner tests.

## Open Questions

- InstallOzyBinary v1: build-from-source only, or also fetch a release asset?
  (Lean build-from-source first.)
- Reconcile the `WriteStarter` semantic default with semantic-on-by-default —
  fix the scaffold, or installer-specific template? (Lean: fix the scaffold so
  `ozy init` and the installer agree.)
- Do we want a single shared `install-state`/`uninstall-state` schema version
  field gating future migrations? (Lean: yes, cheap.)
