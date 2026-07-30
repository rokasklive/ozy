## Why

The bench compares two extremes: `direct` (every MCP wired, all ≥500 corpus tools
loaded) and `ozy` (only the broker surface, everything else behind retrieval).
Neither is how a careful operator actually configures an agent — nobody installs
500 tools they don't need. The honest real-world baseline is "wire only the MCPs
whose tools the task needs, and pay for every tool those MCPs ship." Without it,
`direct` looks like a strawman and ozy's win is easy to dismiss as beating a
configuration no one would run.

## What Changes

- Add a third execution mode, `direct-lean`: wire **only the functional-toolset
  MCP servers** the scenario declares, and load **all** of each server's tools
  (including its non-task sibling tools), but **omit the ≥500-tool corpus**. It is
  `direct` minus the corpus at wiring time — the "installed only what I need"
  baseline.
- Make a single bench invocation run all three modes (`direct`, `direct-lean`,
  `ozy`) in one pass. The default mode (unset `MODE`) becomes `all` (the three
  modes); `both` is kept as a back-compat alias for `direct` + `ozy`. **BREAKING**
  for anyone relying on the unset default meaning exactly two modes.
- Extend the static surface tier to compute a third `direct-lean` surface
  (functional servers only, no corpus) alongside `direct` and `ozy`.
- Extend `comparison.{json,md}` to report all three modes and add the headline
  **ozy − direct-lean** delta (the real-world comparison), keeping the existing
  ozy − direct delta.
- Add the `bench/configs/opencode.direct-lean.jsonc` reference template and update
  bench docs to describe three modes.

## Capabilities

### New Capabilities
<!-- None — this extends existing bench capabilities rather than introducing a new one. -->

### Modified Capabilities

- `scenario-bench`: introduce the `direct-lean` execution mode (functional
  toolsets wired, corpus omitted, all tools of each wired server loaded); make one
  invocation run direct + direct-lean + ozy as a single pass with `all` as the
  default; extend the static surface tier to a third `direct-lean` surface.
- `bench-reporting`: `comparison.{json,md}` reports three modes and surfaces the
  `ozy − direct-lean` delta as the primary real-world comparison; per-mode
  aggregates and surface accounting cover `direct-lean`.

## Impact

- Code: `internal/bench/runner.go` (mode list, per-mode wiring + surface-token
  selection), `internal/bench/surface.go` (third surface + reduction),
  `internal/bench/report.go` (three-mode comparison + delta), `internal/bench/run.go`
  (mode flag help + default). `mcpServersFor` gains corpus-filtering for the lean
  mode.
- Config/docs: new `bench/configs/opencode.direct-lean.jsonc`; `bench/README.md`,
  root `README.md` mode descriptions.
- Tests: `wiring_test.go` (`TestModeTemplatesMatchRunner` covers the new template),
  plus surface/report unit coverage for the third mode.
- No new dependencies. Builds on the unarchived `hermetic-e2e-bench` and
  `bench-retrieval-at-scale` changes (which own the current `scenario-bench` and
  `bench-reporting` deltas); lands after them.
