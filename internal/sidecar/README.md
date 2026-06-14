# internal/sidecar — Go sidecar client

Go client for Ozy's optional Python embedding sidecar. The sidecar is a child
process that the daemon spawns and speaks to over its standard input and output
using newline-delimited JSON (JSONL).

## Architecture

```
┌──────────┐  stdin (JSONL)  ┌──────────┐
│  Client  │ ───────────────→│  Sidecar │
│  (Go)    │←─────────────── │ (Python) │
└──────────┘  stdout (JSONL) └──────────┘
                   stderr → logger
```

- **One Client = one process.** Each `Client` owns exactly one sidecar
  subprocess.
- **One in-flight request at a time.** A `sync.Mutex` serialises writes;
  a single reader goroutine dispatches responses by request ID.
- **Graceful degradation.** Any failure (timeout, crash, garbage response,
  provisioning failure) marks the client unavailable without panicking.
  The search engine falls back to the lexical baseline.

## JSONL protocol

One JSON object per line, stdin to sidecar, stdout from sidecar.

### Request

```json
{"id": "<id>", "op": "<op>", ...args}
```

### Response

```json
{"id": "<id>", "ok": true, ...payload}
{"id": "<id>", "ok": false, "error": "..."}
```

Operations: `health`, `upsert`, `delete`, `query`, `stats`. See `protocol.go`
for the exact field names.

## Files

| File | Purpose |
|------|---------|
| `protocol.go` | JSONL wire types (request/response structs) |
| `client.go` | `Client` struct and public API (`Health`, `Upsert`, `Delete`, `Query`, `Stats`, `Close`) |
| `process.go` | Subprocess management (`os/exec`, stderr drain, `Logger` interface) |
| `provision.go` | On-demand venv provisioning via `uv` (fallback `python -m venv` + `pip`) |
| `semantic.go` | `SemanticAdapter` implementing `search.Semantic` |
| `fake.go` | Fake driver for tests (`ScriptedSidecar`, configurable JSONL responses) |

## Concurrency

- `inFlight` mutex: one request on the wire at a time.
- `pending` map + mutex: reader goroutine dispatches by `id`.
- `healthy` atomic: sticky health flag; `Health` can re-probe.
- `closed` atomic: idempotent `Close()`.

## Testing

```bash
go test ./internal/sidecar/...
```

Tests use the `ScriptedSidecar` fake driver to simulate:
- Happy paths (health, upsert, delete, query, stats)
- Request timeout (scripted delay + short context)
- Sidecar exit mid-session
- Garbage/non-JSON responses
- Malformed responses (missing `id`, etc.)

The provision tests mock `LookPath` and `Runner` to exercise the toolchain
resolution order and `ErrNoToolchain` without requiring a real Python
installation.
