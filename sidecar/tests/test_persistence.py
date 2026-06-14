"""End-to-end persistence tests.

Covers: upsert → write index → instantiate new Store/Backend pair →
verify index loads from disk and queries return correct hits.
"""

from __future__ import annotations

import os

import numpy as np
import pytest

from sidecar.embedder import FakeEmbedder
from sidecar.ops import Ops, load_or_rebuild
from sidecar.store import META_BACKEND, META_DIM, META_MODEL, Store
from sidecar.vector import TurbovecBackend, index_path


def _embedder_for_dim(dim: int) -> FakeEmbedder:
    return FakeEmbedder(model="test-model", dim=dim, separate_query_space=False)


# ------------------------------------------------------------------ turbovec e2e


def test_turbovec_end_to_end_persistence(data_dir: str) -> None:
    """Upsert tools, persist, reload with new Store/Backend, query."""
    model = "test-model"
    dim = 64
    embedder = _embedder_for_dim(dim)
    db_path = os.path.join(data_dir, "embeddings.db")

    # Phase 1: upsert and persist.
    store1 = Store(db_path)
    backend1 = TurbovecBackend(dim=dim)
    ops1 = Ops(
        embedder=embedder,
        store=store1,
        backend=backend1,
        backend_name="turbovec",
        data_dir=data_dir,
    )

    ops1.op_upsert({
        "items": [
            {
                "toolRef": "srv1.search",
                "text": "search for documents and files",
                "contentHash": "h1",
                "serverId": "srv1",
                "tags": ["search"],
            },
            {
                "toolRef": "srv1.notify",
                "text": "send email notifications to users",
                "contentHash": "h2",
                "serverId": "srv1",
                "tags": ["notify"],
            },
            {
                "toolRef": "srv2.search",
                "text": "web search engine query",
                "contentHash": "h3",
                "serverId": "srv2",
                "tags": ["search"],
            },
        ]
    })
    assert store1.count_vectors() == 3
    backend1.write(index_path(data_dir, "turbovec"))

    # Record meta so reload knows the config.
    store1.set_meta(META_BACKEND, "turbovec")
    store1.set_meta(META_MODEL, model)
    store1.set_meta(META_DIM, str(dim))
    store1.close()

    # Phase 2: reload with new instances.
    store2 = Store(db_path)
    backend2 = load_or_rebuild(
        store2,
        backend_name="turbovec",
        model=model,
        dim=dim,
        data_dir=data_dir,
    )
    ops2 = Ops(
        embedder=embedder,
        store=store2,
        backend=backend2,
        backend_name="turbovec",
        data_dir=data_dir,
    )

    # Query with exact text of srv1.search so it ranks first.
    resp = ops2.op_query({"text": "search for documents and files", "k": 5})
    assert resp["ok"] is True
    hits = resp["hits"]
    assert len(hits) >= 2
    tool_refs = [h["toolRef"] for h in hits]
    assert "srv1.search" in tool_refs
    assert "srv2.search" in tool_refs
    # Exact text match should be top hit.
    assert hits[0]["toolRef"] == "srv1.search"
    assert hits[0]["score"] > 0.99

    store2.close()


