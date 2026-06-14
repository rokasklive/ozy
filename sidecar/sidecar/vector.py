"""Pluggable vector index behind the embedding sidecar.

Two backends share a single :class:`VectorBackend` interface:

- :class:`TurbovecBackend` (default): zero-config, quantized (4-bit), kernel-
  level ``allowlist`` filtering. Used when the user does not pick a backend.
- :class:`FaissBackend` (opt-in): exact inner-product over an
  ``IndexIDMap(IndexFlatIP)``, with facet filtering implemented via FAISS's
  ``IDSelectorBatch`` and a ``SearchParameters`` ``sel`` slot. ``faiss`` is
  imported lazily so the default install stays FAISS-free.

The active backend is recorded in SQLite ``meta`` (``backend``) and is
immutable after the first index — switching backends forces a rebuild from
SQLite rather than mixing incompatible artifacts.

Custom file extensions are used so a stray persisted file from one backend
can never be read by the other's reader:

- turbovec → ``.tvim``
- FAISS → ``.faiss``
"""

from __future__ import annotations

import logging
import os
from typing import Protocol, runtime_checkable

import numpy as np

LOGGER = logging.getLogger(__name__)

# Default model dim for BGE-small-en-v1.5. The runtime derives ``dim`` from
# the embedder; this constant exists only as a sanity check for tests.
DEFAULT_DIM = 384

# Default turbovec quantization. 4 bits is a good recall/size trade-off for
# the small catalog sizes Ozy serves; callers can override later via config.
TURBOVEC_BIT_WIDTH = 4

# File extensions used to mark persisted indices. The startup loader refuses
# to read a file with the wrong extension.
TURBOVEC_EXT = ".tvim"
FAISS_EXT = ".faiss"


@runtime_checkable
class VectorBackend(Protocol):
    """Common interface for the supported vector backends.

    Implementations are expected to be safe to use from a single thread
    (the sidecar is single-threaded by design — the stdio loop is one
    coroutine of work). Concurrent use is not part of the contract.
    """

    dim: int

    @property
    def count(self) -> int:
        """Number of vectors currently in the index."""

    def add_with_ids(self, vectors: np.ndarray, ids: np.ndarray) -> None:
        """Add ``vectors`` (shape ``(n, dim)``, ``float32``) to the index.

        ``ids`` is a 1-D array of ``uint64`` external ids. The backend
        stores the mapping; later ``search``/``remove`` calls refer to
        vectors by these ids.
        """

    def search(
        self,
        query: np.ndarray,
        k: int,
        allowlist: np.ndarray | None = None,
    ) -> tuple[np.ndarray, np.ndarray]:
        """Return the top-``k`` nearest neighbors of ``query``.

        ``query`` has shape ``(1, dim)`` or ``(dim,)`` and dtype
        ``float32``. ``allowlist``, when provided, is a 1-D ``uint64``
        array; only neighbors whose id is in the allowlist may be
        returned. ``allowlist=None`` means "all vectors".

        Returns:
            ``(scores, ids)`` each of shape ``(1, k)``. ``scores`` are
            similarity scores (cosine for the default backends) sorted
            descending. Unfilled slots are ``-inf``/``-1`` for
            ``scores``/``ids`` respectively.
        """

    def remove(self, ids: np.ndarray) -> None:
        """Remove the vectors identified by ``ids`` (1-D ``uint64``)."""

    def write(self, path: str) -> None:
        """Persist the index to ``path``."""

    @classmethod
    def load(cls, path: str) -> "VectorBackend":
        """Rehydrate the index from a previously-written file."""


# --------------------------------------------------------------------- turbovec


