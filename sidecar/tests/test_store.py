"""Tests for the SQLite-backed embedding metadata store."""

from __future__ import annotations

import os

import numpy as np
import pytest

from sidecar.store import META_BACKEND, META_DIM, META_MODEL, Store


def test_store_init_creates_schema(data_dir: str) -> None:
    db = os.path.join(data_dir, "embeddings.db")
    with Store(db) as s:
        assert s.get_meta("schema_version") == "1"
        # Tools and meta tables should be reachable even when empty.
        assert s.count_vectors() == 0
        # The on-disk file actually exists.
        assert os.path.exists(db)


def test_store_wal_mode_is_set(data_dir: str) -> None:
    db = os.path.join(data_dir, "embeddings.db")
    with Store(db) as s:
        cur = s._conn.execute("PRAGMA journal_mode")  # type: ignore[attr-defined]
        row = cur.fetchone()
        assert row[0].lower() == "wal"


def test_meta_get_set_roundtrip(store: Store) -> None:
    assert store.get_meta("missing") is None
    store.set_meta("answer", "42")
    assert store.get_meta("answer") == "42"
    store.set_meta("answer", "43")
    assert store.get_meta("answer") == "43"


def test_upsert_get_roundtrip(store: Store) -> None:
    vec = np.arange(8, dtype=np.float32)
    skipped, vector_id = store.upsert_one(
        tool_ref="a.b",
        text="hello",
        content_hash="hash-1",
        server_id="a",
        tags=["alpha", "beta"],
        vector=vec,
        model="fake-bge",
    )
    assert skipped is False
    assert vector_id == 0  # first id is 0

    row = store.get_by_toolref("a.b")
    assert row is not None
    assert row.tool_ref == "a.b"
    assert row.vector_id == 0
    assert row.content_hash == "hash-1"
    assert row.server_id == "a"
    assert row.tags == ["alpha", "beta"]
    assert row.model == "fake-bge"
    assert row.dim == 8
    np.testing.assert_array_equal(row.vector, vec)


def test_upsert_preserves_vector_id_when_changing(store: Store) -> None:
    vec1 = np.ones(4, dtype=np.float32)
    vec2 = np.full(4, 2.0, dtype=np.float32)
    _, vid1 = store.upsert_one(
        tool_ref="a.b",
        text="hello",
        content_hash="h1",
        server_id=None,
        tags=[],
        vector=vec1,
        model="m",
    )
    _, vid2 = store.upsert_one(
        tool_ref="a.b",
        text="hello2",
        content_hash="h2",
        server_id=None,
        tags=[],
        vector=vec2,
        model="m",
    )
    assert vid1 == vid2  # same tool_ref → same vector_id
    # And the vector is updated.
    row = store.get_by_toolref("a.b")
    assert row is not None
    np.testing.assert_array_equal(row.vector, vec2)
    assert row.content_hash == "h2"


def test_compute_skip_classifies(store: Store) -> None:
    vec = np.zeros(4, dtype=np.float32)
    assert store.compute_skip("a.b", "h1") == "new"
    store.upsert_one(
        tool_ref="a.b",
        text="x",
        content_hash="h1",
        server_id=None,
        tags=[],
        vector=vec,
        model="m",
    )
    assert store.compute_skip("a.b", "h1") == "unchanged"
    assert store.compute_skip("a.b", "h2") == "changed"


def test_upsert_skipped_when_unchanged(store: Store) -> None:
    vec = np.arange(4, dtype=np.float32)
    store.upsert_one(
        tool_ref="a.b",
        text="x",
        content_hash="h",
        server_id=None,
        tags=[],
        vector=vec,
        model="m",
    )
    skipped, vid = store.upsert_one(
        tool_ref="a.b",
        text="x",
        content_hash="h",
        server_id=None,
        tags=[],
        vector=vec,
        model="m",
    )
    assert skipped is True
    assert vid == 0


def test_get_by_vector_id_roundtrip(store: Store) -> None:
    vec = np.arange(4, dtype=np.float32)
    _, vid = store.upsert_one(
        tool_ref="a.b",
        text="x",
        content_hash="h",
        server_id=None,
        tags=[],
        vector=vec,
        model="m",
    )
    row = store.get_by_vector_id(vid)
    assert row is not None
    assert row.tool_ref == "a.b"


