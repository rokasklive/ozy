## Why

The bench already writes `comparison.json` / `comparison.md` and a tree of
per-mode aggregates and per-run artifacts, but reading a result today means
opening Markdown and hand-grepping JSON across `<mode>/<runID>/` directories.
There is no way to drill from the headline verdict down to a single pass, and no
way to hold two runs side by side to see whether a change moved the numbers. A
single self-contained HTML report — generated automatically when a run finishes —
turns the existing artifacts into a browsable dashboard and makes run-to-run
comparison a two-click operation.

## What Changes

- The bench emits a single self-contained `report.html` into the run directory at
  the end of every `all`/`both`/single-mode run, alongside the existing
  `comparison.json` / `comparison.md`. No build step, no external assets: CSS and
  JS are baked inline, all result data is embedded in the page, and it opens from
  `file://` with no server.
- A new `ozy bench report <run-dir>` subcommand regenerates `report.html` from an
  existing run directory's on-disk artifacts, so old runs get the viewer without
  re-running the benchmark.
- A `gui` word in the `make bench` invocation (`make bench <runs> gui`) opens the
  freshly generated `report.html` in the host's default browser once the run
  finishes. Opening is a **host-side** Makefile step after the Docker container
  exits — the containerized bench cannot and does not launch a browser. Without
  `gui`, behavior is unchanged (report is written, nothing opens).
- The report renders three drill-down levels: **dashboard** (verdict, surface
  tier, live-tier per-mode table with `ozy − direct-lean` and `ozy − direct`
  deltas), **per-mode** (aggregate + list of passes), and **per-pass** (that run's
  metrics, grading result, final answer, and tool-call trace).
- The report includes an in-browser **compare** control: the user points it at
  another run's `report.html` via a file picker, and the viewer renders both runs'
  headline metrics and deltas side by side. Comparison is a runtime feature of the
  generated page — no re-generation and no second Go invocation required.
- New reporting artifact only; no existing metric, grader, or `comparison.*`
  output changes. Not breaking.

## Capabilities

### New Capabilities
- `bench-html-report`: generation of a single self-contained HTML report from a
  bench run directory (auto-emitted at run end and regenerable via a CLI
  subcommand), its dashboard / per-mode / per-pass drill-down, and the in-browser
  point-at-another-run comparison view.

### Modified Capabilities
<!-- None. The existing bench-reporting comparison.json / comparison.md outputs are
     unchanged; this adds a new artifact and consumes them read-only. -->

## Impact

- **Code**: `internal/bench/` — a new report generator (Go `html/template` +
  `embed`, no new dependencies) invoked after `WriteComparison` in
  [runner.go:546](internal/bench/runner.go); a new `report <run-dir>` subcommand
  wired into the bench CLI ([cli.go](internal/bench/cli.go)).
- **Makefile**: the `bench` target gains an optional `gui` goal that, after the
  Docker run exits, resolves the newest `bench/runs/*/` directory and opens its
  `report.html` with the platform opener (`open` on macOS, `xdg-open` on Linux).
- **Artifacts**: adds `report.html` per run directory. Consumes existing
  `comparison.json`, `<mode>/aggregate.json`, `<mode>/<runID>/metrics.json`,
  `grading.json`, `final-answer.md`, `tool-calls.jsonl` read-only.
- **Dependencies**: none added — stdlib `html/template`, `encoding/json`, `embed`.
- **Docs**: `bench/docs/reading-results.md` gains a short note that `report.html`
  is the browsable equivalent of `comparison.md`.
- **No impact** on the Docker/hermetic run path, graders, or token accounting.
