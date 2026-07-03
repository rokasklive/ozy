## 1. Runtime: reusable, non-blocking startup

- [x] 1.1 Extract the startup body of `daemon.Run` (`provisionSidecar` + `runStartupIndex` + readiness) into `Daemon.Start(ctx, log)` that returns once ready instead of blocking on `<-ctx.Done()`.
- [x] 1.2 Guard the daemon `broker` field with `atomic.Pointer[broker.Broker]`; make `Broker()` load it and `reWireBroker` store the semantic-wired broker atomically.
- [x] 1.3 Add a `BrokerProvider` interface (`Broker() broker.Broker`) satisfied by `*Daemon`.
- [x] 1.4 Test (extend `daemon_test.go`): `Start` wires the semantic provider when the sidecar is healthy, and leaves the lexical broker when semantic is disabled or the sidecar is unavailable.

## 2. MCP adapter and CLI self-bootstrap (shared `Start` path)

- [x] 2.1 Change `ozymcp.New` to take a `BrokerProvider` and read the broker per request rather than capturing it at construction (`commands.go:64`).
- [x] 2.2 In `mcpCmd`: launch `Daemon.Start(ctx, log)` in a background goroutine, serve immediately, and on context cancel / client EOF shut the sidecar down via `Daemon.Shutdown()`.
- [x] 2.3 Keep stdout pure JSON-RPC: route all provisioning/index/readiness output to the log file (and at most a concise stderr line), never stdout.
- [x] 2.4 Test: the adapter answers `findTool` from the lexical broker before warm-up and returns hybrid results after the provider swaps (fake provider + fake sidecar).
- [x] 2.5 Make `ozy search` provision-on-demand: run `Daemon.Start` (provision when semantic enabled + conditional index) before `FindTool`, then `Shutdown()` on exit. Leave `list`/`describe`/`call` on plain `load()` so they never provision the sidecar.
- [x] 2.6 Test: `ozy search` returns hybrid semantic results with no prior `index`/daemon (fake sidecar) and degrades to lexical when provisioning fails; assert `list`/`describe`/`call` do not provision.

## 3. Logging beside the configuration

- [x] 3.1 Add a `log/slog` JSON logger writing to `<dir(configPath)>/logs/ozy.log`; create the `logs/` dir; fall back to stderr-only if it is not writable, without blocking startup.
- [x] 3.2 Thread the logger from `load()` into the daemon so `Start`, provisioning, indexing, degradation, partial-embed, and shutdown emit structured records with an `action` field; reuse the existing secret `scrub` for any field carrying downstream detail.
- [x] 3.3 Replace the ad-hoc `status io.Writer` notices in `daemon.go` with `slog` calls (retain a concise human stderr line only where it aids interactive use).
- [x] 3.4 Test (table-driven): a degradation path writes a record naming cause and action; assert no configured secret value appears in any emitted record.

## 4. Coverage honesty

- [x] 4.1 Change the index loud-fail guard (`index.go:224`) to fire when `VectorCount < ToolsIndexed` (semantic enabled and sink available), with a message naming the tool and vector counts.
- [x] 4.2 `doctor`: cross-check the embedding probe's vector count against the catalog tool count; emit WARN with both counts and the `ozy index` remedy when short, OK when vectors ≥ tools.
- [x] 4.3 Update/extend `index_test.go` and the doctor tests for the new threshold and the cross-check WARN/OK cases.

## 5. Remove the daemon command and fix docs

- [x] 5.1 Remove `daemonCmd` and its registration in `cli.go`; delete the now-unused blocking `Run` wrapper (its startup logic lives in `Start`).
- [x] 5.2 Update README/quickstart to a single `ozy mcp` configuration step; drop all `ozy daemon` references; document the first-run cold model download and the `<config-dir>/logs/` location.
- [x] 5.3 Confirm `ozy --help` no longer lists `daemon` and help text matches the new surface.

## 6. Verify end-to-end

- [x] 6.1 `go build ./...` and `go test ./...` are green.
- [x] 6.2 Manual: `touch` the config to force staleness, run `ozy mcp`, issue a `findTool` over MCP, and confirm it auto-indexes + embeds and returns hybrid results with zero other commands; confirm logs appear under `<config-dir>/logs/`.
- [x] 6.3 Confirm the embedding sidecar process exits when the MCP connection closes (no orphan).
- [x] 6.4 Run `graphify update .` to refresh the knowledge graph.
