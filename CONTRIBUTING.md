# Contributing to Ozy

Thanks for your interest in Ozy. This document covers everything you need to
build, test, and develop Ozy locally. End-user documentation lives in
[README.md](README.md); the full product specification lives in
[SPEC.md](SPEC.md).

## Requirements

- **Go 1.26+** (see `go.mod`)
- **Python 3.10+** with `uv` (only required if you exercise semantic search /
  the embedding sidecar)
- **make**

## Getting the source

```bash
git clone https://github.com/rokasklive/ozy.git
cd ozy
make install-hooks   # one-time: enable the tracked pre-push hook
```

The pre-push hook runs `make lint` before every push; install it once per clone
to keep the CI signal local-first.

## Build, test, lint

The `Makefile` is the entry point for every development task. Run `make help`
for a quick reference of available targets.

| Target                  | Purpose                                                           |
| ----------------------- | ----------------------------------------------------------------- |
| `make build`            | Compile the `ozy` binary to `./ozy`                               |
| `make test`             | Run the full Go test suite                                        |
| `make lint`             | Run `golangci-lint` (vet, staticcheck, gosec, formatting)         |
| `make fmt`              | Apply `gofmt` / `goimports`                                       |
| `make tools`            | Install the pinned `golangci-lint` version                        |
| `make tidy`             | Ensure `go.mod` / `go.sum` are tidy                               |
| `make install-hooks`    | Wire up the tracked Git hooks in `.githooks`                      |
| `make check-real-mcp-examples` | Opt-in check against `examples/test_mcp_examples.jsonc`     |
| `make clean`            | Remove build artifacts                                            |

### Verifying real MCP examples

The committed example servers in `examples/test_mcp_examples.jsonc` are
exercised by an opt-in target. Make the example commands available on your
`$PATH` and run:

```bash
OZY_RUN_REAL_MCP_EXAMPLES=1 make check-real-mcp-examples
```

This runs `ozy index` and `ozy list` against the example set and is a good
end-to-end smoke test before opening a pull request.

## Project layout

```
.
├── cmd/ozy/           # `ozy` binary entry point
├── cmd/ozy-install/   # one-command install/uninstall bootstrap (runs before `ozy` exists)
├── internal/          # implementation packages (broker, catalog, CLI, installer, paths, MCP adapter, …)
├── sidecar/           # Python embedding sidecar (FastEmbed + turbovec / FAISS)
├── evals/             # committed evaluation corpus, scoreboard, and methodology
├── examples/          # starter ozy.jsonc and real MCP example configs
├── openspec/          # OpenSpec change proposals driving implementation
├── assets/            # static assets (mascot, docs images)
├── SPEC.md            # living product specification
├── README.md          # user-facing documentation
├── CONTRIBUTING.md    # this file
├── AGENTS.md          # agent-facing notes for coding agents working in the repo
├── CLAUDE.md          # Claude-specific notes
├── Makefile           # canonical build / test / lint entry points
└── .golangci.yml      # linter configuration
```

## Coding conventions

- **Style**: idiomatic Go, formatted with `gofmt` and `goimports`. Linting is
  enforced by `golangci-lint`; the pinned version lives in the `Makefile`.
- **Errors**: propagate with `%w`; use `errors.Is` / `errors.As` at boundaries.
  See `SPEC.md` for the error envelope shape.
- **Concurrency**: respect `context.Context` cancellation; never leak
  goroutines. Long-running operations must terminate on context done.
- **Logging**: structured (`slog`). Keep log lines machine-parseable.
- **Public surface**: any change to the three MCP tools (`findTool`,
  `describeTool`, `callTool`) is a contract change and must update `SPEC.md`
  in the same change.

## Commit and PR guidelines

- **One logical change per commit.** Atomic commits make review and bisect
  trivial.
- **Reference the OpenSpec proposal.** Significant changes are tracked under
  `openspec/` — link the proposal ID in the PR description.
- **CI must pass.** `make build`, `make test`, and `make lint` all run in
  `.github/workflows/ci.yml` on every push and pull request.
- **Don't bump dependencies casually.** If you do, justify it in the PR
  description and run `go mod tidy`.

## Evaluation harness

Ozy is measured by the committed corpus in `evals/`. The public scoreboard is
[evals/BENCHMARKS.md](evals/BENCHMARKS.md); the methodology is documented in
[evals/METHODOLOGY.md](evals/METHODOLOGY.md); to add a case or run the suite
locally see [evals/README.md](evals/README.md).

A change that affects discovery, ranking, or the catalog flow should ship with
a refreshed benchmark snapshot.

## Release process

Releases are cut from `main` once CI is green and the benchmark gates pass.
The Apache 2.0 license applies to all contributions — see [LICENSE](LICENSE).
