## 1. Mode wiring (`direct-lean` = corpus-filtered `direct`)

- [x] 1.1 In `internal/bench/runner.go`, make `mcpServersFor` skip `s.Corpus` servers when `mode == "direct-lean"`; leave `direct` and `ozy` paths unchanged.
- [x] 1.2 Confirm `writeOpenCodeConfig(dir, "direct-lean", servers)` and the runner's `configPath` (`opencode.direct-lean.jsonc`) flow through unchanged for the new mode (ConfigPath is vestigial; runtime config is written per run).
- [x] 1.3 Add a `toolsets_test.go` (or `runner`-level) case asserting `mcpServersFor("direct-lean", functional+corpus)` returns only the functional server keys.

## 2. Mode set and default

- [x] 2.1 In `internal/bench/runner.go` `Orchestrator.Run`, expand modes: `all → [direct, direct-lean, ozy]`, keep `both → [direct, ozy]`, single modes pass through.
- [x] 2.2 In `internal/bench/run.go`, change the unset-mode default from `both` to `all`; update the `--mode` flag help and the command `Long` text to list `direct | direct-lean | ozy | all | both`.
- [x] 2.3 Select `surfaceTokens` per mode in the live loop: use `surface.DirectLean.SchemaTokens` for `direct-lean` (falls back safely if nil), `Direct`/`Ozy` as today.

## 3. Static surface tier

- [x] 3.1 In `internal/bench/surface.go`, add `DirectLean *SurfaceMetrics` (omitempty) and a `LeanReduction` (ozy vs direct-lean) to `SurfaceComparison`.
- [x] 3.2 In `ComputeSurfaceComparison`, enumerate the functional-only servers (`!bs.Corpus`) into a lean tool set, `MeasureSurface("direct-lean", …)`, and populate the new fields alongside the existing direct/ozy computation.
- [x] 3.3 Update the stderr surface summary line and `surface_test.go` to cover the three-way surface (assert lean tools/tokens sit between direct and ozy for a corpus-attached scenario).

## 4. Reporting (three-mode comparison + delta)

- [x] 4.1 In `internal/bench/report.go` `buildVerdict`, add the `direct-lean` verdict line and emit the `ozy − direct-lean` delta as the primary comparison, keeping the `ozy − direct` delta when direct ran; note lean's structurally-zero distractor calls.
- [x] 4.2 In `writeComparisonMarkdown`, add a `Direct-lean` column and a `Delta (ozy − direct-lean)` column to the live-tier and surface tables; label absent modes "not run (MODE=…)".
- [x] 4.3 Extend `report_test.go` with a three-mode comparison render asserting both deltas, the lean column, and the "not run" labeling when a mode is absent.

## 5. Config template and wiring-test parity

- [x] 5.1 Add `bench/configs/opencode.direct-lean.jsonc` mirroring the functional server set of `opencode.direct.jsonc` (no corpus enumerated), with a header comment explaining lean = functional-only.
- [x] 5.2 In `internal/bench/wiring_test.go`, extend `TestModeTemplatesMatchRunner` to assert the lean template's server keys equal `mcpServersFor("direct-lean", scenarioServers(acmeToolsets, false, …))`.

## 6. Docs

- [x] 6.1 Update `bench/README.md`: describe the three modes, the `ozy − direct-lean` real-world comparison, and the `all` default; note the "New mode" extension still holds.
- [x] 6.2 Update root `README.md` bench section (and `.github/workflows/bench.yml` if it pins `--mode`) to reflect three modes / the `all` default.

## 7. Verify

- [x] 7.1 `go build ./...` and `go test ./internal/bench/...` green.
- [x] 7.2 Run `make bench-surface` (or `--surface-only`) on a corpus-attached scenario and confirm `surface.json`/`comparison.md` show three surfaces with direct-lean between direct and ozy.
- [x] 7.3 Run one live `--mode all` pass in Docker per the bench Docker rule; confirm `comparison.md` renders all three modes and the `ozy − direct-lean` delta, and prior `direct`/`ozy` artifacts are unchanged in shape. **VERIFIED LIVE 2026-07-16** with `BENCH_MODEL=opencode/big-pickle MODE=all BENCH_RUNS=1 BENCH_TIMEOUT=300`: all three modes passed (direct 207s, direct-lean 54s, ozy 110s), `comparison.md` rendered three columns + both deltas (measured token source), no timeouts/parse-fails. (The earlier `deepseek-v4-flash-free` default stalled mid-stream on a gateway 503-adjacent silent hang — external model flakiness, exposed the teardown bug fixed separately.)