class TurbovecBackend:
    """turbovec-backed vector index (default).

    Uses ``turbovec.IdMapIndex`` with 4-bit quantization. The ``allowlist``
    is forwarded as turbovec's native kernel-level filter, so facet
    searches are filtered inside the index rather than via over-fetch
    + post-filter.
    """

    backend_name = "turbovec"
    file_ext = TURBOVEC_EXT

    def __init__(self, dim: int, bit_width: int = TURBOVEC_BIT_WIDTH) -> None:
        if dim <= 0:
            raise ValueError(f"dim must be positive (got {dim!r})")
        from turbovec import IdMapIndex  # local import — package is heavy

        self.dim = int(dim)
        self._bit_width = int(bit_width)
        self._impl = IdMapIndex(dim=self.dim, bit_width=self._bit_width)
        # turbovec exposes no public count; track live ids locally.
        self._ids: set[int] = set()

    @classmethod
    def _from_impl(cls, impl, dim: int, bit_width: int) -> "TurbovecBackend":
        """Construct without creating a new IdMapIndex (used by load)."""
        backend = object.__new__(cls)
        backend.dim = int(dim)
        backend._bit_width = int(bit_width)
        backend._impl = impl
        backend._ids = set()
        return backend

    @property
    def count(self) -> int:
        return len(self._ids)

    def add_with_ids(self, vectors: np.ndarray, ids: np.ndarray) -> None:
        vectors = np.ascontiguousarray(vectors, dtype=np.float32)
        ids = np.ascontiguousarray(ids, dtype=np.uint64)
        if vectors.ndim != 2 or vectors.shape[1] != self.dim:
            raise ValueError(
                f"vectors must be (n, {self.dim}) float32; got {vectors.shape!r}"
            )
        if ids.shape != (vectors.shape[0],):
            raise ValueError(
                f"ids must be 1-D with len == vectors.shape[0] "
                f"(got shape {ids.shape!r}, vectors {vectors.shape!r})"
            )
        self._impl.add_with_ids(vectors, ids)
        for i in ids:
            self._ids.add(int(i))

    def search(
        self,
        query: np.ndarray,
        k: int,
        allowlist: np.ndarray | None = None,
    ) -> tuple[np.ndarray, np.ndarray]:
        if k <= 0:
            # The Go side never asks for k<=0, but be safe.
            return (
                np.zeros((1, 0), dtype=np.float32),
                np.zeros((1, 0), dtype=np.uint64),
            )
        query = np.ascontiguousarray(query, dtype=np.float32)
        if query.ndim == 1:
            query = query.reshape(1, -1)
        if query.ndim != 2 or query.shape[1] != self.dim:
            raise ValueError(
                f"query must be (1, {self.dim}) float32; got {query.shape!r}"
            )
        if allowlist is not None:
            allowlist = np.ascontiguousarray(allowlist, dtype=np.uint64)
        scores, ids = self._impl.search(query, k, allowlist=allowlist)
        # turbovec already returns float32 / uint64 arrays; coerce to be
        # safe across future versions.
        scores = np.asarray(scores, dtype=np.float32)
        ids = np.asarray(ids, dtype=np.uint64)
        return scores, ids

    def remove(self, ids: np.ndarray) -> None:
        ids = np.asarray(ids, dtype=np.uint64).reshape(-1)
        for id_ in ids:
            try:
                self._impl.remove(np.uint64(id_))
                self._ids.discard(int(id_))
            except Exception as exc:
                LOGGER.debug("turbovec remove(%s) ignored: %s", int(id_), exc)

    def write(self, path: str) -> None:
        # Defensive: refuse to overwrite a non-turbovec file.
        if not path.endswith(self.file_ext):
            raise ValueError(
                f"turbovec write path must end in {self.file_ext!r} (got {path!r})"
            )
        os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
        self._impl.write(path)

    @classmethod
    def load(cls, path: str) -> "TurbovecBackend":
        if not path.endswith(cls.file_ext):
            raise ValueError(
                f"turbovec load path must end in {cls.file_ext!r} (got {path!r})"
            )
        from turbovec import IdMapIndex  # local import

        impl = IdMapIndex.load(path)
        backend = cls._from_impl(impl, dim=int(impl.dim), bit_width=TURBOVEC_BIT_WIDTH)
        return backend


# --------------------------------------------------------------------- FAISS


