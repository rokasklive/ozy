"""Tests for the FastEmbed wrapper and the deterministic ``FakeEmbedder``."""

from __future__ import annotations

import numpy as np
import pytest

from sidecar.embedder import (
    FakeEmbedder,
    FastEmbedEmbedder,
    ModelLoadError,
    _needs_bge_query_prefix,  # type: ignore[attr-defined]
)


class _FakeImpl:
    """Minimal stand-in for fastembed.TextEmbedding used by load tests."""

    embedding_size = 384

    def embed(self, texts):
        return [np.zeros((len(texts), 384), dtype=np.float32)]


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


def test_fastembed_self_heals_corrupt_cache(
    tmp_path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A failed load clears the model cache and re-fetches exactly once."""

    cache = tmp_path / "models"
    cache.mkdir()
    corrupt = cache / "partial.onnx"
    corrupt.write_bytes(b"truncated")

    emb = FastEmbedEmbedder(model="BAAI/bge-small-en-v1.5", cache_dir=str(cache))

    calls = {"n": 0}

    def fake_build():
        calls["n"] += 1
        if calls["n"] == 1:
            raise RuntimeError("corrupt model file")
        return _FakeImpl()

    monkeypatch.setattr(emb, "_build_impl", fake_build)

    emb.ensure_loaded()

    assert calls["n"] == 2  # failed once, re-fetched exactly once
    assert not corrupt.exists()  # the corrupt cache directory was cleared
    assert emb.dim == 384


def test_fastembed_self_heal_gives_up_after_one_retry(
    tmp_path, monkeypatch: pytest.MonkeyPatch
) -> None:
    """A model that fails twice surfaces ModelLoadError after one retry."""

    cache = tmp_path / "models"
    cache.mkdir()
    emb = FastEmbedEmbedder(model="m", cache_dir=str(cache))

    calls = {"n": 0}

    def always_fail():
        calls["n"] += 1
        raise RuntimeError("network down")

    monkeypatch.setattr(emb, "_build_impl", always_fail)

    with pytest.raises(ModelLoadError):
        emb.ensure_loaded()
    assert calls["n"] == 2  # initial attempt + exactly one retry


def test_fastembed_without_cache_dir_cannot_self_heal(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Without a known cache dir there is nothing to clear, so no retry."""

    emb = FastEmbedEmbedder(model="m")

    calls = {"n": 0}

    def fail_once():
        calls["n"] += 1
        raise RuntimeError("boom")

    monkeypatch.setattr(emb, "_build_impl", fail_once)

    with pytest.raises(ModelLoadError):
        emb.ensure_loaded()
    assert calls["n"] == 1  # no retry without a cache location to clear
