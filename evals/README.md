# Ozy evaluation suite

The definitive, repeatable measurement of how well Ozy does its job — tool
**discovery**, invocation, agent **ergonomics**, and search **performance**. It
implements [`SPEC.md`](../SPEC.md) §14 and is what every change measures against.

- **[BENCHMARKS.md](BENCHMARKS.md)** — the public scoreboard. Read this first.
- **[METHODOLOGY.md](METHODOLOGY.md)** — the metric math, calibration, and hygiene rules.
- **[snapshots/baseline.json](snapshots/baseline.json)** — the committed reference run.

## Running it

```bash
ozy eval run                 # run every family over the committed corpus
ozy eval run discovery       # scope to one family
ozy eval run --out ""        # run without writing snapshot/scoreboard (CI dry-run)
ozy eval run --format json   # machine-readable result (for CI / tracking)
ozy eval report              # print the latest committed snapshot
```

`ozy eval run` writes a timestamped snapshot plus `snapshots/baseline.json` and
regenerates `BENCHMARKS.md`. It **exits non-zero when a gate fails**, so CI can
gate on it. The corpus is embedded in the binary, so the command works from any
directory; `--out` controls where artifacts are written (default `evals/`).

### Semantic leg (real embedding model)

The semantic and hybrid numbers are produced by the **real** FastEmbed model
through the sidecar — never a stub. It is opt-in so the default run stays fast:

```bash
OZY_EVAL_SEMANTIC=1 ozy eval run        # or: ozy eval run --semantic
```

When the flag is off or the sidecar can't be provisioned, the semantic-only
metrics are recorded as *skipped* and the lexical + structural evals still run.

## The corpus (`data/`)

Everything the suite scores is committed data — adding a case never requires
touching harness code.

```
data/
  catalog/world.json     # the synthetic downstream MCP catalog ("the world")
  discovery/*.jsonl      # labeled intents, one JSON object per line, by category
  invocation/*.json      # callTool scenarios: valid / repair / offline / schema-drift
  ergonomics/*.json      # findTool/describeTool/callTool inputs for §4.5/§9/§13 + parity
  thresholds.json        # gate thresholds (data, ratchetable)
```

### Catalog schema (`catalog/world.json`)

```jsonc
{
  "version": 1,
  "servers": [{ "id": "slack", "status": "online" }],
  "tools": [{
    "toolRef": "slack.post_message",   // MUST equal "<serverId>.<name>"
    "serverId": "slack",
    "name": "post_message",
    "title": "Post Slack Message",
    "description": "Send a message to a Slack channel.",
    "inputSchema": { "type": "object", "required": ["channel", "text"], "properties": { } }
  }]
}
```

The catalog deliberately includes near-duplicate capabilities across servers
(several `search_*` tools) so wrong-server selection is measurable.

### Discovery case schema (`discovery/*.jsonl`)

One JSON object per line:

```jsonc
{
  "intent": "give everyone a heads up that the release is out",
  "category": "semantic",            // lexical | semantic | no_match | ambiguous | wrong_server
  "acceptable": ["slack.post_message"], // [] for no_match (the broker should refuse)
  "rationale": "Broadcasting maps to posting a Slack message; avoids tool-name words."
}
```

| Category | What it tests |
| --- | --- |
| `lexical` | term-overlap intents the BM25 baseline should nail |
| `semantic` | paraphrases with **no** lexical shortcut — needs the embedding leg |
| `no_match` | capability absent from the catalog — the broker must refuse |
| `ambiguous` | more than one acceptable target |
| `wrong_server` | one correct server; same-capability tools elsewhere are traps |

### Invocation scenario schema (`invocation/*.json`)

A file is `{ "version", "_note", "scenarios": [ … ] }`. Each scenario drives
`callTool`; argument validity is judged against the cataloged `inputSchema`:

```jsonc
{
  "name": "jira-create-missing-summary",
  "toolRef": "jira.create_issue",
  "arguments": { "projectKey": "OPS" },
  "expectedOutcome": "repair",            // success | repair | error
  "expectedError": "ARGUMENT_VALIDATION_FAILED", // required for repair/error
  "corrected": { "projectKey": "OPS", "summary": "…" }, // required for repair
  "liveSchema": { },                      // required for TOOL_SCHEMA_CHANGED (the drifted downstream schema)
  "rationale": "…"
}
```

| Outcome | What it tests |
| --- | --- |
| `success` | valid arguments → first-call success |
| `repair` | invalid arguments → structured `expectedError`, then `corrected` succeeds |
| `error` | terminal `expectedError` — `DOWNSTREAM_SERVER_OFFLINE` (target server offline) or `TOOL_SCHEMA_CHANGED` (`liveSchema` differs from the catalog) |

### Ergonomics case schema (`ergonomics/*.json`)

A file is `{ "version", "_note", "cases": [ … ] }`. Each case is exercised on
**both** the CLI `--format json` path and the in-process MCP adapter:

```jsonc
{
  "name": "find-broadcast-to-team",
  "kind": "find",                 // find | describe | call
  "query": "let the whole team know the release shipped", // find
  "toolRef": "confluence.search_pages",  // describe | call
  "arguments": { },               // call
  "expectDecision": "use",        // optional anchor (find)
  "expectErrorType": "TOOL_NOT_FOUND", // optional anchor (describe/call)
  "rationale": "…"
}
```

The harness checks each response is an explicit decision with a grounded,
actionable next step; that every error carries a type and a retry disposition;
that responses stay within the §13 budget; and that the CLI and MCP surfaces
agree (parity). `expectDecision`/`expectErrorType` document intent; the
structural checks run regardless.

### Adding a case

1. Add a line to the right `discovery/*.jsonl` (or a new tool to `world.json`),
   or an entry to an `invocation/*.json` / `ergonomics/*.json` set.
2. Follow the [hygiene rules](METHODOLOGY.md#gold-set-hygiene-rules) — especially: a
   `semantic` intent must not be winnable by lexical search alone.
3. Run `ozy eval run` — the loader validates the data (and fails fast, naming the
   file and field, on a dangling toolRef, a missing rationale, or a bad category),
   and the hygiene check flags lexical freebies in the `semantic` set.

## Gates

`thresholds.json` defines per-scope pass/fail thresholds. `*Min` fields gate
accuracy (higher is better), `*Max` fields gate rates (lower is better). They are
seeded at/below the baseline and **ratcheted upward** as the system improves — see
[METHODOLOGY.md](METHODOLOGY.md#thresholds-seed-low-ratchet-up). Editing a gate is a
data-only change.

## Status

All families are implemented: discovery (lexical **and** real-model hybrid),
invocation & repair, agent ergonomics with CLI↔MCP parity, token economy, and
performance (latency), plus the corpus, reporting, gating, and the `ozy eval`
CLI. The judgment-heavy metrics (instruction usefulness, repair usefulness) are
graded by human/review-board against the rubric scaffold in
[METHODOLOGY.md](METHODOLOGY.md#judgment-heavy-metrics), not an automated judge.
