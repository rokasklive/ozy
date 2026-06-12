## ADDED Requirements

### Requirement: Connect to local (stdio) servers

Ozy SHALL connect to each enabled `mcp` server of `type: local` by launching its `command` (with arguments and `environment`) and speaking MCP over the process's stdio, using the MCP client connector.

#### Scenario: Local server connects and initializes

- **WHEN** Ozy connects to an enabled `local` server whose command starts successfully
- **THEN** an initialized MCP client session is established for that server

#### Scenario: Local command environment is applied

- **WHEN** a `local` server declares `environment` values (after reference resolution)
- **THEN** the launched process receives those environment variables

### Requirement: Connect to remote (HTTP) servers

Ozy SHALL connect to each enabled `mcp` server of `type: remote` by opening an HTTP MCP transport to its `url`, sending the configured `headers` on requests.

#### Scenario: Remote server connects with headers

- **WHEN** Ozy connects to an enabled `remote` server
- **THEN** it establishes an MCP session to the `url` and includes the configured (resolved) `headers` on its requests

### Requirement: Per-server isolation

Connecting to downstream servers SHALL be isolated per server: a server that is disabled is skipped, and a server that fails to connect SHALL NOT prevent connection to or discovery of the other servers. Each failure SHALL be captured as a structured, per-server error.

#### Scenario: Disabled servers are skipped

- **WHEN** an `mcp` entry has `enabled: false`
- **THEN** Ozy does not attempt to connect to it

#### Scenario: One unreachable server does not abort the others

- **WHEN** one enabled server is unreachable while others are reachable
- **THEN** Ozy records a structured error for the failing server and still connects to and proceeds with the reachable servers

### Requirement: Non-leaky connection errors

Connection errors SHALL be structured and SHALL NOT expose resolved secret values from `headers` or `environment`.

#### Scenario: Connection failure error is redacted

- **WHEN** a connection attempt fails for a server that uses a secret header or environment value
- **THEN** the reported error names the server and reason without including the resolved secret value
