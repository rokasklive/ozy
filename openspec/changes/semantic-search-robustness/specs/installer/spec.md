## MODIFIED Requirements

### Requirement: Python and asset provisioning with graceful degradation

The SetupPythonEnvironment and DownloadEmbeddingAssets steps SHALL reuse the
existing sidecar provisioner (uv → python3 managed venv, pinned dependencies,
marker-cached) under a Ozy-managed directory. The DownloadEmbeddingAssets step
SHALL actively fetch and verify the embedding model during install — rather than
deferring the download to first query — using a timeout generous enough for a
cold model download, and SHALL confirm the model loads and produces a vector
before reporting semantic available. If a previous attempt left a partial or
corrupt model cache, the step SHALL detect it and re-fetch once before failing.
When no usable Python toolchain exists, the installer SHALL NOT fail the install;
it SHALL continue in lexical-only mode and report that mode clearly. When Python
is present but the model cannot be downloaded or verified, the step SHALL surface
an actionable error (per "Actionable errors") instead of reporting silent
success, and the overall install MAY still complete in lexical-only mode.

#### Scenario: Toolchain present

- **WHEN** a usable Python toolchain is available and the user consents
- **THEN** the managed venv is provisioned via the existing provisioner, the
  embedding model is downloaded and verified, and semantic mode is reported
  available

#### Scenario: Toolchain absent degrades, not fails

- **WHEN** no Python toolchain is available
- **THEN** the step completes in lexical-only mode, the install still succeeds,
  and `ozy doctor` reports lexical-only

#### Scenario: Model is downloaded during install, not deferred

- **WHEN** DownloadEmbeddingAssets runs with a provisioned venv
- **THEN** the embedding model is fetched and loaded during this step, so the
  first semantic query does not trigger a cold download

#### Scenario: Partial or corrupt model download is recovered

- **WHEN** a prior run left an incomplete or corrupt model in the cache and the
  step runs again
- **THEN** the step detects the bad cache, clears it, and re-fetches the model
  once before reporting status

#### Scenario: Model provisioning failure is actionable

- **WHEN** Python is present but the embedding model cannot be downloaded or
  verified
- **THEN** the step reports an actionable error naming the failure, whether retry
  is safe, and the next command — and does not report semantic available
