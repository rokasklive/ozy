## Context

The bench grew a retrieval-at-scale estate (556 tools) and, in `bench-enable-semantic`,
real semantic retrieval. But two things still measure the wrong thing:

- **Grading gates on tool attribution.** `Grade()` (`internal/bench/grader.go:127-164`)
  folds `required_tools` and `forbidden_tool_patterns` into `Overall` alongside the
  result criteria. A run that completes the task via a different valid tool fails; a
  run that avoids a "forbidden" tool passes even if the outcome is wrong. That is a
  proxy for what we care about (did the task get done, at what cost).
- **Distractors are dead stubs.** Corpus tools return a generic schema-plausible stub
  (`internal/bench/corpus.go`, `CorpusTool.CannedResponse`). Under outcome grading, an
  agent that reasonably picks `brave-search` (a real web-search engine) then fails —
  not because it erred, but because the bench wired that tool as an empty stub. The
  functional tools (`duckduckgo`, `weather`, `wikipedia`, `pdf-toolkit`) are Go
  handlers that read the scenario fixture; the ~550 others are data-defined stubs.

The corpus already lists the real MCPs it mirrors (`mirrorSource`), and the functional
toolsets are modeled on real **no-auth** servers (weather-mcp, duckduckgo-mcp,
wikipedia-mcp). The confusable search distractors (`brave`/`kagi`/`bing`) mirror
**auth-gated** services — Brave/Bing need API keys, Kagi needs a subscription — so a
keyless agent genuinely cannot use them.

## Goals / Non-Goals

**Goals:**
- Success reflects the outcome (result criteria), not which tool was chosen.
- The scoreboard leads with efficiency (schema tokens, total tokens, tool calls,
  duration) + success — the metrics that express a broker's value.
- Same-capability tools behave like the real service: no-auth ones work, auth-gated
  ones fail legibly, so no pick is an unfair silent dead end.
- Stay hermetic and deterministic.

**Non-Goals:**
- Live MCP calls at runtime (breaks hermeticity + determinism — rejected).
- Per-engine result variation beyond "usable results".
- Re-authoring all ~550 unrelated-capability distractors — only the same-capability
  alternatives for a scenario's required capabilities are upgraded.
- Touching `bench-enable-semantic`'s semantic enablement (only its scenario/grading
  deltas are revised here).

## Decisions

### D1: Split grading into outcome (gating) vs tool-usage (informational)
`Grade()` computes the same checks, but `Overall` is the AND of **result** criteria
only — `answer_must_contain`, `commit_check`, artifact `must_contain`,
`forbidden_answer_patterns`. `required_tools` and `forbidden_tool_patterns` move to a
separate `informational` section on `GradingResult` that is recorded and reported but
never flips `Overall`. Alternative: delete the tool checks entirely — rejected; the
telemetry (canonical hits, distractor calls) is still useful, just not a gate.
**BREAKING** for the grading contract; `grader_test.go` and any scenario relying on
tool-gating are updated.

### D2: Verdict leads with efficiency; attribution demoted
`report.go` reorders the verdict/`comparison.md` so schema tokens → total tokens →
tool-call count → duration → success k/N lead; canonical-hit / distractor-call lines
become an informational footer. No metric is removed — only the emphasis changes.
`report_test.go` updates the expected verdict ordering.

### D3: Tool behavior mirrors the real service — three tiers
Corpus/fixture tools resolve to one of three behaviors:
- **Functional (no-auth, same-capability):** returns real data. Reuse the existing
  shared handlers (the search ranker over the fixture corpus; the PDF writer). The
  confusable no-auth alternatives route to the same handler as the canonical tool.
- **Auth-error (auth-gated):** returns a realistic `authentication required / missing
  API key` error. This is a **data** change — a `cannedResponse` (or a new
  `authRequired: true` flag the corpus server honors) on the keyed servers' tools.
- **Generic stub (unrelated capability):** unchanged; must not carry scenario facts.

Preferred mechanism: a data-driven `behavior` field on `CorpusTool`
(`functional:<backend>` | `auth_error` | `stub`) so the corpus server mode
(`internal/bench/mcp.go`) routes accordingly — keeping "corpus is data" intact and
letting future scenarios opt in without Go edits. The functional backend needs the
scenario fixture dir, so corpus servers with a functional tool receive `--fixture-dir`
(as the functional toolsets already do).

### D4: Record-and-replay capture at fixture-generation time
A `fixture` step (network allowed, outside the hermetic runtime) MAY invoke the real
no-auth MCP for a scenario's capabilities and bake the captured tool schemas +
representative responses into the fixture. Runtime replays the baked data with the
existing hermetic network restrictions. This upgrades fidelity of the retrieval
surface (real descriptions/schemas) and response shapes without a runtime dependency.
Capture is **opt-in per scenario** and cached in-repo so a normal run needs no
network. Alternative: keep hand-authored fixtures — rejected for the tools we want to
feel real, kept for everything else. This is the largest new piece and can land after
D1–D3 (grading value is independent of capture).

### D5: De-specify the scenario task
`historical-weather-report/task.md` reverts to "research via a privacy-focused search
engine" (capability, not a named tool); `ground_truth.json` drops the
`brave/kagi/bing` `forbidden_tool_patterns` and its `required_tools` become
informational. This reverses exactly the two scenario deltas `bench-enable-semantic`
added; its semantic work is untouched.

## Risks / Trade-offs

- **Outcome-only grading lets a lucky/hallucinated answer pass** → the result
  criteria are tool-derived facts (e.g. `25.3`/`14.9` only come from the weather
  tool; artifact PDF must embed them), so the outcome already forces the real
  capability chain. Wrong-capability tools carry no facts (D3), so they can't fake it.
- **Functional no-auth distractors all return the same corpus → unrealistic sameness**
  → accepted; "usable results containing the facts" is the fidelity bar. Per-engine
  variation is a non-goal.
- **Capture pipeline adds a network step + baked blobs** → opt-in, generation-time
  only, cached in-repo; a normal run stays hermetic. Real MCPs can drift/version →
  the baked capture is pinned; re-capture is explicit.
- **BREAKING grading change invalidates old pass/fail comparisons** → intended;
  documented in the provenance/README so pre-change numbers aren't mixed in.

## Migration Plan

1. Land D1 (grading) + D2 (verdict) + D5 (task) — small, high-value, independently
   verifiable with the existing image.
2. Land D3 (three-tier tool behavior) — data + corpus-server routing; re-run to
   confirm auth-gated picks error and no-auth alternatives complete the task.
3. Land D4 (capture) last — its own step; a run without capture still works.
   Rollback is per-decision (each is an isolated diff).

## Open Questions

- `behavior` field on `CorpusTool` vs promoting confusable tools to Go functional
  toolsets — decide at implementation by which is the smaller diff for the ~4 tools
  in scope (lean data-driven).
- Which auth-gated servers get the auth-error treatment now: only the scenario's
  same-capability rivals (brave/kagi/bing), or all keyed services — start with the
  in-scope rivals, widen if a run shows the agent wandering into other keyed stubs.
