# installer

## Purpose

The Ozy installer — a separate Go bootstrap binary at `cmd/ozy-install` — that
sets up a working `ozy` on a fresh machine: inspects the platform, produces a
plan the user can read and decline, provisions the binary, config, sidecar
runtime, embedding assets, and `PATH` reachability, runs `ozy doctor` as the
verification step, and hands the user off to the MCP integration. It is
dry-run-first, consent-based, resumable, durable-logged, and degrades
gracefully (e.g. lexical-only) when optional dependencies are unavailable.

## Requirements

### Requirement: One-command bootstrap entrypoint

Ozy SHALL provide a Go bootstrap binary at `cmd/ozy-install` runnable as
`go run github.com/rokasklive/ozy/cmd/ozy-install@<version>`. The documented
`go run` import path MUST match the module path in `go.mod`. The binary SHALL
also be installable via `go install …/cmd/ozy-install@<version>` and then run as
`ozy-install`. Running `go install` alone MUST NOT perform setup; setup begins
only when the binary is executed.

#### Scenario: Bootstrap runs setup

- **WHEN** the user runs `go run …/cmd/ozy-install@<version>`
- **THEN** the binary builds and executes, printing the intro and entering the
  dry-run-first flow

#### Scenario: go install does not setup

- **WHEN** the user runs `go install …/cmd/ozy-install@<version>`
- **THEN** the `ozy-install` binary is placed on the Go bin path and nothing on
  the system is otherwise modified until the user runs `ozy-install`

### Requirement: Dry-run-first default behavior

With no mutating flag, the installer SHALL inspect the system, print the install
plan and dependency report, state that nothing has changed yet, and prompt
`Proceed with this installation? [Y/n]` before any mutation. No filesystem,
PATH, config, or download mutation SHALL occur before the user confirms.

#### Scenario: Plan shown before any change

- **WHEN** the bare installer is run in a TTY
- **THEN** it prints the plan, prints "Nothing has changed yet.", and waits for
  confirmation, having made no changes

#### Scenario: Decline leaves system untouched

- **WHEN** the user answers `n` at the confirmation prompt
- **THEN** the installer exits without creating directories, writing config,
  downloading, or editing PATH

### Requirement: Installation modes and flags

The installer SHALL support auto mode (default), manual/guided mode (`--manual`),
and plan-only mode (`--dry-run` / `--plan`). It SHALL support `--yes`,
`--verbose`, and `--no-color`. `--dry-run` and `--plan` MUST NOT modify the
system under any circumstances. `--yes` MAY skip ordinary confirmations but MUST
NOT silently perform risky actions (deleting config, editing shell profiles,
system-level changes, or destructive operations).

#### Scenario: Plan flag never mutates

- **WHEN** the installer runs with `--dry-run` or `--plan`
- **THEN** it prints the full plan and exits with no mutations, even if
  dependencies are missing

#### Scenario: Yes skips ordinary prompts only

- **WHEN** the installer runs with `--yes`
- **THEN** ordinary confirmations are auto-accepted, but shell-profile edits and
  other risky actions still require an explicit prompt or are skipped with a
  printed manual instruction

#### Scenario: Manual mode is a guided checklist

- **WHEN** the installer runs with `--manual`
- **THEN** each step shows what to do, why it matters, how to verify it, and the
  next command — without dumping the user into external docs

### Requirement: Install plan transparency

Before installation the installer SHALL display a plan including detected OS and
architecture, Ozy version, install root, and the binary/config/data/cache/log
paths; the Python venv and vector/model/asset locations; required, optional, and
missing dependencies; which dependencies Ozy can install automatically vs.
manually; planned network downloads and estimated disk usage where practical;
planned PATH/shell changes; any existing Ozy install or config detected; and any
compatibility warnings.

#### Scenario: Plan enumerates locations and actions

- **WHEN** the plan is rendered
- **THEN** it lists every resolved location and every planned action, grouped
  and human-readable, ending with "Nothing has changed yet."

#### Scenario: Existing install detected

- **WHEN** an existing Ozy binary or config is present
- **THEN** the plan reports it and describes the install as an update rather than
  a fresh install

### Requirement: Dependency report

