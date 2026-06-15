# eval-harness

## Purpose

Define the eval harness — the runner that loads the eval-datasets corpus, drives the broker to compute discovery/invocation/repair metrics, runs agent-ergonomics conformance checks, measures token economy, benchmarks performance against the real embedding model, and emits a structured run result with a gate signal — so the system is continuously evaluated against the committed corpus.

## Requirements

### Requirement: Data-driven scenario loading

The eval harness SHALL load every scenario and gold label from committed data
files in the `eval-datasets` corpus rather than from inline Go fixtures, validate
each file against the dataset schema before running, and fail the run with a clear
error that names the offending file and field when a dataset is malformed. Adding
or changing a scenario MUST be possible by editing data only, without changing
harness code.

#### Scenario: Scenarios come from data, not code

- **WHEN** a new discovery intent with its expected target tool is added to a gold-set data file and the harness is run
- **THEN** the new intent is evaluated and counted in the metrics without any change to harness Go source

#### Scenario: Malformed dataset fails fast

- **WHEN** a dataset file is missing a required field or references a toolRef that is not present in the corpus catalog
- **THEN** the harness aborts before scoring and reports the file path and the specific validation failure

### Requirement: Deterministic discovery metrics

The harness SHALL drive the real search engine and broker `findTool` seam over the
corpus catalog and compute, per discovery gold set, the metrics defined in
`SPEC.md` §14.2: top-1 accuracy, top-3 accuracy, mean reciprocal rank,
wrong-server rate, and no-match correctness. For a fixed corpus, model, and
configuration the computed metrics MUST be deterministic across runs.

#### Scenario: Discovery metrics are computed from the broker

- **WHEN** a discovery gold set is evaluated
- **THEN** the harness reports top-1, top-3, MRR, wrong-server rate, and no-match correctness derived from the broker's ranked decision for each intent

#### Scenario: No-match intents are scored as correct refusals

- **WHEN** an intent whose capability is absent from the catalog is evaluated
- **THEN** a `no_good_match` (or `catalog_empty`) decision counts as correct for no-match correctness, and a confident wrong selection counts as incorrect

#### Scenario: Repeated runs are stable

- **WHEN** the same discovery gold set is evaluated twice with the same corpus and configuration
- **THEN** the reported metric values are identical

### Requirement: Invocation and repair metrics

The harness SHALL evaluate `callTool` against fixtures that reproduce the
`SPEC.md` §14.1 invocation and repair families — valid arguments, invalid
arguments, schema drift, and offline downstream servers — and compute valid
argument rate, first-call success rate, repair success rate, schema error rate,
and downstream error clarity. Repair evaluation MUST verify that a structured
error leads to a corrected follow-up call that succeeds.

#### Scenario: First-call success on valid arguments

- **WHEN** an invocation scenario supplies arguments that satisfy a reachable tool's schema
- **THEN** the harness records a successful first call and counts it toward first-call success rate

#### Scenario: Repair loop after a structured error

- **WHEN** an invocation scenario supplies invalid arguments, receives an `ARGUMENT_VALIDATION_FAILED` error, and then issues the corrected call described by the error's repair guidance
- **THEN** the harness records the corrected call as a repair success

#### Scenario: Offline downstream is scored, not crashed

- **WHEN** an invocation scenario targets a tool whose downstream server is offline
- **THEN** the harness records a structured `DOWNSTREAM_SERVER_OFFLINE` error with a non-retry-amplifying instruction and does not count it as a first-call success

### Requirement: Agent-ergonomics conformance checks

The harness SHALL evaluate the agent-facing CLI and MCP interfaces against the
`SPEC.md` §4.5 instructional-quality criteria and §9 contract shapes as
automatable, structural checks: every `findTool` result is a decision carrying a
grounded, conditional, actionable `agentInstruction` or `nextAction`; every error
carries a structured type and a retry-disposition; and responses stay within the
§13 response budgets. The harness SHALL additionally verify CLI ↔ MCP parity for
the same inputs and provide a rubric scaffold for the judgment-heavy metrics that
`SPEC.md` §14 grades by human/review-board.

