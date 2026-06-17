"""Per-operation orchestration tests.

Covers: health, upsert (skip-unchanged, embed-new, embed-changed), delete,
query (ordered toolRefs, facet filter), stats.
"""

from __future__ import annotations

import numpy as np
import pytest

from sidecar.embedder import FakeEmbedder
from sidecar.ops import Ops
from sidecar.store import META_BACKEND, META_DIM, META_MODEL, Store
from sidecar.vector import TurbovecBackend


def test_health_returns_model_info(ops: Ops) -> None:
    resp = ops.op_health({})
    assert resp["ok"] is True
    assert resp["model"] == "fake-bge-small"
    assert resp["dim"] == 384
    assert resp["backend"] == "turbovec"
    assert resp["vectorCount"] == 0


def test_warmup_returns_readiness_payload(ops: Ops) -> None:
    # The readiness probe loads the model (no-op for the fake), runs a probe
    # query, and reports the same shape as health.
    resp = ops.op_warmup({})
    assert resp["ok"] is True
    assert resp["model"] == "fake-bge-small"
    assert resp["dim"] == 384
    assert resp["backend"] == "turbovec"
    assert resp["vectorCount"] == 0


def test_stats_returns_counts(ops: Ops) -> None:
    resp = ops.op_stats({})
    assert resp["ok"] is True
    assert resp["backend"] == "turbovec"
    assert resp["model"] == "fake-bge-small"
    assert resp["dim"] == 384
    assert resp["vectorCount"] == 0
    assert resp["toolCount"] == 0


def test_upsert_embeds_new_tool(ops: Ops) -> None:
    resp = ops.op_upsert({
        "items": [
            {
                "toolRef": "test.tool",
                "text": "semantic search",
                "contentHash": "hash-abc",
                "serverId": "srv1",
                "tags": ["search"],
            }
        ]
    })
    assert resp["ok"] is True
    assert resp["upserted"] == 1
    assert resp["skipped"] == 0
    assert resp["errors"] == []

    # Verify it's stored.
    row = ops.store.get_by_toolref("test.tool")
    assert row is not None
    assert row.tool_ref == "test.tool"
    assert row.content_hash == "hash-abc"
    assert row.server_id == "srv1"
    assert row.tags == ["search"]
    assert row.vector.shape == (384,)


def test_upsert_skips_unchanged_by_content_hash(ops: Ops) -> None:
    args = {
        "items": [
            {
                "toolRef": "test.tool",
                "text": "same text",
                "contentHash": "same-hash",
                "serverId": "srv1",
                "tags": [],
            }
        ]
    }
    r1 = ops.op_upsert(args)
    assert r1["upserted"] == 1
    assert r1["skipped"] == 0

    # Same contentHash → skip.
    r2 = ops.op_upsert(args)
    assert r2["upserted"] == 0
    assert r2["skipped"] == 1

    # Verify only one row exists.
    assert ops.store.count_vectors() == 1


def test_upsert_embeds_changed_tool(ops: Ops) -> None:
    ops.op_upsert({
        "items": [
            {
                "toolRef": "test.tool",
                "text": "initial text",
                "contentHash": "hash-1",
                "serverId": "srv1",
                "tags": [],
            }
        ]
    })
    # Changed contentHash → re-embed and update.
    r2 = ops.op_upsert({
        "items": [
            {
                "toolRef": "test.tool",
                "text": "updated text",
                "contentHash": "hash-2",
                "serverId": "srv1",
                "tags": ["updated"],
            }
        ]
    })
    assert r2["upserted"] == 1
    assert r2["skipped"] == 0

    row = ops.store.get_by_toolref("test.tool")
    assert row is not None
    assert row.content_hash == "hash-2"
    assert row.tags == ["updated"]


def test_upsert_handles_multiple_items(ops: Ops) -> None:
    resp = ops.op_upsert({
        "items": [
            {"toolRef": "a.x", "text": "t1", "contentHash": "h1", "serverId": None, "tags": []},
            {"toolRef": "a.y", "text": "t2", "contentHash": "h2", "serverId": None, "tags": []},
            {"toolRef": "a.z", "text": "t3", "contentHash": "h3", "serverId": None, "tags": []},
        ]
    })
    assert resp["upserted"] == 3
    assert resp["skipped"] == 0
    assert ops.store.count_vectors() == 3


