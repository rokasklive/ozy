"""Shared pytest fixtures for the Ozy embedding sidecar tests.

Every test gets a fresh data directory and a deterministic
:class:`FakeEmbedder`. The data directory holds the SQLite store and the
on-disk vector index; it is wiped between tests by pytest's ``tmp_path``.
"""

from __future__ import annotations

import os
import sys
from pathlib import Path
from typing import Iterator

import pytest

# Make the sidecar package importable when pytest is invoked from the
# repo root (e.g. ``pytest sidecar/tests``).
_ROOT = Path(__file__).resolve().parent.parent
if str(_ROOT) not in sys.path:
    sys.path.insert(0, str(_ROOT))

from sidecar.embedder import FakeEmbedder  # noqa: E402
from sidecar.ops import Ops  # noqa: E402
from sidecar.store import Store  # noqa: E402
from sidecar.vector import TurbovecBackend  # noqa: E402


@pytest.fixture
def data_dir(tmp_path: Path) -> str:
    """A fresh data dir for one test.

    Returned as a string because that's the type every other module
    expects. Using ``tmp_path`` ensures pytest cleans it up after the
    test finishes.
    """

    return str(tmp_path)


@pytest.fixture
def fake_embedder() -> FakeEmbedder:
    """A deterministic embedder with the BGE-small default dim.

    Query and document vectors share the same space so that semantic
    search tests work with the FakeEmbedder.
    """

    return FakeEmbedder(model="fake-bge-small", dim=384, separate_query_space=False)


@pytest.fixture
def small_embedder() -> FakeEmbedder:
    """A tiny-dim FakeEmbedder for vector backend tests.

    Using a small dim keeps test vectors cheap and avoids 4 KiB blobs
    per upsert. The actual value doesn't matter for backend correctness.
    """

    return FakeEmbedder(model="fake-small", dim=16)


@pytest.fixture
def store(data_dir: str) -> Iterator[Store]:
    """A :class:`Store` rooted in ``data_dir``; closed at teardown."""

    db_path = os.path.join(data_dir, "embeddings.db")
    s = Store(db_path)
    try:
        yield s
    finally:
        s.close()


@pytest.fixture
def turbovec_backend(small_embedder: FakeEmbedder) -> TurbovecBackend:
    """A turbovec backend at the test dim.

    Tests that need a FAISS backend construct one directly; turbovec is
    the default and always available.
    """

    return TurbovecBackend(dim=small_embedder.dim)


@pytest.fixture
def ops(
    fake_embedder: FakeEmbedder,
    store: Store,
    data_dir: str,
) -> Iterator[Ops]:
    """A wired :class:`Ops` with a turbovec backend and the fake embedder.

    Mirrors the production wiring from :mod:`sidecar.__main__` but
    swaps in a :class:`FakeEmbedder` and skips persistence. Tests that
    care about on-disk state build their own :class:`Ops` and call
    :func:`sidecar.ops.load_or_rebuild` explicitly.
    """

    backend = TurbovecBackend(dim=fake_embedder.dim)
    o = Ops(
        embedder=fake_embedder,
        store=store,
        backend=backend,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    try:
        yield o
    finally:
        store.close()
