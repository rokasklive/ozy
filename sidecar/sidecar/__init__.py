"""Ozy embedding sidecar.

The sidecar is an stdio JSONL server that embeds cataloged tools with
FastEmbed, persists the raw vectors and facets in SQLite, and serves
nearest-neighbor queries through a pluggable vector index (turbovec by
default, FAISS as an opt-in). It is owned by the Go daemon and never touches
Ozy's authoritative tool catalog.

Package layout:

- :mod:`sidecar.embedder` — FastEmbed wrapper (with a ``FakeEmbedder`` for tests).
- :mod:`sidecar.store` — SQLite-backed embedding metadata store.
- :mod:`sidecar.vector` — ``VectorBackend`` interface + turbovec / FAISS impls.
- :mod:`sidecar.ops` — per-operation orchestration (health, upsert, ...).
- :mod:`sidecar.protocol` — newline-delimited JSON dispatch loop.
- :mod:`sidecar.__main__` — argparse entrypoint.
"""

from __future__ import annotations

__version__ = "0.1.0"
__all__ = ["__version__"]
