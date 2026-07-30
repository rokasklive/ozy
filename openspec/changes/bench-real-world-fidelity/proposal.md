## Why

The scenario bench currently decides success partly on **which tool the agent
picked** (`required_tools`, `forbidden_tool_patterns`) — an attribution proxy that
both false-fails (the task was completed via a different, equally-valid tool) and
unfairly penalizes ozy for surfacing more candidates. What we actually care about
is what a broker like ozy sells: **the same or better outcome for a fraction of the
schema/token/latency cost.** So success should be judged on the **result**, and the
scoreboard should lead with **efficiency** (startup schema tokens, total tokens,
tool-call count, duration).

Moving to outcome grading exposes a second problem: the corpus's near-duplicate
distractors are **dead stubs**. Under outcome grading, an agent that reasonably
picks `brave-search` (a real web-search engine) then fails — not because it erred,
but because the bench wired that tool as an empty stub. That is an unfair landmine.
The fix is real-world fidelity: same-capability alternatives must **behave like
their real selves** — no-auth tools work, auth-gated tools return a real auth error
— so a pick is either usable or legibly unusable, and any detour cost lands in the
token data where it belongs.

## What Changes

- **Outcome-based grading.** Run success SHALL be determined by predetermined
  **result** criteria only — `answer_must_contain`, artifact `must_contain`,
  `commit_check`, `forbidden_answer`. `required_tools` and `forbidden_tool_patterns`
  are **removed from the pass/fail decision**; `canonical_tools_hit` /
  `distractor_call_count` remain as **informational** telemetry, demoted from the
  verdict. **BREAKING** for the grading contract (`GroundTruth` semantics).
- **Efficiency-first reporting.** The `comparison.md` verdict leads with startup
  schema tokens → total tokens → tool-call count → duration → success k/N. Tool
  attribution is a footnote, not a headline.
- **De-specified scenario task.** `historical-weather-report` reverts to a
  **capability-level** ask ("a privacy-focused search engine") instead of naming
  DuckDuckGo, and drops the `brave-search`/`kagi-search`/`bing-search`
  `forbidden_tool_patterns` that `bench-enable-semantic` added. (Reverses those two
  deltas of `bench-enable-semantic`; its semantic enablement is untouched.)
- **Real-world corpus fidelity.** Same-capability alternatives behave like the real
  services they mirror:
  - **No-auth** tools (DuckDuckGo, weather-mcp, wikipedia) are **functional** —
    they return real responses (captured from the real MCP) rather than stubs, so
    any of them completes the task.
  - **Auth-gated** tools (Brave/Kagi/Bing search; and keyed services like
    stripe/github/slack) return a realistic **`authentication required`** error
    instead of an empty stub, exactly as they would for an unprovisioned agent.
- **Record-and-replay capture.** A fixture-generation step (network allowed,
  **outside** the hermetic runtime) runs the real no-auth MCPs once, captures their
  actual tool schemas + representative responses, and bakes them into the
  corpus/fixtures for hermetic replay at runtime. Preserves hermeticity and
  deterministic grading.

Not in scope: live MCP calls at runtime (breaks hermeticity + determinism —
explicitly rejected); per-engine response variation beyond "usable results";
re-authoring every one of the ~550 unrelated-capability distractors (only the
same-capability alternatives for a scenario's required capabilities are upgraded).

## Capabilities

### New Capabilities
<!-- none — this evolves existing bench capabilities -->

### Modified Capabilities
- `bench-reporting`: run success SHALL be determined by outcome/result criteria,
  not tool attribution; the cross-mode verdict leads with efficiency metrics and
  treats canonical-hit / distractor counts as informational.
- `bench-fixtures`: the scenario task prompt SHALL specify a capability, not a
  named tool; machine-checkable ground truth SHALL gate success on result criteria
  (tool lists become informational); fixtures MAY be **captured from real no-auth
  MCP servers** at generation time and replayed hermetically.
- `bench-tool-corpus`: same-capability distractor tools SHALL behave like the real
  service they mirror — no-auth alternatives functional, auth-gated alternatives
  returning a realistic auth-required error — rather than a uniform empty stub.

## Impact

- **Grading**: `internal/bench/grader.go` (`Grade` success computation splits
  outcome criteria from informational tool checks), `GroundTruth`/`GradingResult`
  shape, `grader_test.go`.
- **Reporting**: `internal/bench/report.go` verdict/comparison ordering,
  `report_test.go`.
- **Scenario**: `bench/scenarios/historical-weather-report/task.md` (capability
  ask), `expected/ground_truth.json` (drop forbidden patterns; tool lists become
  informational).
- **Corpus/fixtures**: `bench/corpus/*.json` (auth-error `cannedResponse` for keyed
  servers; functional/captured responses for no-auth same-capability tools),
  `internal/bench/corpus.go` + the corpus server mode (`internal/bench/mcp.go` /
  `toolsets.go`) for auth-error + shared no-auth handlers, `internal/bench/fixture*.go`
  for the capture step.
- **Depends on** `bench-enable-semantic` (semantic retrieval); this change revises
  its scenario/grading deltas. Constraint unchanged: hermetic runtime (egress =
  model gateway only), deterministic grading.