#### Scenario: Every decision is instructional

- **WHEN** the ergonomics checks run over the discovery gold set on both the CLI and MCP surfaces
- **THEN** each result carries an explicit decision value and a non-empty, grounded next-action or agent instruction, and any result that restates the query without a concrete next action is flagged as a failure

#### Scenario: CLI and MCP agree

- **WHEN** the same `findTool`, `describeTool`, and `callTool` inputs are run through the CLI `--format json` path and the MCP adapter
- **THEN** the harness asserts the two produce semantically equivalent decisions, selected toolRefs, and error types, and flags any divergence

#### Scenario: Responses stay within budget

- **WHEN** a `findTool` or `callTool` response is measured
- **THEN** the harness records its size and flags responses that exceed the configured §13 budget (e.g. full schemas in `findTool`, or unbounded `callTool` results)

### Requirement: Token-economy measurement

The harness SHALL measure the `SPEC.md` §13 token-economy metrics deterministically
from captured request/response payloads and tool schemas — startup tool-schema
tokens, total tokens-to-success, largest response payload, and number of broker
calls — and report them for both a direct-MCP baseline (all downstream tool
schemas loaded) and the Ozy broker path (the three Ozy tools plus per-task calls),
using a documented, swappable token estimator.

#### Scenario: Direct-MCP vs Ozy comparison

- **WHEN** a token-economy scenario is evaluated
- **THEN** the harness reports startup schema tokens, tokens-to-success, largest payload, and broker-call count for both the direct-MCP baseline and the Ozy path

#### Scenario: Token estimator is documented and swappable

- **WHEN** the token estimator is changed
- **THEN** only the estimator implementation changes and the methodology document states which estimator produced the committed numbers

### Requirement: Real embedding model for semantic evals

The harness SHALL run the semantic and hybrid accuracy evals against the real
FastEmbed embedding model through the actual sidecar — never the fake provider —
so the committed semantic numbers are real. The semantic leg SHALL be gated behind
an explicit opt-in; when the opt-in is off or the model/sidecar is unavailable,
the harness MUST skip the semantic-only metrics with a recorded "skipped:
semantic unavailable" status rather than failing the whole run or silently
substituting a stub.

#### Scenario: Semantic accuracy uses the real model

- **WHEN** the semantic eval leg is enabled and the sidecar is provisioned
- **THEN** the harness embeds the corpus and queries with the real model and reports hybrid (RRF) discovery metrics alongside the lexical baseline

#### Scenario: Semantic leg skips cleanly when unavailable

- **WHEN** the semantic opt-in is off or the sidecar cannot be provisioned
- **THEN** the lexical and structural evals still run and the semantic-only metrics are recorded as skipped rather than failed

### Requirement: Performance benchmarks

The harness SHALL provide reproducible latency and throughput benchmarks for the
lexical ranker, the semantic query path, RRF fusion, and the end-to-end broker
`findTool` path, reporting representative percentiles (e.g. p50/p95) so retrieval
performance is tracked alongside accuracy.

#### Scenario: Search performance is benchmarked

- **WHEN** the performance benchmarks run against the corpus
- **THEN** the harness reports per-path latency percentiles and throughput for the lexical, semantic, fusion, and end-to-end paths

### Requirement: Structured run result and gate signal

A harness run SHALL emit a single structured result object capturing every
computed metric, the run provenance (corpus version, model, commit, timestamp,
and whether the semantic leg ran), and a pass/fail verdict computed against the
configured `eval-benchmarks` thresholds, so a run is usable both as a report and
as a CI gate.

#### Scenario: Run produces a machine-readable result

- **WHEN** a harness run completes
- **THEN** it emits a structured result containing all metrics, provenance, and an overall pass/fail verdict

#### Scenario: Gate failure is signalled

- **WHEN** a tracked metric falls below its configured threshold
- **THEN** the run result's verdict is fail and the process exit status is non-zero