def test_vector_blob_preserves_float32(store: Store) -> None:
    rng = np.random.default_rng(42)
    vec = rng.standard_normal(16).astype(np.float32)
    store.upsert_one(
        tool_ref="a.b",
        text="x",
        content_hash="h",
        server_id=None,
        tags=[],
        vector=vec,
        model="m",
    )
    row = store.get_by_toolref("a.b")
    assert row is not None
    assert row.vector.dtype == np.float32
    assert row.vector.shape == (16,)
    np.testing.assert_array_equal(row.vector, vec)


def test_facet_allowlist_by_server_id(store: Store) -> None:
    v = np.zeros(4, dtype=np.float32)
    store.upsert_one(
        tool_ref="a.x", text="x", content_hash="1",
        server_id="atlassian", tags=[],
        vector=v, model="m",
    )
    store.upsert_one(
        tool_ref="a.y", text="x", content_hash="2",
        server_id="atlassian", tags=[],
        vector=v, model="m",
    )
    store.upsert_one(
        tool_ref="b.x", text="x", content_hash="3",
        server_id="github", tags=[],
        vector=v, model="m",
    )

    # No filter → all ids in insertion order.
    assert store.resolve_facet_allowlist() == [0, 1, 2]

    # serverId filter narrows to matching rows.
    assert store.resolve_facet_allowlist(server_id="atlassian") == [0, 1]
    assert store.resolve_facet_allowlist(server_id="github") == [2]
    assert store.resolve_facet_allowlist(server_id="missing") == []


def test_facet_allowlist_by_tags(store: Store) -> None:
    v = np.zeros(4, dtype=np.float32)
    store.upsert_one(
        tool_ref="a.x", text="x", content_hash="1",
        server_id=None, tags=["search", "wiki"],
        vector=v, model="m",
    )
    store.upsert_one(
        tool_ref="a.y", text="x", content_hash="2",
        server_id=None, tags=["search"],
        vector=v, model="m",
    )
    store.upsert_one(
        tool_ref="a.z", text="x", content_hash="3",
        server_id=None, tags=["page"],
        vector=v, model="m",
    )

    # Tag intersection: search → first two rows.
    assert store.resolve_facet_allowlist(tags=["search"]) == [0, 1]
    # Multi-tag (OR) union: search OR page → all three.
    assert sorted(store.resolve_facet_allowlist(tags=["search", "page"])) == [0, 1, 2]
    # Empty list is the same as "no filter".
    assert store.resolve_facet_allowlist(tags=[]) == [0, 1, 2]


def test_delete_many_returns_vector_ids(store: Store) -> None:
    v = np.zeros(4, dtype=np.float32)
    store.upsert_one(
        tool_ref="a.x", text="x", content_hash="1",
        server_id=None, tags=[], vector=v, model="m",
    )
    store.upsert_one(
        tool_ref="a.y", text="x", content_hash="2",
        server_id=None, tags=[], vector=v, model="m",
    )

    deleted = store.delete_many(["a.x", "a.y", "a.missing"])
    assert sorted(deleted) == [0, 1]
    assert store.count_vectors() == 0
    assert store.get_by_toolref("a.x") is None


def test_iter_all_yields_rows_in_vector_id_order(store: Store) -> None:
    v = np.zeros(4, dtype=np.float32)
    for i, ref in enumerate(["a", "b", "c"]):
        store.upsert_one(
            tool_ref=ref, text="x", content_hash=str(i),
            server_id=None, tags=[], vector=v, model="m",
        )
    rows = list(store.iter_all())
    assert [r.tool_ref for r in rows] == ["a", "b", "c"]
    assert [r.vector_id for r in rows] == [0, 1, 2]


def test_all_vector_ids_sorted(store: Store) -> None:
    v = np.zeros(4, dtype=np.float32)
    for ref in ["a", "b", "c"]:
        store.upsert_one(
            tool_ref=ref, text="x", content_hash=ref,
            server_id=None, tags=[], vector=v, model="m",
        )
    assert store.all_vector_ids() == [0, 1, 2]


def test_meta_helpers_use_known_keys(store: Store) -> None:
    store.set_meta(META_BACKEND, "turbovec")
    store.set_meta(META_MODEL, "BAAI/bge-small-en-v1.5")
    store.set_meta(META_DIM, "384")
    assert store.get_meta(META_BACKEND) == "turbovec"
    assert store.get_meta(META_MODEL) == "BAAI/bge-small-en-v1.5"
    assert store.get_meta(META_DIM) == "384"
