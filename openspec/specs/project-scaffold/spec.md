# project-scaffold

## Purpose

Define the buildable foundation for Ozy: a single `ozy` binary, a standard Go
repository layout that maps to the architectural boundaries in `SPEC.md` §6, and
the build/test/lint tooling that gives every later change a working quality gate.

## Requirements

### Requirement: Single binary build

The project SHALL be a Go module that builds into a single executable named `ozy` via a documented build command.

#### Scenario: Building from a clean checkout

- **WHEN** a contributor runs the documented build command (e.g. `make build`) on a clean checkout
- **THEN** the build succeeds and produces a single `ozy` executable that prints its version when run with `--version`

#### Scenario: Binary exposes the command tree

- **WHEN** the built `ozy` binary is run with `--help`
- **THEN** it lists the subcommands defined by the CLI interface without error

### Requirement: Standard repository layout

The repository SHALL follow standard Go project layout, placing the command entrypoint under `cmd/ozy/` and non-public packages under `internal/`, so that architectural boundaries from `SPEC.md` §6 map to packages.

#### Scenario: Entry point and internal packages present

- **WHEN** the repository is inspected after this change
- **THEN** `cmd/ozy/main.go` exists as the sole entrypoint and broker/daemon/config/cli/mcp/catalog concerns live under separate `internal/` packages

### Requirement: Test and lint tooling

The project SHALL provide a working test target and a lint/vet target, and SHALL include at least one passing baseline test so future changes inherit a functioning quality gate.

#### Scenario: Tests run green

- **WHEN** a contributor runs the documented test command (e.g. `make test`)
- **THEN** `go test ./...` runs and the baseline test suite passes

#### Scenario: Continuous integration runs build, test, and lint

- **WHEN** a commit is pushed and CI executes
- **THEN** CI runs build, test, and lint/vet steps and fails if any step fails