The installer SHALL print a dependency table before installation. For each
dependency it SHALL report name, required-or-optional, detected version,
required version, status, why it is needed, whether Ozy can install/manage it,
and the fallback behavior if missing. Dependency setup SHALL prefer user-space,
project-managed dependencies and MUST NOT require sudo/admin rights or install
Python packages globally.

#### Scenario: Required dependency missing

- **WHEN** Python 3.11+ is not found
- **THEN** the table marks it missing, explains it is needed for the semantic
  backend, states Ozy can provision a managed venv where supported, and names
  lexical-only as the fallback

#### Scenario: Optional dependency absent

- **WHEN** the semantic backend is unavailable
- **THEN** the table marks it optional and states Ozy will run lexical-only

### Requirement: Consent boundaries

Even in auto mode and even with `--yes`, the installer SHALL obtain explicit
consent before downloads, PATH changes, shell rc/profile edits, dependency
installs, creating managed runtimes, or any change outside the Ozy-managed
directory.

#### Scenario: Download requires consent

- **WHEN** the installer is about to download an asset or dependency
- **THEN** it asks for consent first, and skips the download (continuing where
  possible) if consent is denied

#### Scenario: Shell edit requires consent under --yes

- **WHEN** the installer runs with `--yes` and would edit a shell profile
- **THEN** it still prompts for that specific edit or prints manual instructions
  instead of editing silently

### Requirement: Resumable step state machine

Installation SHALL be modeled as discrete, individually idempotent steps
(DetectPlatform, ResolveInstallDirs, CheckExistingInstall, dependency checks,
CreateInstallRoot, InstallOzyBinary, CreateOrUpdateConfig, SetupPythonEnvironment,
DownloadEmbeddingAssets, BuildOrLoadToolCatalog, ConfigurePath, RunDoctor,
PrintNextSteps). Installer state SHALL persist to a small JSON file under the Ozy
state area, recording each completed step with enough metadata to validate its
output still exists and matches the requested version. A rerun SHALL skip
completed-and-valid steps and continue from the last safe point.

#### Scenario: Resume after failure

- **WHEN** a step fails and the installer is rerun
- **THEN** prior completed steps are detected as done and skipped, and execution
  resumes at the failed step

#### Scenario: Resume after interruption

- **WHEN** the installer process is interrupted mid-run and rerun
- **THEN** no completed step is repeated destructively and the run continues from
  the last recorded safe point

#### Scenario: Stale step output revalidated

- **WHEN** a step was recorded complete but its output is missing or version-
  mismatched on rerun
- **THEN** the step is treated as not-done and re-executed

### Requirement: Config create-or-update without clobbering

The installer SHALL create a default `ozy.jsonc` if none exists (reusing the
existing scaffold) with defaults consistent with Ozy's semantic-on-by-default
behavior. If a config already exists it MUST NOT be overwritten silently;
instead the installer SHALL validate it, back it up before any modification,
apply only minimal safe changes, preserve comments where practical, and report
what changed. The final output SHALL show the config path.

#### Scenario: Fresh config created

- **WHEN** no `ozy.jsonc` exists at the resolved path
- **THEN** a default config is written and its path is reported

#### Scenario: Existing config preserved

- **WHEN** a valid `ozy.jsonc` already exists
- **THEN** it is not overwritten; if any change is needed a timestamped backup is
  written first and the change is reported

### Requirement: Python and asset provisioning with graceful degradation

The SetupPythonEnvironment and DownloadEmbeddingAssets steps SHALL reuse the
existing sidecar provisioner (uv → python3 managed venv, pinned dependencies,
marker-cached) under a Ozy-managed directory. When no usable Python toolchain
exists, the installer SHALL NOT fail the install; it SHALL continue in
lexical-only mode and report that mode clearly.

#### Scenario: Toolchain present

- **WHEN** a usable Python toolchain is available and the user consents
- **THEN** the managed venv is provisioned via the existing provisioner and
  semantic mode is reported available

#### Scenario: Toolchain absent degrades, not fails

- **WHEN** no Python toolchain is available
- **THEN** the step completes in lexical-only mode, the install still succeeds,
  and `ozy doctor` reports lexical-only

### Requirement: Consent-based PATH configuration

