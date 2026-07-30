## 1. View model + artifact loading

- [ ] 1.1 Add a `reportView` struct in `internal/bench/htmlreport.go` that holds the run's `Comparison` (surface + per-mode aggregates + verdict), plus per-mode lists of per-pass detail (metrics, grading, final answer, truncated transcript ref, tool-call trace) and a `schema`/`generatedBy` version stamp.
- [ ] 1.2 Write `loadRunDir(dir string) (*reportView, error)` that reads `comparison.json`, each `<mode>/aggregate.json`, and each `<mode>/<runID>/{metrics.json,grading.json,final-answer.md,tool-calls.jsonl}`; error clearly when `comparison.json` is absent.
- [ ] 1.3 Truncate each pass's `transcript.jsonl` to a fixed byte budget (default 256 KB) and record the relative on-disk path + a truncated flag (design D3).
- [ ] 1.4 Unit-test `loadRunDir` against a small fixture run tree (happy path + missing-`comparison.json` error).

## 2. HTML generator

- [ ] 2.1 Create the embedded template `internal/bench/report.html.tmpl` with inline `<style>`, inline `<script>`, and a `<script type="application/json" id="benchdata">` data island; `//go:embed` it.
- [ ] 2.2 Implement `WriteHTMLReport(dir string, view *reportView) error` that marshals `reportView` to JSON, executes the `html/template` with it, and writes `report.html` into `dir`.
- [ ] 2.3 Verify existing artifacts are untouched by generation (test asserts `comparison.json` bytes unchanged before/after).

## 3. Dashboard + drill-down UI (in-template JS)

- [ ] 3.1 On load, parse `#benchdata` with `JSON.parse` and render the dashboard: verdict text, surface-tier table, and live-tier per-mode table with `ozy − direct-lean` and `ozy − direct` delta columns for the headline metrics.
- [ ] 3.2 Render attribution figures (canonical hits, distractor calls) in a clearly-labeled informational section, never as headlines; label a not-run mode's column with its reason instead of a blank/dash.
- [ ] 3.3 Per-mode view: aggregate stats + a selectable list of passes with each pass's success/failure and headline cost.
- [ ] 3.4 Per-pass view: that run's metrics, grading outcome + deciding criteria, final answer, tool-call trace, and the truncated-transcript note with its relative path.
- [ ] 3.5 Insert all run-derived strings via `textContent` only (no `innerHTML` of untrusted data) — design risk mitigation.

## 4. Point-at-another-run comparison

- [ ] 4.1 Add an `<input type="file">` compare control; on select, read the chosen `report.html` with `FileReader` and extract its `#benchdata` island via `DOMParser`.
- [ ] 4.2 Render both runs' headline metrics side by side with the between-run delta per metric.
- [ ] 4.3 Handle an invalid/unreadable selection with an inline message that leaves the current view intact; show a non-blocking notice when the other report's `schema` version differs.

## 5. Wiring + CLI

- [ ] 5.1 Call `loadRunDir` + `WriteHTMLReport` after `WriteComparison` in `internal/bench/runner.go` (~line 546); log-and-continue on error, never fail the run (design D4).
- [ ] 5.2 Add a `report <run-dir>` subcommand in `internal/bench/cli.go` that runs `loadRunDir` + `WriteHTMLReport` on an existing directory, exiting non-zero with a clear message on missing inputs.
- [ ] 5.3 Add a `make bench-report DIR=...` target (or document the subcommand) for regenerating a report without re-running the bench.
- [ ] 5.4 Add a `gui` goal to the `make bench` target: filter `gui` out of the run-count parsing (extend the Makefile:24 no-op-goal mechanism), and after the Docker run exits, if `gui` was passed, open the newest `bench/runs/*/report.html` with the platform opener (`open`/`xdg-open`). Verify `make bench 10 gui` runs 10 runs and opens the report, and `make bench 10` opens nothing.

## 6. Verification + docs

- [ ] 6.1 Generate `report.html` for a real (or fixture) run dir and confirm it opens from `file://` with no network, renders the dashboard, drills into a pass, and compares against a second run's `report.html`.
- [ ] 6.2 Note in `bench/docs/reading-results.md` that `report.html` is the browsable equivalent of `comparison.md`, with the compare control described in one line.
