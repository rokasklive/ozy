# Tool corpus

A checked-in estate of realistic MCP server/tool definitions that both bench
modes face: **504 tools across 40 servers**. Direct mode advertises them all;
ozy indexes them all. The corpus turns retrieval reliability into a measured
quantity — the agent must resolve *the* right tool out of hundreds, not the right
neighborhood.

## Layout

- `<server>.json` — one file per fixture server: `{server, mirrorSource, tools[]}`,
  each tool `{name, description, inputSchema, behavior?}`. Every schema property is
  typed and described.
- `gen.go` + `gen_data.go` (`//go:build ignore`) — the authoring source. The JSON
  files are the reviewable artifact; regenerate with:

  ```sh
  go run bench/corpus/gen.go bench/corpus/gen_data.go
  ```

Served by the generic corpus mode (`ozy-bench mcp --server <name> --corpus-dir
bench/corpus`), which routes each call by the tool's `behavior` so a
same-capability rival behaves like the real service it mirrors:

- **`stub`** (default, empty) — deterministic, schema-plausible response that never
  carries a scenario ground-truth fact (the unrelated-capability estate).
- **`auth_error`** — a realistic `authentication required` error for auth-gated
  services a keyless agent cannot use, so the pick fails legibly.
- **`functional:<backend>`** — real data via a shared backend (`search` ranks the
  fixture corpus; `pdf` writes a PDF to the agent workspace) for no-auth
  same-capability rivals that would really complete the task. The behavior policy
  lives in `gen.go`'s `toolBehavior`.

## Distractor families (mandatory near-misses)

Aligned against the `historical-weather-report` scenario's canonical tools, so
retrieval must pick the exact tool. Each rival's `behavior` matches the real
service (auth-gated → `auth_error`; no-auth same-capability → `functional`):

- **Rival search** (vs `duckduckgo`): `brave-search`, `bing-search`, `kagi-search`
  — all API-key/subscription gated → `auth_error`.
- **Weather/climate lookalikes** (vs `weather`): `climate-analytics` (keyed climate
  APIs → `auth_error`), `aviation-weather`, `air-quality-index` (no-auth but a
  different sub-capability → `stub`).
- **Document/PDF rivals** (vs `pdf-toolkit`): `doc-converter`
  (`convert_html_to_pdf` → `functional:pdf`), `office-export` (`export_to_pdf` →
  `functional:pdf`), `esign-service` (different capability → `stub`).

## Content review checklist (task 2.5)

Enforced structurally by `TestCorpusFloor`, `TestCorpusDistractorFamilies`,
`TestCorpusHasNoScenarioFacts`, and reviewed by hand:

- [x] ≥ 500 tools, ≥ 25 servers.
- [x] Real servers mirrored where documented (github, slack, jira, stripe,
      kubernetes, datadog, notion, linear, sentry, gdrive, postgres, gitlab,
      mongodb, elasticsearch, twilio, sendgrid, pagerduty, grafana, confluence,
      salesforce, hubspot, zendesk, redis, bigquery, snowflake, okta, shopify,
      vault, cloudflare, airtable, aws-s3); synthesized servers labelled
      `synthesized`.
- [x] Descriptions are production tone, 1–2 sentences, no copy-paste repetition.
- [x] Every schema property typed and described; tool names unique per server.
- [x] Each distractor family is genuinely tempting (e.g. `brave-search.web_search`,
      `climate-analytics.get_temperature_anomaly`, `doc-converter.convert_html_to_pdf`).
- [x] No corpus tool leaks the scenario's ground-truth facts.
