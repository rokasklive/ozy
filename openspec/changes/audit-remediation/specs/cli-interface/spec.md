## ADDED Requirements

### Requirement: Doctor flags inline secret-shaped values

`ozy doctor` SHALL warn when a configured server header or environment value that does not use an `{env:…}` reference matches a known secret pattern (at minimum: `ghp_`, `github_pat_`, `sk-`, `AKIA`, `xox` token prefixes, and `Bearer ` values). The warning SHALL name the server, the key, and the matched pattern kind, and SHALL NOT print any part of the value. Values using `{env:…}` references SHALL NOT be flagged.

#### Scenario: A plaintext token is flagged without being printed

- **WHEN** a server's `environment` contains a literal value beginning with `ghp_` and the user runs `ozy doctor`
- **THEN** doctor emits a warning naming the server, the key, and that a GitHub token pattern was found inline, recommending an `{env:…}` reference — and no part of the token value appears in any output format

#### Scenario: Env-referenced secrets are not flagged

- **WHEN** a server's header value is `Bearer {env:ATLASSIAN_MCP_TOKEN}` and the user runs `ozy doctor`
- **THEN** no inline-secret warning is emitted for that value
