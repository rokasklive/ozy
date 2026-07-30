## Context

A bench run writes a tree of artifacts under `bench/runs/<ts>-<scenario>/`:

```
comparison.json   comparison.md   surface.json   environment.json
<mode>/                              # direct | direct-lean | ozy
  aggregate.json
  <runID>/
    metrics.json  grading.json  final-answer.md
    transcript.jsonl  tool-calls.jsonl  calls.jsonl  workspace/
```

`internal/bench/report.go` already defines the Go structs behind these files
(`Comparison`, `Aggregate`, `RunMetrics`) and `SurfaceComparison` lives in
`surface.go`. The runner writes the comparison as its last step at
[runner.go:546](internal/bench/runner.go). Everything the viewer needs is
already on disk and already has typed Go models — this change is a read-only
presentation layer over them.

The bench runs in Docker (hard constraint), unattended, and the result directory
is copied out. So the report must be a single portable file that works offline
with no server — the person reading it may be looking at a downloaded artifact on
their laptop, not the machine that ran the bench.

## Goals / Non-Goals

**Goals:**
- One self-contained `report.html` per run, generated automatically at run end and
  regenerable from an existing run dir via `bench report <run-dir>`.
- Drill-down from dashboard → per-mode → per-pass with all per-run detail.
- Compare against another run by pointing the page at its `report.html`, entirely
  in the browser.
- Zero new dependencies; no build step; no JS toolchain.

**Non-Goals:**
- No live/streaming view during a run — the report is a post-run artifact.
- No server, no SPA framework, no charting library.
- No change to what metrics mean or how they are computed — presentation only.
- No hosting/sharing pipeline; distribution is "copy the HTML file."
- No aggregation across >2 runs or historical trend lines (2-way compare only).

## Decisions

### D1: Go `html/template` + `embed`, data as an embedded JSON blob

The generator loads the run dir's artifacts into the existing structs, marshals a
single view model to JSON, and injects it into a template rendered with stdlib
`html/template`. The template (one `.html` with `<style>` and `<script>` inline)
is `//go:embed`-ed into the binary. The page reads its data from
`<script type="application/json" id="benchdata">…</script>` on load and renders
the DOM with vanilla JS.

- **Why**: rungs 2–3 of the ladder — stdlib does it, native browser features (a
  `<script type="application/json">` island + `FileReader`) cover the rest. No
  React, no bundler, no CDN. The structs already exist; the generator is mostly
  "read files, assemble struct, execute template."
- **Alternatives**: (a) a JS framework/SPA — rejected: build step, dependencies,
  CSP/offline friction for a static artifact. (b) Server-rendered dashboard —
  rejected: the artifact must open from `file://` after being copied out of
  Docker. (c) Emit only JSON and a separate generic viewer — rejected: two files
  to keep in sync; the requirement is one HTML file.

### D2: Compare is a browser-side feature via `FileReader` + embedded JSON island

The compare control is `<input type="file">`. On select, the page reads the chosen
`report.html` with `FileReader`, extracts the `#benchdata` JSON island with
`DOMParser` (not regex), and diffs the two view models client-side. Because the
data lives in a parseable island, any run's `report.html` is also its own portable
data source — no sibling `.json` file to emit or track.

- **Why**: satisfies "user points at another set of runs" with no server, no Go
  `--compare` flag, no re-generation. Works from `file://` because `FileReader`
  operates on user-selected files (unlike `fetch`, which Chrome blocks for
  `file://`).
- **Alternatives**: (a) Go `bench report --compare <dir>` baking both runs in —
  rejected as the primary path: forces re-generation to compare, and comparing two
  *existing* reports would be impossible. Could be added later if a non-interactive
  compare artifact is ever needed. (b) `fetch()` a sibling data file — rejected:
  blocked under `file://` in Chromium.

### D3: Embed structured artifacts fully; cap raw transcripts

`metrics.json`, `grading.json`, `final-answer.md`, and `tool-calls.jsonl` are
small and structured — embed them fully per pass. `transcript.jsonl` is the raw
OpenCode stream and can be multi-MB per run; embedding every transcript across
3 modes × N runs would bloat the HTML to tens of MB.

