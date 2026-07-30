## Context

The bench harness (`internal/bench/`) runs a scenario against an agent in two
wirings and compares them. The estate for a scenario is resolved once by
`scenarioServers(toolsets, corpus, …)` into an ordered `[]benchServer`: the
scenario's **functional toolsets** first (each a real fixture MCP server that
also carries mirrored non-task sibling tools), then every **corpus** server when
`corpus: true` (the ≥500-tool realism estate), each tagged `benchServer.Corpus`.

- `direct` mode wires *all* of that set via `mcpServersFor("direct", servers)`.
- `ozy` mode wires only the broker; `setupOzy` points Ozy at the same full set.

The static surface tier (`ComputeSurfaceComparison`) enumerates both wirings
in-process with no model. Per-mode live runs write `metrics.json` →
`aggregate.json`; `WriteComparison` renders `comparison.{json,md}` with a hardcoded
two-column (Direct/Ozy) table and an `ozy − direct` delta.

The insight that makes this change small: **`direct-lean` is exactly `direct`
with the corpus servers filtered out at wiring/enumeration time.** The functional
toolset servers already bundle their sibling stubs, so "load all tools of a needed
MCP even if the task uses three" falls out for free — no per-tool filtering, just
per-server `!Corpus` filtering.

## Goals / Non-Goals

**Goals:**
- Add `direct-lean` as a third mode: functional servers only, all their tools, no corpus.
- One invocation runs `direct` + `direct-lean` + `ozy` (default `all`).
- Third surface in `surface.json`; three-mode `comparison.{json,md}` with the `ozy − direct-lean` delta as the headline.
- Keep the existing `direct` and `ozy` behavior and artifacts byte-for-byte unchanged when those modes run.

**Non-Goals:**
- No change to fixtures, corpus, grading, retrieval scoring, or token-source logic.
- No new per-tool selection ("load only the 3 tools used") — lean loads whole servers, matching real MCP installs.
- No generalized N-mode reporting engine; the three modes are enumerated explicitly.
- No new external dependencies.

## Decisions

### D1: `direct-lean` = corpus-filtered `direct`, keyed off `benchServer.Corpus`

`mcpServersFor` becomes mode-aware for the lean case: for `direct-lean` it skips
`s.Corpus` servers; `direct` and `ozy` are untouched. The surface tier filters the
same way before enumeration. One predicate (`!Corpus`) is the entire semantic
difference, so direct and direct-lean cannot drift in what "functional" means.

*Alternative — a separate lean server list plumbed through the Orchestrator:*
rejected; it duplicates the estate and invites the two lists to diverge. Filtering
the single resolved set at the wiring boundary keeps one source of truth.

### D2: Mode set — default `all`, keep `both`

`--mode`/`MODE` accepts `direct | direct-lean | ozy | all | both`. The orchestrator
expands `all → [direct, direct-lean, ozy]` and `both → [direct, ozy]` (unchanged).
Unset default flips from `both` to `all` so the user gets the three-mode pass with
zero config — the stated purpose. `both` is retained so existing scripts/CI keep
their exact two-mode behavior.

*Alternative — reuse `both` to mean all three:* rejected; it silently changes what
existing `both` invocations produce. A new keyword makes the expansion explicit and
back-compat trivial.

### D3: Surface struct — add an optional `DirectLean` field, not a rewrite

`SurfaceComparison` gains `DirectLean *SurfaceMetrics` (pointer, omitempty) plus a
`LeanReduction` (ozy vs direct-lean). Existing `Direct`/`Ozy`/`Reduction` stay,
so every current reader and the surface-only path keep working. The lean surface is
always computed (cheap, deterministic, model-free), so it is present even for a
single-mode live run.

*Alternative — replace the named fields with `Modes map[string]SurfaceMetrics`:*
rejected as scope creep; it churns every surface consumer and the JSON shape for no
new capability.

### D4: Reporting — extend to three columns + a second delta, still explicit

`aggregates` is already `map[string]*Aggregate` keyed by mode, so `direct-lean`
flows through aggregation for free. The change is confined to `buildVerdict` and
`writeComparisonMarkdown`: add the `direct-lean` column, emit the `ozy − direct-lean`
delta as the primary line (and keep `ozy − direct`), and label any absent mode
"not run". The markdown grows a "Delta (ozy − direct-lean)" column alongside the
existing one.

### D5: Config template + wiring test parity

Add `bench/configs/opencode.direct-lean.jsonc`. Because the checked-in `direct`
template already enumerates only the functional set (corpus is never listed there),
the lean template is the same functional server list. `TestModeTemplatesMatchRunner`
gains a case asserting `mcpServersFor("direct-lean", functionalOnly)` equals the
lean template's server keys — with `corpus=false` in the test, direct and
direct-lean wire the same functional set, which is exactly what the templates
document.

### D6: Structural facts to record, not fight

In `direct-lean` no corpus server is wired, so `computeRetrieval` counts **zero
distractor calls** by construction (every reachable server is functional). This is
correct and meaningful — lean's cost is the *schema surface* of loading whole MCPs
(functional tools + their in-server siblings), not corpus-call waste. The verdict
notes this so a reader doesn't misread lean's low distractor count as retrieval
quality. `surfaceTokens` for the estimated token source is selected per mode
(`surface.DirectLean.SchemaTokens` for lean), so estimated runs price the lean
surface, not direct's.

## Risks / Trade-offs

- **Stale requirement rename** (`Two-mode…` → `Identical-environment multi-mode…`) → RENAMED delta plus MODIFIED body; validated by `openspec validate`. Low risk, name-only.
- **Default behavior change (`both` → `all`)** → BREAKING for anyone depending on the unset default being two modes; mitigated by keeping `both` explicit and documenting the flip in proposal + bench README. Runtime cost rises ~50% (three modes instead of two) per invocation — acceptable for the informational bench, and a single mode can still be selected.
- **Report code is hand-rolled two-column** → adding a third column touches every row helper; mitigated by keeping helpers mode-parameterized and adding a `report_test.go` case that renders a three-mode comparison and asserts both deltas and the "not run" labels.
- **Lean surface between direct and ozy could be misread as ozy's competitor** → the design is explicit that `ozy − direct-lean` is the intended real-world comparison; the verdict states it.

## Migration Plan

1. Land after the unarchived `hermetic-e2e-bench` and `bench-retrieval-at-scale` changes (they own the current `scenario-bench`/`bench-reporting` deltas).
2. Ship code + template + docs together; no data migration. Existing run directories are unaffected (new fields are additive/omitempty).
3. Rollback: revert the change; `both`/`direct`/`ozy` invocations were never altered, so no artifact-shape breakage for prior modes.

## Open Questions

- Should the primary headline delta be `ozy − direct-lean` in all outputs, or should `ozy − direct` remain primary and lean be secondary? (Design assumes lean is primary when present, per the proposal's real-world-comparison intent.)
- Do CI/`bench.yml` and `make bench` want `all` by default too, or should CI pin `--mode all` explicitly to make the three-mode cost visible in the workflow? (Design leans to pinning it explicitly in CI.)
