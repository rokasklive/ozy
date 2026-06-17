## ADDED Requirements

### Requirement: Sidecar readiness requires a loaded model and queryable vectors

Ozy SHALL treat the sidecar as "available" only when its embedding model is
loaded and a probe query returns successfully — not merely when the process has
started or answered a liveness ping. Sidecar startup SHALL distinguish a fast
liveness check from a one-time model warm-up: the warm-up (which may trigger a
cold model download) SHALL be given a timeout generous enough to complete the
download, and a liveness or health timeout SHALL NOT abort an in-progress model
download or kill the process while it is downloading the model. If the model
cache is partial or corrupt, Ozy SHALL detect it and re-fetch once before
declaring semantic search unavailable.

#### Scenario: Cold model warm-up is not aborted by the liveness timeout

- **WHEN** semantic search is enabled and the model has not been downloaded yet,
  so the first warm-up must fetch it
- **THEN** Ozy allows the warm-up to run under a generous timeout and does not
  terminate the sidecar mid-download because a short liveness or health deadline
  elapsed

#### Scenario: Availability means a query succeeds

- **WHEN** Ozy reports semantic search available
- **THEN** the embedding model is loaded and a probe query against the vector
  store returns, so "available" cannot be true while zero vectors are queryable

#### Scenario: Partial or corrupt model cache is re-fetched

- **WHEN** a previous start left an incomplete or corrupt model cache and the
  sidecar is started again
- **THEN** Ozy detects the bad cache, clears it, and re-fetches the model once
  rather than repeatedly failing to start

## MODIFIED Requirements

### Requirement: Graceful degradation when the sidecar is unavailable

Ozy SHALL treat the embedding sidecar as optional at every step: if Python is absent, provisioning fails, the sidecar cannot start, it crashes, or a request exceeds its timeout, the daemon SHALL mark semantic search unavailable, continue serving `findTool` from the lexical baseline, and surface the degraded mode rather than returning an error (`SPEC.md` §4.10, §10.1). The surfaced degraded mode SHALL name the specific reason (for example missing Python, provisioning failure, incomplete model download, or health-check failure) and the next command to run, so the user is never left with a silent lexical-only fallback.

#### Scenario: Missing Python degrades to lexical-only

- **WHEN** semantic search is enabled but no usable Python runtime or sidecar environment can be provisioned
- **THEN** the daemon starts, `findTool` returns lexical-ranked results, and the response surfaces that semantic search is unavailable

#### Scenario: Sidecar crash mid-session does not fail findTool

- **WHEN** the sidecar process exits or a `query` times out during operation
- **THEN** `findTool` falls back to lexical ranking for that query and surfaces degraded mode rather than propagating a failure

#### Scenario: Degraded mode states the reason and the next step

- **WHEN** semantic search is enabled but unavailable for any reason
- **THEN** the degraded notice names the specific cause and the command to run to repair or retry (for example `ozy doctor` or `ozy index`), rather than only saying "lexical-only"
