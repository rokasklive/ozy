## ADDED Requirements

### Requirement: Catalog records the last successful index time

The catalog SHALL persist the timestamp of the last successful index run and expose it through the `catalog.Store` interface, so the daemon can compute catalog staleness against the configuration file's modification time on startup. The recorded time SHALL survive process restarts and SHALL be distinguishable from the absence of any prior index.

#### Scenario: A successful index run records its timestamp

- **WHEN** an index pass completes and writes tools to the catalog
- **THEN** the catalog persists the time of that successful index run

#### Scenario: Last index time is readable without re-discovery

- **WHEN** a freshly started Ozy process reads the catalog
- **THEN** it can read the persisted last successful index time without connecting to any downstream server

#### Scenario: Absence of a prior index is distinguishable

- **WHEN** the catalog has never completed a successful index run
- **THEN** reading the last successful index time reports that no prior index exists, rather than reporting a zero or misleading timestamp
