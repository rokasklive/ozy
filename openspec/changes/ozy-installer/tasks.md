## 1. Foundations (interfaces + shared seams)

- [x] 1.1 Add `internal/paths` (the `install-paths` capability): resolve config, data, cache, state/logs, venv, asset, and user-bin locations per-OS, reusing `config.Home`/`config.DefaultPath` and the sidecar venv logic; honor `OZY_CONFIG` and XDG overrides
- [x] 1.2 Add PATH-reachability detection to `internal/paths` (is the resolved binary on `PATH`; which bin dir to add) with no mutation
- [ ] 1.3 Scaffold `internal/installer` with the engine interfaces: `Platform`, `Paths`, `Runner`, `Downloader`, `DepChecker`, `PathManager`, `ConfigManager`, `StateStore`, `Logger`, `Progress`, `Planner`, `UninstallPlanner`, `Prompter` (deliberately skipped — YAGNI; concrete types + minimal `runner`/`lookPath`/`valid` seams are introduced only where a second consumer/test actually needs them)
- [x] 1.4 Implement `StateStore` (read/write JSON state with `version`/`schemaVersion`/per-step result; atomic save, fresh-on-mismatch resume)
- [x] 1.5 Implement `Logger` (durable timestamped file log + friendly terminal stream, satisfies `sidecar.Logger`) and a credential-pattern redactor; never log values/secrets/config contents
- [x] 1.6 Implement `Prompter` consent model: ordinary vs risky classification; `--yes` accepts ordinary only; non-interactive risky → skip with printed manual instruction (pure policy + `Confirm` IO layer)

## 2. Bootstrap entrypoint + modes

- [x] 2.1 Create `cmd/ozy-install/main.go` that parses flags and calls `internal/installer`; verify `go run ./cmd/ozy-install` runs end-to-end locally
- [x] 2.2 Implement flags: `--dry-run`/`--plan`, `--yes`, `--manual`, `--verbose`, `--no-color`; ensure `--dry-run`/`--plan` short-circuit before any mutation
- [x] 2.3 Implement intro + dry-run-first flow: inspect → print plan → "Nothing has changed yet." → `Proceed? [Y/n]`; decline exits cleanly with zero changes
- [x] 2.4 Implement manual/guided mode: per-step what/why/verify/next-command checklist

## 3. Detection, plan, dependency report

- [x] 3.1 Implement `Platform` detection (OS/arch) and terminal-capability detection (TTY, width, color)
- [x] 3.2 Implement `DepChecker` for Go toolchain, Git, Python 3.11+, SQLite, semantic backend — each reporting name, required/optional, detected vs required version, status, why-needed, can-Ozy-manage, fallback
- [x] 3.3 Implement `CheckExistingInstall` (detect existing binary + config; mark run as update)
- [x] 3.4 Implement `Planner` producing the full install plan (locations, version, planned actions, downloads, est. disk usage, PATH/config changes, existing-install/config, compatibility warnings)
- [x] 3.5 Render the dependency table and install plan in human-readable form (and plain non-TTY form)

## 4. Install state machine + steps