The ConfigurePath step SHALL first detect whether the installed binary is
reachable on `PATH`. If not, it SHALL offer to add the user bin directory, on
Unix detecting the shell and appending a clearly marked block to the relevant
rc/profile only with consent, and on Windows modifying user-level PATH only. If
automatic modification is unsafe or unsupported, it SHALL print exact
copy-paste instructions instead.

#### Scenario: Already reachable

- **WHEN** the binary is already on `PATH`
- **THEN** no PATH change is offered or made

#### Scenario: Consent-based rc edit

- **WHEN** the binary is not on `PATH` and the user consents
- **THEN** a clearly marked block is appended to the detected shell's rc/profile
  file and the change is reported

#### Scenario: Fallback to instructions

- **WHEN** the binary is not on `PATH` and automatic edit is declined or unsafe
- **THEN** exact copy-paste instructions for adding the bin directory are printed

### Requirement: Persistent progress dashboard

During execution the installer SHALL render a dashboard where every planned step
is visible from the start, completed steps remain at 100%, not-started steps
remain at 0%, and the running step shows honest progress — a real bar when total
work is measurable (downloads, extraction, asset/package install) or an
indeterminate spinner when it is not. The installer MUST NOT show a percentage it
cannot substantiate. A failed step SHALL keep its partial/failed bar visible and
expand an actionable error beneath it. The dashboard SHALL reflect the confirmed
plan and be regenerated if the plan changes before execution.

#### Scenario: Steps stay visible

- **WHEN** a step completes
- **THEN** its full 100% bar remains visible while later steps remain at 0%

#### Scenario: Failure expands inline

- **WHEN** a step fails
- **THEN** its failed-state bar stays visible and a descriptive error expands
  beneath it without hiding any step

#### Scenario: Non-TTY fallback

- **WHEN** output is not a TTY, is in CI, or `--no-color` is set
- **THEN** the dashboard degrades to plain static lines that preserve the same
  step/percentage semantics

### Requirement: Durable redacted logging

Every install run SHALL create a durable log file recording the run timestamp,
installer version, OS/arch, resolved paths, each step start/end/failure,
commands executed, downloaded URLs/asset ids and checksums where applicable,
dependency detection results, config mutations, and PATH/shell changes, ending
with the final status. The log MUST NOT contain secrets, tokens, environment
variable values, private config contents, or credentials. Every run SHALL print
the log path at the end; on failure it SHALL also print the safe-retry command.

#### Scenario: Log written every run

- **WHEN** any install run finishes (success or failure)
- **THEN** a log file exists with the recorded fields and its path is printed

#### Scenario: Secrets redacted

- **WHEN** the run touches config or environment that contains secret-bearing
  values
- **THEN** those values are absent from the log (names may appear, values never)

#### Scenario: Failure prints retry

- **WHEN** a run fails
- **THEN** the output prints both the log path and the exact command to safely
  retry

### Requirement: Actionable errors

Every failed step SHALL produce an error stating what failed, what Ozy was
trying to do, why the step matters, whether retry is safe, the next command to
run, and the log path. Bare messages like "install failed" are not acceptable.

#### Scenario: Failure is descriptive

- **WHEN** a step fails
- **THEN** the error names the failed step and includes the cause, the impact,
  the safe-to-retry flag, the next command, and the log path

### Requirement: Success summary and MCP handoff

On success the installer SHALL print a summary listing installed binary, config,
data, and log paths; a status block (binary on PATH, config valid, MCP adapter
ready, semantic vs lexical mode); next commands (`ozy doctor`, `ozy mcp`); the
MCP-harness integration block (`command: ozy`, `args: ["mcp"]`); where to add
downstream MCP server definitions; and the log path. RunDoctor SHALL execute as
the verification step before the summary. Paths in the summary MUST be the
platform-resolved paths, not hardcoded examples.

#### Scenario: Summary and handoff printed

- **WHEN** installation completes successfully
- **THEN** the summary, doctor-verified status, next commands, MCP integration
  block, config location for downstream servers, and log path are printed using
  resolved paths

#### Scenario: Doctor run before summary

- **WHEN** the RunDoctor step executes
- **THEN** its result feeds the status block and a doctor failure is surfaced
  rather than hidden by the success summary