def test_upsert_errors_on_invalid_items(ops: Ops) -> None:
    resp = ops.op_upsert({
        "items": [
            "not-an-object",
            {"toolRef": "", "text": "x", "contentHash": "h", "serverId": None, "tags": []},
            {"toolRef": "ok.tool", "text": "x", "contentHash": "h", "serverId": None, "tags": []},
        ]
    })
    assert resp["ok"] is True  # partial success
    assert resp["upserted"] == 1
    assert len(resp["errors"]) == 2


def test_upsert_rejects_missing_items_key(ops: Ops) -> None:
    resp = ops.op_upsert({})
    assert resp["ok"] is False
    assert "items must be a list" in resp["error"]


def test_delete_removes_tools(ops: Ops) -> None:
    ops.op_upsert({
        "items": [
            {"toolRef": "a.x", "text": "x", "contentHash": "h1", "serverId": None, "tags": []},
            {"toolRef": "a.y", "text": "y", "contentHash": "h2", "serverId": None, "tags": []},
        ]
    })
    resp = ops.op_delete({"toolRefs": ["a.x", "nonexistent"]})
    assert resp["ok"] is True
    assert resp["deleted"] == 1
    assert ops.store.get_by_toolref("a.x") is None
    assert ops.store.get_by_toolref("a.y") is not None


def test_delete_empty_list(ops: Ops) -> None:
    resp = ops.op_delete({"toolRefs": []})
    assert resp["ok"] is True
    assert resp["deleted"] == 0


def test_delete_rejects_missing_tool_refs(ops: Ops) -> None:
    resp = ops.op_delete({})
    assert resp["ok"] is False
    assert "toolRefs must be a list" in resp["error"]


def test_query_returns_ordered_hits(ops: Ops) -> None:
    # Insert tools with distinct texts.
    ops.op_upsert({
        "items": [
            {"toolRef": "a.search", "text": "search for documents",
             "contentHash": "h1", "serverId": None, "tags": []},
            {"toolRef": "b.notify", "text": "send notification email",
             "contentHash": "h2", "serverId": None, "tags": []},
            {"toolRef": "c.find", "text": "find and retrieve data",
             "contentHash": "h3", "serverId": None, "tags": []},
        ]
    })

    # Query with the exact text of the first tool so it matches exactly.
    resp = ops.op_query({"text": "search for documents", "k": 3})
    assert resp["ok"] is True
    hits = resp["hits"]
    assert len(hits) == 3
    assert all("toolRef" in h and "score" in h for h in hits)
    # The first hit should be the exact text match (cosine ≈ 1.0).
    assert hits[0]["toolRef"] == "a.search"
    assert hits[0]["score"] > 0.99


def test_query_with_facet_filter(ops: Ops) -> None:
    ops.op_upsert({
        "items": [
            {"toolRef": "srv1.a", "text": "search tool",
             "contentHash": "h1", "serverId": "srv1", "tags": []},
            {"toolRef": "srv2.b", "text": "search engine",
             "contentHash": "h2", "serverId": "srv2", "tags": []},
        ]
    })

    # Filter to srv1 only.
    resp = ops.op_query({"text": "search", "k": 5, "filter": {"serverId": "srv1"}})
    assert resp["ok"] is True
    assert len(resp["hits"]) == 1
    assert resp["hits"][0]["toolRef"] == "srv1.a"


def test_query_facet_filter_no_match(ops: Ops) -> None:
    ops.op_upsert({
        "items": [
            {"toolRef": "srv1.a", "text": "search tool",
             "contentHash": "h1", "serverId": "srv1", "tags": []},
        ]
    })

    resp = ops.op_query({"text": "search", "k": 5, "filter": {"serverId": "srv99"}})
    assert resp["ok"] is True
    assert resp["hits"] == []


def test_query_with_k_cap(ops: Ops) -> None:
    ops.op_upsert({
        "items": [
            {"toolRef": f"tool.{i}", "text": f"tool number {i}",
             "contentHash": f"h{i}", "serverId": None, "tags": []}
            for i in range(5)
        ]
    })

    resp = ops.op_query({"text": "tool", "k": 2})
    assert resp["ok"] is True
    assert len(resp["hits"]) == 2


def test_query_with_zero_k(ops: Ops) -> None:
    resp = ops.op_query({"text": "search", "k": 0})
    assert resp["ok"] is True
    assert resp["hits"] == []


def test_query_missing_text(ops: Ops) -> None:
    resp = ops.op_query({})
    assert resp["ok"] is False
    assert "text required" in resp["error"]
