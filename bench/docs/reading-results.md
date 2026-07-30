# How to read the benchmark results

Every run writes `bench/runs/<timestamp>-<scenario>/comparison.md`. This is the
guide to what the modes, tiers, and metrics in it mean, and how to draw a
conclusion from them.

## The one question the bench answers

> In the **same** environment — identical fixture, prompt, model, and tool
> estate — does routing through ozy get the **same task done** for **less
> token / latency cost**?

So you read the results as a cost comparison *at equal success*, not as a
leaderboard. Lower cost at the same success is the broker winning; higher cost or
lower success is it losing. The numbers state facts — the bench applies no
pass/fail threshold to them.

## The three modes

Each mode runs the identical task; only the **tool wiring** the agent sees
differs.

| Mode | What the agent is wired to | Represents |
|------|----------------------------|------------|
| **direct** | Every scenario server: the functional MCPs the task needs **plus the full ≥500-tool corpus** | The "everything installed" worst case — what it costs to hand the model every tool it might conceivably want. |
| **direct-lean** | **Only** the functional MCPs the task needs (all of each server's tools), **no corpus** | The honest real-world baseline — a careful operator installs the MCPs the task needs, not hundreds it doesn't. |
| **ozy** | Only ozy's **3 broker tools** (`findTool` / `describeTool` / `callTool`); ozy brokers the identical full estate behind them | The product — a tiny startup surface with full reach, resolving the right tool out of the estate via semantic retrieval. |

**direct-lean is the mode that matters most.** `ozy − direct-lean` is the primary
comparison, because both are reasonable real setups (broker vs. a disciplined
hand-install). `ozy − direct` is the worst-case baseline (broker vs. dumping the
whole estate on the model).

## The two tiers

| Tier | Needs a model? | What it measures |
|------|----------------|------------------|
| **Static surface** | No | The startup tool/schema footprint each mode advertises *before any work* — deterministic, computed in-process. |
| **Live** | Yes | Real OpenCode runs: task success, token economy, tool-use behavior, over N runs per mode. |

`make bench-surface` produces the surface tier alone (fast, no Docker/model).
`make bench` produces both.

## Reading `comparison.md`, section by section

### Header

```
Model: opencode/big-pickle · Retrieval: semantic · Estimator: chars/4 · Provenance: environment.json
```

- **Retrieval** — which retrieval stack ozy actually exercised: **semantic**
  (hybrid semantic + lexical, the default and intended mode) or **lexical**
  (fallback). Verified at image build time, never assumed. If this says
  `lexical`, ozy is not running its core feature — treat the ozy numbers with
  suspicion and rebuild the image.
- **Estimator** — how token counts are approximated when a run reports none.
- **Provenance** — `environment.json` records the model, versions, and token
  source; it carries no credentials.

### Verdict

A few sentences a reader can quote without opening anything else. It **leads with
efficiency and outcome** — startup schema tokens → total tokens/run → tool
calls/run → duration → success k/N — then the `ozy − direct-lean` and `ozy −
direct` deltas. Tool-attribution figures (canonical hits, distractor calls) come
last, explicitly labeled *informational, not scored*.

### Startup surface (deterministic, no model)

| Metric | Meaning |
|--------|---------|
| **Tools visible** | How many tool schemas the agent sees at startup. |
| **Schema tokens** | Estimated tokens those schemas cost — the fixed per-request tax the agent pays on *every* turn before doing any work. This is the surface ozy shrinks. |
| **Irrelevant schema tokens** | Schema tokens spent on tools outside the scenario's canonical set — i.e. estate bloat the task never needed. |

### Live tier

The headline rows come first (what a broker sells), then an
`_Informational (tool attribution)_` divider, then telemetry that does **not**
affect the verdict.

**Headline (efficiency + outcome):**

| Row | Meaning |
|-----|---------|
| **Total tokens/run** | Input + output tokens per run, `mean±stdev` over N runs. The headline cost. |
| **Tool calls/run** | How many tool invocations the agent made per run. |
| **Duration/run** | Wall-clock seconds per run. |
| **Success k/N** | How many of N runs passed the outcome grader (see below). |
| **Input / Output tokens mean** | The token total split into its two halves. |

**Informational (tool attribution — recorded, never gates success):**

| Row | Meaning |
|-----|---------|
| **Canonical tools hit** | Of the scenario's N "canonical" tools, how many the run engaged (`x/N`). Telemetry about retrieval, not a score. |
| **Distractor calls/run** | Calls to corpus / non-canonical servers. |
| **Downstream calls/run** | Total server-side tool calls (the ground truth for what actually ran). |
| **Wasted tokens/run (est)** | Estimated tokens spent on distractor calls. |
| **Timed out / Parse failed** | Runs the harness could not complete or parse. |
| **Failure reasons** | Named infra failures (`toolset_rejected`, `context_overflow`) — e.g. direct mode exceeding a tool-definition cap at estate scale. Recorded as a finding, never worked around. |

**Delta columns:** `Δ (ozy − direct)` and `Δ (ozy − direct-lean)` — signed
differences (ozy minus the baseline). Success is in percentage points; counts and
means are signed. Negative token / duration deltas mean ozy spent less.

## What makes a run "pass" — outcome-based grading

A run passes on the **outcome**, not on which tool it used. `grading.json` gates
`overall` on **result criteria only**:

- `answer_must_contain` — facts the final answer must include,
- `commit_check` — the correct culprit commit (archaeology scenarios),
- artifact `must_contain` — facts a produced file (e.g. `output/report.pdf`) must contain,
- `forbidden_answer_patterns` — phrases the answer may not contain.

Which tools the agent called (`required_tools`, `forbidden_tool_patterns`) is
recorded in the `informational` section but **never flips the verdict**. So an
agent that reaches the right answer via a different search engine passes; one that
calls every "canonical" tool but produces a wrong answer fails. This is why
`Canonical tools hit` can be below N on a passing run.

### Why a wrong tool pick isn't an unfair trap

Under outcome grading, same-capability rival tools in the corpus behave like the
real service (`behavior` field):

- **functional** — a no-auth rival returns real data and genuinely completes the task,
- **auth_error** — an auth-gated rival (needs an API key/subscription) returns a
  realistic `authentication required` error, so the agent routes to a working tool,
- **stub** — an unrelated-capability tool returns a generic response carrying no
  scenario fact (so a wrong-capability path can't fake success).

A rival pick is therefore either usable or legibly unusable — never a silent dead
end that fails the run through no fault of the agent.

## Provenance you should check

- **Token source** (`measured` / `estimated` / `mixed`) — `measured` comes from
  OpenCode's own usage events; `estimated` is derived from transcript bytes +
  startup schemas when a run reports no usage. `mixed` means don't read token
  deltas as exact.
- **Retrieval** (`semantic` / `lexical`) — must be `semantic` for the ozy numbers
  to reflect the real product.

## One-paragraph read

Confirm **Success k/N** is equal across modes, then compare **Total tokens/run**,
**Tool calls/run**, and **Duration/run**, focusing on the **ozy − direct-lean**
delta. Lower cost at equal success is the broker earning its place; the
`_Informational_` rows explain *how* (retrieval quality, distractor waste) but do
not decide it.