def test_turbovec_facet_filter_after_reload(data_dir: str) -> None:
    """Facet filter still works after reloading the index from disk."""
    model = "test-model"
    dim = 64
    embedder = _embedder_for_dim(dim)
    db_path = os.path.join(data_dir, "embeddings.db")

    # Phase 1: upsert.
    store1 = Store(db_path)
    backend1 = TurbovecBackend(dim=dim)
    ops1 = Ops(
        embedder=embedder,
        store=store1,
        backend=backend1,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    ops1.op_upsert({
        "items": [
            {
                "toolRef": "a.x",
                "text": "search documents efficiently",
                "contentHash": "h1",
                "serverId": "server-a",
                "tags": [],
            },
            {
                "toolRef": "b.x",
                "text": "find files and documents",
                "contentHash": "h2",
                "serverId": "server-b",
                "tags": [],
            },
        ]
    })
    backend1.write(index_path(data_dir, "turbovec"))
    store1.set_meta(META_BACKEND, "turbovec")
    store1.set_meta(META_MODEL, model)
    store1.set_meta(META_DIM, str(dim))
    store1.close()

    # Phase 2: reload.
    store2 = Store(db_path)
    backend2 = load_or_rebuild(
        store2,
        backend_name="turbovec",
        model=model,
        dim=dim,
        data_dir=data_dir,
    )
    ops2 = Ops(
        embedder=embedder,
        store=store2,
        backend=backend2,
        backend_name="turbovec",
        data_dir=data_dir,
    )

    # Filtered query after reload.
    resp = ops2.op_query({
        "text": "search documents",
        "k": 5,
        "filter": {"serverId": "server-a"},
    })
    assert resp["ok"] is True
    assert len(resp["hits"]) == 1
    assert resp["hits"][0]["toolRef"] == "a.x"

    store2.close()


def test_skip_unchanged_survives_reload(data_dir: str) -> None:
    """After reload, re-upsert with same contentHash skips re-embedding."""
    model = "test-model"
    dim = 64
    embedder = _embedder_for_dim(dim)
    db_path = os.path.join(data_dir, "embeddings.db")

    store1 = Store(db_path)
    backend1 = TurbovecBackend(dim=dim)
    ops1 = Ops(
        embedder=embedder,
        store=store1,
        backend=backend1,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    ops1.op_upsert({
        "items": [
            {
                "toolRef": "t.x",
                "text": "some text",
                "contentHash": "hash-42",
                "serverId": None,
                "tags": [],
            }
        ]
    })
    backend1.write(index_path(data_dir, "turbovec"))
    store1.set_meta(META_BACKEND, "turbovec")
    store1.set_meta(META_MODEL, model)
    store1.set_meta(META_DIM, str(dim))
    store1.close()

    # Reload everything.
    store2 = Store(db_path)
    backend2 = load_or_rebuild(
        store2,
        backend_name="turbovec",
        model=model,
        dim=dim,
        data_dir=data_dir,
    )
    ops2 = Ops(
        embedder=embedder,
        store=store2,
        backend=backend2,
        backend_name="turbovec",
        data_dir=data_dir,
    )

    # Re-upsert with same hash — should skip.
    resp = ops2.op_upsert({
        "items": [
            {
                "toolRef": "t.x",
                "text": "some text",
                "contentHash": "hash-42",
                "serverId": None,
                "tags": [],
            }
        ]
    })
    assert resp["upserted"] == 0
    assert resp["skipped"] == 1

    store2.close()


def test_rebuild_from_sqlite_preserves_data(data_dir: str) -> None:
    """When the persisted index is deleted, rebuild from SQLite recovers all vectors."""
    model = "test-model"
    dim = 64
    embedder = _embedder_for_dim(dim)
    db_path = os.path.join(data_dir, "embeddings.db")

    store1 = Store(db_path)
    backend1 = TurbovecBackend(dim=dim)
    ops1 = Ops(
        embedder=embedder,
        store=store1,
        backend=backend1,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    ops1.op_upsert({
        "items": [
            {
                "toolRef": f"tool.{i}",
                "text": f"tool number {i} for searching",
                "contentHash": f"h{i}",
                "serverId": None,
                "tags": [],
            }
            for i in range(10)
        ]
    })
    backend1.write(index_path(data_dir, "turbovec"))
    store1.set_meta(META_BACKEND, "turbovec")
    store1.set_meta(META_MODEL, model)
    store1.set_meta(META_DIM, str(dim))
    store1.close()

    # Delete the persisted index file.
    os.remove(index_path(data_dir, "turbovec"))

    # Rebuild from SQLite.
    store2 = Store(db_path)
    backend2 = load_or_rebuild(
        store2,
        backend_name="turbovec",
        model=model,
        dim=dim,
        data_dir=data_dir,
    )

    # Query should still return results.
    ops2 = Ops(
        embedder=embedder,
        store=store2,
        backend=backend2,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    resp = ops2.op_query({"text": "tool 5", "k": 5})
    assert resp["ok"] is True
    assert len(resp["hits"]) >= 1

    store2.close()