- [x] 4.1 Implement the step runner: ordered idempotent steps, persisted status, resume from last safe point, revalidate completed-but-stale step output
- [x] 4.2 Implement `CreateInstallRoot` (idempotent `MkdirAll` of resolved dirs)
- [x] 4.3 Implement `InstallOzyBinary` (build-from-source via the user's Go toolchain into the resolved bin path; re-install if missing/older) behind `Downloader`/`Runner` so a release-asset path can be added later
- [x] 4.4 Implement `CreateOrUpdateConfig` via `ConfigManager`: create default-on-consistent `ozy.jsonc` if missing (reuse `config.WriteStarter`); else validate, backup before edit, minimal safe change, preserve comments, report changes
- [x] 4.5 Implement `SetupPythonEnvironment` + `DownloadEmbeddingAssets` by calling `sidecar.Provision` (consent-gated); degrade to lexical-only on `ErrNoToolchain` without failing the install
- [x] 4.6 Implement `BuildOrLoadToolCatalog` (initialize/load catalog; rebuild if missing)
- [x] 4.7 Implement `RunDoctor` step feeding the status block; surface doctor failures rather than hiding them
- [x] 4.8 Implement `PrintNextSteps` success summary + MCP-harness handoff using resolved paths (config/data/log paths, `ozy doctor`, `ozy mcp`, `command: ozy`/`args:["mcp"]`, downstream-defs location, log path)

## 5. PATH configuration

- [x] 5.1 Implement `PathManager` detection-first flow: no-op when reachable
- [x] 5.2 Implement consent-based Unix rc/profile edit (detect shell; append a clearly marked, idempotent block) and Windows user-level PATH edit (never machine-level)
- [x] 5.3 Implement copy-paste fallback instructions when automatic edit is declined/unsafe

## 6. Progress UI + errors

- [x] 6.1 Implement `Progress` persistent dashboard: all steps visible from start, completed stay 100%, not-started 0%, running shows real bar (known total) or spinner (unknown)
- [x] 6.2 Implement failure expansion (keep failed/partial bar; expand actionable error beneath) and plan-change regeneration before execution
- [x] 6.3 Implement non-TTY / CI / `--no-color` plain static fallback preserving step/percentage semantics
- [x] 6.4 Implement actionable error format for every failed step (what/why/impact/safe-to-retry/next-command/log-path); print log path every run and retry command on failure

## 7. Uninstall flow

- [x] 7.1 Implement `UninstallPlanner` + detection of all Ozy-managed locations, categorized (binary, managed-runtime, cache, data/models, config, mcp-defs, path-block) — mcp-defs live in the config file, so they share the config category
- [x] 7.2 Implement uninstall state machine (DetectInstall → PlanRemovals → RemoveBinary → RemoveManagedData → CleanPath → WriteSummary), idempotent removals safe to rerun (idempotency via `os.RemoveAll`; no persisted state file — it would live in the tree being deleted)
- [x] 7.3 Implement conservative default scope + per-category consent; preserve user config and downstream MCP definitions unless explicitly confirmed
- [x] 7.4 Implement `--dry-run`, `--keep-config`, `--keep-data`, and `--purge` (purge requires distinct explicit confirmation; `--yes` alone never purges)
- [x] 7.5 Implement consent-based PATH/rc cleanup that removes only the installer-added marked block
- [x] 7.6 Wire `go run …/cmd/ozy-install uninstall` to the same flow

## 8. CLI integration

- [x] 8.1 Add `ozy uninstall` subcommand in `internal/cli` (wired through the app/render plumbing) delegating to `internal/installer` uninstall, with the same flags
- [x] 8.2 Update CLI command-surface docs to reference `go run …/cmd/ozy-install@<version>` and `ozy uninstall`

## 9. Tests (fakes — no real installs)

- [x] 9.1 Plan generation + OS/path resolution (table-driven across Linux/macOS/Windows fakes)
- [x] 9.2 Step state persistence; resume after failed step; retry after interrupted run; stale-output revalidation
- [x] 9.3 Dependency detection + dependency-table rendering; error-message quality assertions
- [x] 9.4 Consent boundaries (download / PATH / shell-edit / dep-install blocked without consent; `--yes` does not bypass risky)
- [x] 9.5 Dry-run and default dry-run-first behavior make zero mutations
- [x] 9.6 Existing-config preservation (no clobber; backup before edit)
- [x] 9.7 PATH detection + PATH cleanup (marked block add/remove, idempotent)
- [x] 9.8 Progress UI non-TTY fallback; failure expansion does not hide steps
- [x] 9.9 Durable log creation + log redaction (no secrets/values/config contents)
- [x] 9.10 Uninstall: dry-run, conservative removal, purge, interrupted-rerun, config/MCP-defs preservation

## 10. Cross-platform + CI

- [ ] 10.1 Make Linux the fully working path; verify the full install + uninstall on a Linux container — Linux uses the implemented unix code path (XDG dirs, bash/zsh rc) and is exercised by the CI `installer (ubuntu-latest)` job; a *real* end-to-end `go install`+sidecar run is gated on a published module tag (post-merge)
- [ ] 10.2 Verify macOS path (XDG-consistent locations, PATH/rc edits) on a native runner — flow verified locally on darwin/arm64 (dry-run, plan, manual, decline, uninstall; XDG locations + rc/PATH logic) and via the CI `installer (macos-latest)` job; real `go install` likewise gated on a published tag
- [x] 10.3 Provide structured, tested Windows stubs (paths, user PATH, shell behavior) behind the same interfaces
- [x] 10.4 Add CI: Linux container job for the state machine/dependency-mocks/non-TTY/dry-run/retry; native macOS/Windows jobs for PATH, rc edits, and filesystem/permission behavior

## 11. Reconciliation + docs

- [x] 11.1 Reconcile `config.WriteStarter` semantic default with semantic-on-by-default so `ozy init` and the installer agree
- [x] 11.2 Update README/CONTRIBUTING install sections to lead with the one-line bootstrap and document the `go install` secondary path
- [x] 11.3 Run `graphify update .` after implementation to refresh the knowledge graph
