## ADDED Requirements

### Requirement: Capability breadcrumb opt-out

Ozy's configuration SHALL support a `surface.capabilityBreadcrumb` boolean that controls whether the `findTool` description advertises a breadcrumb of available downstream servers. The setting SHALL default to enabled when the `surface` section or the field is omitted, and setting it to `false` SHALL disable the breadcrumb. The resolved configuration SHALL expose the effective value to the MCP adapter.

#### Scenario: Breadcrumb defaults to enabled when omitted

- **WHEN** Ozy loads a configuration file that omits the `surface` section or the `capabilityBreadcrumb` field
- **THEN** the resolved configuration reports the capability breadcrumb as enabled

#### Scenario: Breadcrumb can be disabled explicitly

- **WHEN** Ozy loads a configuration file with `surface.capabilityBreadcrumb` set to `false`
- **THEN** the resolved configuration reports the capability breadcrumb as disabled and the `findTool` description omits the server summary
