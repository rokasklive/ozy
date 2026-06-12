# catalog-persistence

## Purpose

Define durable catalog storage so indexed tool metadata survives process
restarts and is readable offline, without persisting configuration secrets.

## Requirements

### Requirement: Durable catalog store

Ozy SHALL persist the cataloged tools to durable local storage through the `catalog.Store` interface, replacing the in-memory placeholder, so the catalog is not lost when the process exits.

#### Scenario: Indexed tools are written to durable storage

- **WHEN** `ozy index` discovers and catalogs tools
- **THEN** the tools are written to durable local storage rather than only in-process memory

### Requirement: Catalog survives restarts

A freshly started Ozy process SHALL read the previously persisted catalog without re-running discovery.

#### Scenario: A new process sees previously indexed tools

- **WHEN** `ozy index` has populated the catalog and then a separate `ozy list` process runs
- **THEN** the second process lists the persisted tools without connecting to any downstream server

### Requirement: Offline catalog reads

`ozy list` and `describeTool` SHALL serve persisted catalog data even when the downstream servers are unreachable, with freshness/runtime status indicating the data may be stale (SPEC.md §4.4, §4.6).

#### Scenario: Describe works while the server is offline

- **WHEN** a tool was previously indexed and its downstream server is now unreachable
- **THEN** `ozy describe <toolRef>` still returns the cached schema, marked with its freshness/status, and does not claim the tool is callable solely on cached data

### Requirement: Catalog storage holds no secrets

The persisted catalog SHALL contain only tool capability metadata (refs, names, descriptions, schemas, freshness, status) and SHALL NOT store resolved secret values from configuration.

#### Scenario: Persisted catalog excludes secrets

- **WHEN** the catalog is written after indexing a server that uses secret headers or environment values
- **THEN** the persisted catalog contains no resolved secret values
