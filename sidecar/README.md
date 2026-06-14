# Ozy embedding sidecar

The Python worker that powers Ozy's hybrid (lexical + semantic) search. It
embeds cataloged tools with FastEmbed, stores the raw vectors plus facets in
SQLite, and serves nearest-neighbor queries out of a pluggable vector index
(`turbovec` by default, `faiss` as an opt-in). The Go daemon talks to it over
the worker's stdio using newline-delimited JSON; the worker never owns the
authoritative tool catalog (`SPEC.md` §10.4).

## Install

The sidecar is a regular Python package; the Go daemon normally provisions an
isolated environment for it, but the package also runs standalone.

```bash
# Default install — fastembed + turbovec. No FAISS, no GPU.
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt

# Opt-in FAISS backend.
pip install -r requirements-faiss.txt
```

Python 3.10+ is required.

## Run

```bash
python -m sidecar --data-dir ~/.local/state/ozy/sidecar --backend turbovec
```

The worker reads newline-delimited JSON requests on stdin and writes id-matched
JSON responses on stdout. Logs go to stderr.

Flags:

| Flag | Default | Description |
| --- | --- | --- |
| `--data-dir` | `$OZY_SIDECAR_STATE_DIR` or `~/.local/state/ozy/sidecar` | Where the SQLite DB and the persisted index live. |
| `--backend` | `turbovec` | Vector backend. `turbovec` (default) or `faiss`. |
| `--model` | `BAAI/bge-small-en-v1.5` | FastEmbed model id. The vector dimension is derived from the model. |
| `--required` | off (or `EMBEDDING_REQUIRED=1`) | Eagerly load the embedding model at startup so failures surface during `health`. |

## Operations

The protocol is one JSON object per line. Every request carries an `id`; every
response echoes that `id`.

| Op | Args | Response |
| --- | --- | --- |
| `health` | — | `{ok, model, dim, backend, vectorCount}` |
| `upsert` | `{items: [{toolRef, text, contentHash, serverId, tags}]}` | `{ok, upserted, skipped, errors: [...]}` |
| `delete` | `{toolRefs: [...]}` | `{ok, deleted}` |
| `query` | `{text, k, filter: {serverId, tags?}\|null}` | `{ok, hits: [{toolRef, score}]}` |
| `stats` | — | `{ok, backend, model, dim, vectorCount, toolCount}` |

Unknown or unparseable requests produce `{ok: false, error: "..."}` and the
worker keeps running.

## Architecture

```
        Go daemon (stdin/stdout)
                |
        sidecar/protocol.py  <-- framed JSONL dispatch
                |
        sidecar/ops.py       <-- per-op orchestration
          /     |      \
   embedder/  store/   vector/
   FastEmbed  SQLite   turbovec | faiss
```

`store.py` (SQLite) is the source of truth for embeddings: `toolRef ↔
vector_id`, content hashes, facets, the active backend/model/dim. The vector
index is a derived artifact that can always be rebuilt from SQLite without
re-embedding.

`vector.py` exposes a single `VectorBackend` interface so the `turbovec` and
`faiss` implementations are interchangeable. The active backend is recorded in
SQLite and is immutable after the first index — switching backends forces a
rebuild from SQLite rather than mixing incompatible artifacts.

## Development

```bash
pip install -r requirements.txt
pip install -r requirements-faiss.txt
pytest tests/ -v
```

Tests use a `FakeEmbedder` so they never download the real FastEmbed model.
Tests marked `@pytest.mark.slow` exercise the real model and are skipped by
default.