class FaissBackend:
    """FAISS-backed vector index (opt-in).

    Wraps ``faiss.IndexIDMap(faiss.IndexFlatIP(dim))``. Inner-product over
    unit-normalized vectors is equivalent to cosine similarity, which is
    what the sidecar promises to the Go side. Facet filtering uses
    ``faiss.IDSelectorBatch`` with a ``SearchParameters(sel=...)`` slot.

    FAISS uses ``int64`` ids internally; the wrapper translates the
    sidecar's ``uint64`` ids at the boundary.
    """

    backend_name = "faiss"
    file_ext = FAISS_EXT

    def __init__(self, dim: int) -> None:
        if dim <= 0:
            raise ValueError(f"dim must be positive (got {dim!r})")
        import faiss  # local import: this is the opt-in path

        self.dim = int(dim)
        self._impl = faiss.IndexIDMap(faiss.IndexFlatIP(self.dim))
        self._removed_ids: set[int] = set()

    @property
    def count(self) -> int:
        return int(self._impl.ntotal)

    def add_with_ids(self, vectors: np.ndarray, ids: np.ndarray) -> None:
        vectors = np.ascontiguousarray(vectors, dtype=np.float32)
        if vectors.ndim != 2 or vectors.shape[1] != self.dim:
            raise ValueError(
                f"vectors must be (n, {self.dim}) float32; got {vectors.shape!r}"
            )
        ids = np.ascontiguousarray(ids, dtype=np.uint64)
        # FAISS needs int64; convert at the boundary.
        ids_i64 = ids.astype(np.int64)
        self._impl.add_with_ids(vectors, ids_i64)

    def search(
        self,
        query: np.ndarray,
        k: int,
        allowlist: np.ndarray | None = None,
    ) -> tuple[np.ndarray, np.ndarray]:
        if k <= 0:
            return (
                np.zeros((1, 0), dtype=np.float32),
                np.zeros((1, 0), dtype=np.uint64),
            )
        query = np.ascontiguousarray(query, dtype=np.float32)
        if query.ndim == 1:
            query = query.reshape(1, -1)
        if query.ndim != 2 or query.shape[1] != self.dim:
            raise ValueError(
                f"query must be (1, {self.dim}) float32; got {query.shape!r}"
            )

        import faiss

        params = None
        if allowlist is not None and len(allowlist) > 0:
            allowlist_i64 = np.ascontiguousarray(
                np.asarray(allowlist, dtype=np.uint64), dtype=np.int64
            )
            sel = faiss.IDSelectorBatch(allowlist_i64)
            params = faiss.SearchParameters(sel=sel)
            scores, ids = self._impl.search(query, k, params=params)
        else:
            scores, ids = self._impl.search(query, k)

        scores = np.asarray(scores, dtype=np.float32)
        ids = np.asarray(ids, dtype=np.int64)
        # FAISS pads with -1 ids and -FLT_MAX scores when fewer than k
        # matches are found. Filter these sentinel values out so callers
        # never see them.
        valid_mask = ids[0] != -1
        if not np.any(valid_mask):
            return (
                np.zeros((1, 0), dtype=np.float32),
                np.zeros((1, 0), dtype=np.uint64),
            )
        scores = scores[:, valid_mask]
        ids = ids[:, valid_mask]
        ids_u64 = ids.astype(np.uint64)
        return scores, ids_u64

    def remove(self, ids: np.ndarray) -> None:
        ids = np.asarray(ids, dtype=np.uint64).reshape(-1)
        if ids.size == 0:
            return
        ids_i64 = ids.astype(np.int64)
        self._impl.remove_ids(ids_i64)
        for i in ids:
            self._removed_ids.add(int(i))

    def write(self, path: str) -> None:
        if not path.endswith(self.file_ext):
            raise ValueError(
                f"faiss write path must end in {self.file_ext!r} (got {path!r})"
            )
        os.makedirs(os.path.dirname(os.path.abspath(path)) or ".", exist_ok=True)
        import faiss

        faiss.write_index(self._impl, path)

    @classmethod
    def load(cls, path: str) -> "FaissBackend":
        if not path.endswith(cls.file_ext):
            raise ValueError(
                f"faiss load path must end in {cls.file_ext!r} (got {path!r})"
            )
        import faiss

        impl = faiss.read_index(path)
        if impl.d != cls.__name__ and not hasattr(impl, "ntotal"):
            # Defensive: if a non-FAISS file somehow slipped through, this
            # attribute access will fail.
            pass  # pragma: no cover
        dim = int(impl.d)
        backend = cls(dim=dim)
        backend._impl = impl  # type: ignore[attr-defined]
        return backend


# --------------------------------------------------------------------- factory


_SUPPORTED = {"turbovec", "faiss"}


def make_backend(name: str, dim: int) -> VectorBackend:
    """Factory: instantiate a :class:`VectorBackend` by name.

    Raises ``ValueError`` for an unknown backend. The dim is the vector
    dimension derived from the active embedding model.
    """

    name = (name or "").strip().lower()
    if name == "turbovec":
        return TurbovecBackend(dim=dim)
    if name == "faiss":
        return FaissBackend(dim=dim)
    raise ValueError(
        f"unknown vector backend {name!r}; expected one of {sorted(_SUPPORTED)}"
    )


def file_ext_for(name: str) -> str:
    """Return the persisted file extension for a given backend name."""

    name = (name or "").strip().lower()
    if name == "turbovec":
        return TURBOVEC_EXT
    if name == "faiss":
        return FAISS_EXT
    raise ValueError(f"unknown vector backend {name!r}")


def index_path(data_dir: str, backend_name: str) -> str:
    """Return the canonical on-disk path for a backend's persisted index."""

    return os.path.join(data_dir, f"index{file_ext_for(backend_name)}")