Decision: embed the tool-call trace (already compact) and final answer inline;
embed the raw transcript **only** truncated to a fixed byte budget per pass, with
a visible "truncated — full transcript at `<mode>/<runID>/transcript.jsonl`" note
and the relative path. This keeps the artifact portable and the page responsive.

`ponytail: fixed per-transcript byte cap; if a scenario needs full raw
transcripts in-page, raise the cap or add opt-in full embedding behind a flag.`

- **Why**: bounded HTML size with the detail that actually gets read (metrics,
  grading, tool trace, answer) always present. The full raw stream stays one file
  away on disk for the rare deep dive.
- **Alternative**: inline everything — rejected: unbounded artifact size, sluggish
  page. Lazy-load transcripts via `fetch` — rejected: `file://` blocks it (D2).

### D4a: `gui` auto-open is a host-side Makefile step, never in the container

`make bench <runs> gui` opens the report after the run. The open runs on the
**host** after `docker compose ... up` exits, not inside the bench container: the
container has no browser and no display, and the memory constraint is that the
bench runs in Docker, never on the host — but *reading* its output is a host
action. The Makefile resolves the newest `bench/runs/*/` dir
(`ls -td bench/runs/*/ | head -1`) and runs the platform opener (`open` on macOS,
`xdg-open` on Linux). `gui` is detected as a make goal and filtered out of the
run-count parsing (same no-op-goal mechanism the run count already uses at
Makefile:24), so `make bench 10 gui` still means 10 runs.

- **Why**: the Go binary stays environment-agnostic — it only ever writes the
  file. Browser launching is a host concern and belongs in the host-side target.
- **Alternatives**: (a) have the Go binary shell out to `open` — rejected: it runs
  in Docker where that fails, and it couples the generator to a display. (b) a
  separate `make bench-gui` target — rejected: duplicates the run recipe; a goal
  flag is lazier and composes with the existing run-count goal.

`ponytail: opener is open/xdg-open by OS; add a Windows 'start' branch only if a
Windows host ever runs the bench.`

### D4: Generation is best-effort and never fails the run

The report is written after `WriteComparison` and its error is logged, not
returned — a formatting bug in the viewer must never fail an expensive Docker
bench run whose real outputs (`comparison.json`) already landed.

- **Why**: the JSON/MD artifacts are the source of truth and are already written;
  the HTML is a convenience view. Failing the run over it would be the tail wagging
  the dog.

## Risks / Trade-offs

- **[Large HTML from many passes/transcripts]** → D3 caps transcripts; structured
  data is small. A 3-mode × 5-run report embeds ~15 metrics/grading blobs — well
  within a comfortable single-file size.
- **[XSS from embedded transcript/answer text rendered into the DOM]** →
  `html/template` auto-escapes template-injected values; the JSON island is emitted
  as text and parsed with `JSON.parse`, and all run-derived strings are inserted
  via `textContent`, never `innerHTML`. No untrusted string reaches `innerHTML`.
- **[Two reports with mismatched schema versions compared]** → embed a small
  `schema`/`generatedBy` version in the JSON island; on compare, if versions differ
  the page shows a non-blocking "generated by a different bench version" notice and
  compares the fields it recognizes.
- **[Viewer drifts from `comparison.md` semantics]** → the generator reuses the
  same structs and the same delta math the Markdown writer uses; it does not
  recompute metrics independently. Labels are sourced to match
  `bench/docs/reading-results.md`.
- **[Browser variance under `file://`]** → the page uses only widely-supported APIs
  (`DOMParser`, `FileReader`, `<script type="application/json">`); no modules, no
  `fetch`, no service workers.

## Migration Plan

Additive. No migration: the report is a new artifact next to existing ones. Old run
directories gain the viewer on demand via `bench report <run-dir>`. Rollback is
deleting the generator call — the source-of-truth artifacts are unaffected.

## Open Questions

- Transcript byte cap value (D3) — pick a default (e.g. 256 KB/pass) during
  implementation; trivially tunable.
- Whether to link into `workspace/` produced artifacts (e.g. a generated PDF) from
  the per-pass view, or only name them. Default: name + relative path (consistent
  with D3's transcript handling); revisit if a scenario's artifact is central to
  judging the pass.
