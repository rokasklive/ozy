## MODIFIED Requirements

### Requirement: Durable catalog store

Ozy SHALL persist the cataloged tools to durable local storage through the `catalog.Store` interface, replacing the in-memory placeholder, so the catalog is not lost when the process exits. The store SHALL support deleting tools by `toolRef` (batch), so reconciliation can remove entries for tools their servers no longer serve; without deletion, vanished tools remain selectable forever.

#### Scenario: Indexed tools are written to durable storage

- **WHEN** `ozy index` discovers and catalogs tools
- **THEN** the tools are written to durable local storage rather than only in-process memory

#### Scenario: Deleted tools are gone after restart

- **WHEN** reconciliation deletes a tool's catalog entry and the process restarts
- **THEN** the tool is absent from listings, `describeTool` returns `TOOL_NOT_FOUND` for its `toolRef`, and `findTool` cannot select it

## ADDED Requirements

### Requirement: Catalog exposes its age

The catalog SHALL expose the elapsed time since the last successful index run (derived from the recorded last-indexed timestamp), so response assembly can report catalog age to agents instead of presenting index-time snapshots as current truth.

#### Scenario: Age reflects the last successful index

- **WHEN** the catalog was last successfully indexed at a known time and a broker response is assembled
- **THEN** the catalog reports the elapsed seconds since that index run, and reports the never-indexed state distinctly rather than as age zero
