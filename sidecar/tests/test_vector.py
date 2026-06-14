"""Vector backend tests.

Covers: turbovec default, FAISS construction, allowlist-filtered search,
persistence survive reload, and backend/model mismatch triggers rebuild.
"""

from __future__ import annotations

import os

import numpy as np
import pytest

from sidecar.embedder import FakeEmbedder
from sidecar.ops import load_or_rebuild
from sidecar.store import META_BACKEND, META_DIM, META_MODEL, Store
from sidecar.vector import (
    FaissBackend,
    TurbovecBackend,
    VectorBackend,
    file_ext_for,
    index_path,
    make_backend,
)


def _unit_vecs(n: int, dim: int, seed: int = 42) -> np.ndarray:
    """Generate n unit-normalised float32 vectors of shape (n, dim)."""
    rng = np.random.default_rng(seed)
    v = rng.standard_normal((n, dim)).astype(np.float32)
    norms = np.linalg.norm(v, axis=1, keepdims=True)
    return v / norms


# ------------------------------------------------------------------ factory


def test_make_backend_turbovec() -> None:
    backend = make_backend("turbovec", dim=16)
    assert isinstance(backend, TurbovecBackend)
    assert backend.dim == 16


def test_make_backend_faiss() -> None:
    backend = make_backend("faiss", dim=16)
    assert isinstance(backend, FaissBackend)
    assert backend.dim == 16


def test_make_backend_unknown_raises() -> None:
    with pytest.raises(ValueError, match="unknown vector backend"):
        make_backend("bad", dim=16)


def test_file_ext_for() -> None:
    assert file_ext_for("turbovec") == ".tvim"
    assert file_ext_for("faiss") == ".faiss"
    with pytest.raises(ValueError):
        file_ext_for("unknown")


def test_index_path(data_dir: str) -> None:
    assert index_path(data_dir, "turbovec") == os.path.join(data_dir, "index.tvim")
    assert index_path(data_dir, "faiss") == os.path.join(data_dir, "index.faiss")


# ------------------------------------------------------------------ turbovec


