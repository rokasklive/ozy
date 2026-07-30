## ADDED Requirements

### Requirement: Self-contained HTML report artifact

The bench SHALL write a single `report.html` file into the run directory whenever
a run produces `comparison.json`. The file SHALL be fully self-contained: all CSS
and JavaScript SHALL be inlined, all result data SHALL be embedded in the page,
and it SHALL render correctly when opened directly from the filesystem
(`file://`) with no network access, no web server, and no external asset. The
report generation SHALL NOT modify or replace `comparison.json`, `comparison.md`,
or any per-mode or per-run artifact.

#### Scenario: Report emitted at run end

- **WHEN** a bench run finishes writing `comparison.json` for a run directory
- **THEN** a `report.html` file exists in that same run directory

#### Scenario: Opens with no server or network

- **WHEN** `report.html` is opened directly from disk via a `file://` URL with no network available
- **THEN** the dashboard renders fully, including all embedded result data, with no failed external requests

#### Scenario: Existing artifacts untouched

- **WHEN** the report is generated
- **THEN** `comparison.json`, `comparison.md`, `surface.json`, `environment.json`, and every `<mode>/<runID>/` artifact are byte-for-byte unchanged

### Requirement: Regenerate report from an existing run directory

The bench CLI SHALL provide a `report <run-dir>` subcommand that reads the on-disk
artifacts of an already-completed run directory and writes `report.html` into it,
without re-running the benchmark. The subcommand SHALL fail with a clear error
when the target directory is missing required inputs (at minimum
`comparison.json`).

#### Scenario: Regenerate for an old run

- **WHEN** `report <run-dir>` is invoked on a directory that contains `comparison.json` but no `report.html`
- **THEN** `report.html` is written into that directory and no live agent run is started

#### Scenario: Missing inputs reported

- **WHEN** `report <run-dir>` is invoked on a directory with no `comparison.json`
- **THEN** the command exits non-zero with a message naming the missing input

### Requirement: Dashboard, per-mode, and per-pass drill-down

The report SHALL present the run at three levels of detail navigable within the
single page:

- **Dashboard**: the verdict text, the surface tier (tools visible, schema tokens,
  irrelevant schema tokens per mode), and a live-tier table with one column per
  run mode and the `ozy − direct-lean` and `ozy − direct` delta columns for each
  headline metric (total tokens/run, tool calls/run, duration/run, success k/N,
  input/output tokens). Tool-attribution figures SHALL be shown as clearly labeled
  informational rows and SHALL NOT lead the dashboard.
- **Per-mode**: for each run mode, its aggregate statistics and the list of its
  individual passes (runs), each pass showing its success/failure and headline
  cost.
- **Per-pass**: for a selected pass, that run's `metrics.json` figures, its
  `grading.json` outcome (pass/fail with the criteria that decided it), its final
  answer, and its tool-call trace.

Every metric label the report displays SHALL carry the same meaning as in
`comparison.md` (no metric is renamed or recomputed).

#### Scenario: Dashboard shows verdict and deltas

- **WHEN** the dashboard is viewed for a run where `direct-lean` and `ozy` both have aggregates
- **THEN** the verdict text is shown and the live-tier table shows the `ozy − direct-lean` delta for total tokens/run, tool calls/run, duration/run, and success k/N

#### Scenario: Drill from mode to a single pass

- **WHEN** the user selects a run mode and then one of its passes
- **THEN** the report shows that pass's metrics, its grading outcome and the criteria that decided it, its final answer, and its tool-call trace

#### Scenario: Attribution kept informational

- **WHEN** the dashboard is viewed
- **THEN** canonical-tools-hit and distractor-call figures appear only in a section labeled informational, never as a headline metric

#### Scenario: Missing mode labeled, not blank

- **WHEN** a run mode has no aggregate (it was not part of the invocation)
- **THEN** that mode's column is labeled with the reason it did not run rather than shown as a blank or bare dash

### Requirement: Optional auto-open in the default browser

The `make bench` target SHALL accept an optional `gui` word in its invocation
(`make bench <runs> gui`). When present, after the run completes and `report.html`
has been written, the target SHALL open that report in the host's default browser
using the platform opener. Opening SHALL be a host-side step performed after the
Docker run exits; the containerized bench SHALL NOT attempt to launch a browser.
When `gui` is absent, the target SHALL behave exactly as before (the report is
still written; nothing is opened). The run count SHALL still be parsed correctly
whether or not `gui` is present.

#### Scenario: gui opens the report

- **WHEN** `make bench <runs> gui` completes and a `report.html` exists in the newest run directory
- **THEN** that `report.html` is opened in the host default browser via the platform opener

#### Scenario: no gui, no open

- **WHEN** `make bench <runs>` is run without `gui`
- **THEN** `report.html` is written and no browser is opened, and the run count is honored

#### Scenario: run count parsed alongside gui

- **WHEN** `make bench 10 gui` is invoked
- **THEN** the run executes with 10 runs and `gui` is not mistaken for the run count

### Requirement: Point-at-another-run comparison

The report SHALL provide an in-page control that lets the user select another
run's `report.html` from the local filesystem and compare the two runs. When a
second run is loaded, the report SHALL render the headline metrics of both runs
side by side together with the between-run delta for each. Loading the comparison
SHALL NOT require re-generating either report, a web server, or a second
invocation of the bench tool.

#### Scenario: Load a second run and see side-by-side deltas

- **WHEN** the user selects another run's `report.html` via the compare control
- **THEN** the report shows both runs' headline metrics side by side with the delta between them for each metric

#### Scenario: Incompatible or unreadable selection

- **WHEN** the user selects a file that is not a valid bench report
- **THEN** the report shows a clear inline message and leaves the current run's view intact
