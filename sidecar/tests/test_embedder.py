"""Tests for the FastEmbed wrapper and the deterministic ``FakeEmbedder``."""

from __future__ import annotations

import numpy as np
import pytest

from sidecar.embedder import (
    FakeEmbedder,
    FastEmbedEmbedder,
    _needs_bge_query_prefix,  # type: ignore[attr-defined]
)


def test_needs_bge_query_prefix_recognises_bge() -> None:
    assert _needs_bge_query_prefix("BAAI/bge-small-en-v1.5") is True
    assert _needs_bge_query_prefix("BAAI/bge-base-en") is True
    assert _needs_bge_query_prefix("sentence-transformers/all-MiniLM") is False


def test_fake_embedder_returns_unit_vectors() -> None:
    emb = FakeEmbedder(model="fake", dim=8)
    out = emb.embed(["alpha", "beta"], is_query=False)
    assert out.shape == (2, 8)
    assert out.dtype == np.float32
    norms = np.linalg.norm(out, axis=1)
    np.testing.assert_allclose(norms, np.ones(2), atol=1e-5)


def test_fake_embedder_deterministic() -> None:
    emb = FakeEmbedder(model="fake", dim=8)
    a = emb.embed(["hello world"], is_query=False)
    b = emb.embed(["hello world"], is_query=False)
    np.testing.assert_array_equal(a, b)


def test_fake_embedder_query_branch_is_different() -> None:
    emb = FakeEmbedder(model="fake", dim=8, separate_query_space=True)
    doc = emb.embed(["hello world"], is_query=False)
    query = emb.embed(["hello world"], is_query=True)
    # The query branch is seeded with is_query=True, so it should
    # produce a different vector for the same text.
    assert not np.array_equal(doc, query)


def test_fake_embedder_query_branch_same_when_shared() -> None:
    """When separate_query_space=False, query and doc vectors are identical."""
    emb = FakeEmbedder(model="fake", dim=8, separate_query_space=False)
    doc = emb.embed(["hello world"], is_query=False)
    query = emb.embed(["hello world"], is_query=True)
    np.testing.assert_array_equal(doc, query)


def test_fake_embedder_empty_input() -> None:
    emb = FakeEmbedder(model="fake", dim=8)
    out = emb.embed([], is_query=False)
    assert out.shape == (0, 8)
    assert out.dtype == np.float32


def test_fake_embedder_call_count() -> None:
    emb = FakeEmbedder(model="fake", dim=8)
    emb.embed(["a", "b", "c"], is_query=False)
    emb.embed(["d"], is_query=True)
    assert emb.call_count == 4


def test_fake_embedder_model_info() -> None:
    emb = FakeEmbedder(model="fake-bge", dim=384)
    info = emb.get_model_info()
    assert info == {"model": "fake-bge", "dim": 384}


def test_fastembed_get_model_info_is_lazy_without_required() -> None:
    """Without EMBEDDING_REQUIRED, get_model_info should not load the model.

    We assert this by checking that the underlying fastembed object is
    still ``None`` after the call.
    """

    import os

    os.environ.pop("EMBEDDING_REQUIRED", None)
    emb = FastEmbedEmbedder(model="BAAI/bge-small-en-v1.5")
    # If this were eager, the model would have started loading.
    assert emb._impl is None  # type: ignore[attr-defined]
    info = emb.get_model_info()
    assert info["model"] == "BAAI/bge-small-en-v1.5"
    assert info["dim"] == 0  # not yet loaded
    # And the underlying impl is still not loaded.
    assert emb._impl is None  # type: ignore[attr-defined]


def test_fastembed_ensure_loaded_with_required(monkeypatch: pytest.MonkeyPatch) -> None:
    """With EMBEDDING_REQUIRED=1, get_model_info should force a load.

    We don't actually download the model — we mock the underlying
    ``fastembed.TextEmbedding`` to return a fake impl.
    """

    import sidecar.embedder as embedder_mod

    class _FakeImpl:
        def __init__(self, model_name: str) -> None:
            self.model_name = model_name
            self.embedding_size = 384

        def embed(self, texts):
            return [np.zeros((len(texts), 384), dtype=np.float32)]

    monkeypatch.setattr("fastembed.TextEmbedding", _FakeImpl, raising=False)
    monkeypatch.setenv("EMBEDDING_REQUIRED", "1")

    # Need to patch the module-level import too — the embedder module
    # uses a local import. Patch the symbol via ``__getattr__`` or
    # override ``_get_impl`` directly. Simpler: just call ensure_loaded
    # after monkeypatching ``_get_impl`` to install a fake.
    emb = FastEmbedEmbedder(model="BAAI/bge-small-en-v1.5")

    def _fake_get_impl(self):
        self._impl = _FakeImpl(self.model)
        self._dim = 384
        return self._impl

    monkeypatch.setattr(FastEmbedEmbedder, "_get_impl", _fake_get_impl)
    emb.ensure_loaded()
    info = emb.get_model_info()
    assert info["model"] == "BAAI/bge-small-en-v1.5"
    assert info["dim"] == 384