def test_turbovec_add_and_search(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = TurbovecBackend(dim=dim)

    vectors = _unit_vecs(10, dim)
    ids = np.arange(10, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)
    assert backend.count == 10

    query = vectors[0].reshape(1, -1)
    scores, result_ids = backend.search(query, k=3)
    assert result_ids.shape == (1, 3)
    # The query itself should be the top hit (cosine ≈ 1.0).
    assert result_ids[0, 0] == 0
    assert pytest.approx(float(scores[0, 0]), abs=0.02) == 1.0


def test_turbovec_allowlist_filter(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = TurbovecBackend(dim=dim)

    vectors = _unit_vecs(10, dim)
    ids = np.arange(10, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)

    query = vectors[0].reshape(1, -1)
    allowlist = np.array([5, 7, 9], dtype=np.uint64)
    _, result_ids = backend.search(query, k=10, allowlist=allowlist)
    returned = set(int(x) for x in result_ids[0] if int(x) != 0)
    assert returned.issubset({5, 7, 9})


def test_turbovec_remove(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = TurbovecBackend(dim=dim)

    vectors = _unit_vecs(5, dim)
    ids = np.arange(5, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)
    assert backend.count == 5

    backend.remove(np.array([0, 2], dtype=np.uint64))
    assert backend.count == 3

    query = vectors[1].reshape(1, -1)
    _, result_ids = backend.search(query, k=3)
    # id 1 should still be in results; ids 0 and 2 should not.
    returned = set(int(x) for x in result_ids[0] if int(x) != 0)
    assert 1 in returned
    assert 0 not in returned
    assert 2 not in returned


def test_turbovec_persistence_roundtrip(data_dir: str, small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = TurbovecBackend(dim=dim)

    vectors = _unit_vecs(5, dim)
    ids = np.arange(5, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)

    path = os.path.join(data_dir, "index.tvim")
    backend.write(path)
    assert os.path.exists(path)

    loaded = TurbovecBackend.load(path)
    assert loaded.dim == dim

    query = vectors[0].reshape(1, -1)
    scores, result_ids = loaded.search(query, k=3)
    assert result_ids[0, 0] == 0
    assert pytest.approx(float(scores[0, 0]), abs=0.02) == 1.0


def test_turbovec_refuses_wrong_extension(data_dir: str) -> None:
    backend = TurbovecBackend(dim=16)
    with pytest.raises(ValueError, match="must end in"):
        backend.write(os.path.join(data_dir, "index.faiss"))
    with pytest.raises(ValueError, match="must end in"):
        TurbovecBackend.load(os.path.join(data_dir, "index.faiss"))


def test_turbovec_empty_search(small_embedder: FakeEmbedder) -> None:
    backend = TurbovecBackend(dim=small_embedder.dim)
    query = np.random.default_rng().standard_normal(small_embedder.dim).astype(np.float32)
    scores, ids = backend.search(query, k=5)
    # Empty index returns 0 results (not padded to k).
    assert ids.shape[1] == 0


def test_turbovec_zero_k(small_embedder: FakeEmbedder) -> None:
    backend = TurbovecBackend(dim=small_embedder.dim)
    vectors = _unit_vecs(3, small_embedder.dim)
    backend.add_with_ids(vectors, np.arange(3, dtype=np.uint64))
    scores, ids = backend.search(vectors[0], k=0)
    assert scores.shape == (1, 0)
    assert ids.shape == (1, 0)


# ------------------------------------------------------------------ FAISS


def test_faiss_add_and_search(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = FaissBackend(dim=dim)

    vectors = _unit_vecs(10, dim)
    ids = np.arange(10, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)
    assert backend.count == 10

    query = vectors[0].reshape(1, -1)
    scores, result_ids = backend.search(query, k=3)
    assert result_ids.shape == (1, 3)
    assert result_ids[0, 0] == 0
    assert pytest.approx(float(scores[0, 0]), abs=0.02) == 1.0


def test_faiss_allowlist_filter(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = FaissBackend(dim=dim)

    vectors = _unit_vecs(10, dim)
    ids = np.arange(10, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)

    query = vectors[5].reshape(1, -1)
    allowlist = np.array([0, 1, 2], dtype=np.uint64)
    _, result_ids = backend.search(query, k=10, allowlist=allowlist)
    returned = set(int(x) for x in result_ids[0] if int(x) != 0)
    assert returned.issubset({0, 1, 2})


def test_faiss_remove(small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = FaissBackend(dim=dim)

    vectors = _unit_vecs(5, dim)
    ids = np.arange(5, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)
    assert backend.count == 5

    backend.remove(np.array([0, 2], dtype=np.uint64))
    # FAISS remove_ids shrinks ntotal.
    assert backend.count == 3


def test_faiss_persistence_roundtrip(data_dir: str, small_embedder: FakeEmbedder) -> None:
    dim = small_embedder.dim
    backend = FaissBackend(dim=dim)

    vectors = _unit_vecs(5, dim)
    ids = np.arange(5, dtype=np.uint64)
    backend.add_with_ids(vectors, ids)

    path = os.path.join(data_dir, "index.faiss")
    backend.write(path)
    assert os.path.exists(path)

    loaded = FaissBackend.load(path)
    assert loaded.dim == dim

    query = vectors[0].reshape(1, -1)
    scores, result_ids = loaded.search(query, k=3)
    assert result_ids[0, 0] == 0


def test_faiss_refuses_wrong_extension(data_dir: str) -> None:
    backend = FaissBackend(dim=16)
    with pytest.raises(ValueError, match="must end in"):
        backend.write(os.path.join(data_dir, "index.tvim"))
    with pytest.raises(ValueError, match="must end in"):
        FaissBackend.load(os.path.join(data_dir, "index.tvim"))


# ------------------------------------------------------------------ load_or_rebuild


def test_load_or_rebuild_missing_file(data_dir: str) -> None:
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)
    backend = load_or_rebuild(
        store, backend_name="turbovec", model="test-model", dim=16, data_dir=data_dir
    )
    assert isinstance(backend, TurbovecBackend)
    assert store.get_meta(META_BACKEND) == "turbovec"
    assert store.get_meta(META_MODEL) == "test-model"
    assert store.get_meta(META_DIM) == "16"


def test_load_or_rebuild_rebuilds_on_backend_mismatch(data_dir: str) -> None:
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)

    # First: create with turbovec.
    b1 = load_or_rebuild(
        store, backend_name="turbovec", model="m", dim=16, data_dir=data_dir
    )
    assert store.get_meta(META_BACKEND) == "turbovec"

    # Now request faiss — should rebuild (meta mismatch).
    b2 = load_or_rebuild(
        store, backend_name="faiss", model="m", dim=16, data_dir=data_dir
    )
    assert isinstance(b2, FaissBackend)
    assert store.get_meta(META_BACKEND) == "faiss"


def test_load_or_rebuild_rebuilds_on_model_mismatch(data_dir: str) -> None:
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)

    b1 = load_or_rebuild(
        store, backend_name="turbovec", model="m1", dim=16, data_dir=data_dir
    )
    assert store.get_meta(META_MODEL) == "m1"

    b2 = load_or_rebuild(
        store, backend_name="turbovec", model="m2", dim=16, data_dir=data_dir
    )
    assert store.get_meta(META_MODEL) == "m2"


def test_load_or_rebuild_rebuilds_on_dim_mismatch(data_dir: str) -> None:
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)

    b1 = load_or_rebuild(
        store, backend_name="turbovec", model="m", dim=16, data_dir=data_dir
    )
    assert store.get_meta(META_DIM) == "16"

    b2 = load_or_rebuild(
        store, backend_name="turbovec", model="m", dim=32, data_dir=data_dir
    )
    assert store.get_meta(META_DIM) == "32"


def test_load_or_rebuild_reuses_persisted_index(data_dir: str) -> None:
    """When everything matches, load the persisted file without rebuilding."""
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)

    b1 = load_or_rebuild(
        store, backend_name="turbovec", model="m", dim=16, data_dir=data_dir
    )
    index_file = index_path(data_dir, "turbovec")
    assert os.path.exists(index_file)

    # Second call with same config: should load from file.
    store2 = Store(db_path)
    b2 = load_or_rebuild(
        store2, backend_name="turbovec", model="m", dim=16, data_dir=data_dir
    )
    assert isinstance(b2, TurbovecBackend)
    assert b2.dim == 16


def test_load_or_rebuild_rebuilds_from_sqlite_data(data_dir: str, small_embedder: FakeEmbedder) -> None:
    """Rebuild path re-adds vectors from SQLite raw vectors to the index."""
    db_path = os.path.join(data_dir, "embeddings.db")
    store = Store(db_path)

    vectors = _unit_vecs(5, small_embedder.dim)
    for i in range(5):
        store.upsert_one(
            tool_ref=f"t.{i}",
            text=f"text {i}",
            content_hash=f"h{i}",
            server_id=None,
            tags=[],
            vector=vectors[i],
            model="m",
        )

    backend = load_or_rebuild(
        store, backend_name="turbovec", model="m", dim=small_embedder.dim, data_dir=data_dir
    )

    query = vectors[0].reshape(1, -1)
    scores, result_ids = backend.search(query, k=3)
    assert result_ids[0, 0] == 0
    assert pytest.approx(float(scores[0, 0]), abs=0.02) == 1.0
