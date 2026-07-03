## ADDED Requirements

### Requirement: Per-server call timeout

Server configuration SHALL accept a `callTimeout` (milliseconds) bounding a single brokered `callTool` invocation end-to-end (connect plus execute), defaulting to 60000 when absent. `callTimeout` SHALL be independent of the existing discovery `timeout`, which continues to bound discovery/connection during indexing. The starter configuration SHALL document both knobs and their distinct purposes.

#### Scenario: Default call timeout applies when absent

- **WHEN** a server entry declares no `callTimeout`
- **THEN** brokered invocations for that server run under the 60-second default, and the server's discovery `timeout` (or its 5-second default) does not bound the invocation

#### Scenario: An explicit call timeout is honored

- **WHEN** a server entry declares `callTimeout: 180000`
- **THEN** brokered invocations for that server may run up to 180 seconds before Ozy's deadline fires

### Requirement: findTool result budget is load-bearing

`budgets.findTool.maxResults` SHALL bound the total candidates a `findTool` response surfaces (the selected tool plus alternatives), defaulting to 5 when absent. A configuration knob that exists SHALL affect behavior; scaffolding knobs that nothing reads is prohibited by this requirement's intent.

#### Scenario: maxResults bounds surfaced candidates

- **WHEN** `budgets.findTool.maxResults` is set to 3 and a query matches many cataloged tools
- **THEN** a `use` decision surfaces at most the selected tool and 2 alternatives
