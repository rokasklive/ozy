"""FastEmbed wrapper for the Ozy embedding sidecar.

The embedder is the only place that touches the ONNX runtime. Production code
uses :class:`FastEmbedEmbedder`; tests substitute a deterministic
:class:`FakeEmbedder` so they never trigger the model download.

Embedding contract (matches the Go side's expectations):

- Output is always a contiguous ``np.ndarray`` of ``dtype=float32`` with shape
  ``(n, dim)`` where ``dim`` is the model's native dimension.
- The same model is used for documents and queries. BGE-style query-prefixing
  is applied here so callers just hand over the raw text.
- Every result is stamped with the model id and dimension so persisted vectors
  can detect embedding-space drift.
"""

from __future__ import annotations

import logging
import os
import threading
from typing import Protocol, runtime_checkable

import numpy as np

LOGGER = logging.getLogger(__name__)

# BGE's documented convention: prepend ``"query: "`` to a query string so the
# model produces a query-shaped embedding. The model id is checked so we only
# apply the prefix for known BGE models; the prefix is a no-op for other models.
_BGE_QUERY_PREFIX = "query: "
_BGE_MODEL_MARKERS = ("bge-", "BAAI/")


def _needs_bge_query_prefix(model_name: str) -> bool:
    """Return True if the BGE ``"query: "`` prefix should be applied."""

    name = model_name.lower()
    return any(marker.lower() in name for marker in _BGE_MODEL_MARKERS)


@runtime_checkable
class Embedder(Protocol):
    """Protocol implemented by both the real and fake embedders.

    The protocol is what the rest of the sidecar depends on; the concrete
    :class:`FastEmbedEmbedder` is only constructed in production.
    """

    model: str
    dim: int

    def embed(self, texts: list[str], *, is_query: bool = False) -> np.ndarray:
        """Embed a batch of texts and return a ``(len(texts), dim)`` float32 array.

        When ``is_query`` is True the input strings are treated as queries
        (BGE ``"query: "`` prefix applied) and the implementation may use
        model-specific query branches.
        """

    def get_model_info(self) -> dict[str, object]:
        """Return ``{"model": str, "dim": int}`` describing the active model."""


class FastEmbedEmbedder:
    """Lazy FastEmbed wrapper.

    The underlying ``fastembed.TextEmbedding`` is only constructed on the
    first :meth:`embed` call so importing :mod:`sidecar` stays cheap and
    commands like ``health`` (which need metadata only) don't pay the
    model-loading cost.

    When ``EMBEDDING_REQUIRED=1`` is set in the environment, the model is
    forced to load eagerly in :meth:`ensure_loaded` so any download or
    initialization failure surfaces as a ``health`` error rather than a
    mysterious first-query failure.
    """

    def __init__(self, model: str = "BAAI/bge-small-en-v1.5") -> None:
        self.model = model
        self._dim: int | None = None
        self._lock = threading.Lock()
        self._impl = None  # the underlying fastembed.TextEmbedding

    def ensure_loaded(self) -> None:
        """Eagerly load the underlying model.

        Triggered by ``EMBEDDING_REQUIRED=1`` so a missing model surfaces
        during ``health`` instead of failing on the first query.
        """

        self._get_impl()

    def _get_impl(self):
        if self._impl is not None:
            return self._impl

        with self._lock:
            if self._impl is not None:
                return self._impl

            from fastembed import TextEmbedding  # local import: heavy

            LOGGER.info("loading FastEmbed model %s", self.model)
            self._impl = TextEmbedding(model_name=self.model)
            # fastembed exposes the dimension either as ``embedding_size`` or
            # via the underlying model. We sniff both for forward compat.
            dim = getattr(self._impl, "embedding_size", None)
            if dim is None:
                model_obj = getattr(self._impl, "model", None)
                dim = getattr(model_obj, "embedding_size", None)
            if dim is None:
                # Fall back to inferring from a probe embed call.
                probe = self._embed_raw(["probe"])
                dim = int(probe.shape[1])
            self._dim = int(dim)
            return self._impl

    @property
    def dim(self) -> int:
        if self._dim is not None:
            return self._dim
        # Trigger lazy init so the dim is known.
        self._get_impl()
        return int(self._dim or 0)

    def _embed_raw(self, texts: list[str]) -> np.ndarray:
        """Call the underlying fastembed model and return a numpy array.

        This bypasses the BGE query-prefix logic so the loader's probe call
        doesn't accidentally prefix its own synthetic text.
        """

        impl = self._get_impl()
        # ``embed`` returns a generator of numpy arrays; stack them.
        chunks = list(impl.embed(texts))
        if not chunks:
            return np.zeros((0, 0), dtype=np.float32)
        return np.vstack([np.asarray(c, dtype=np.float32) for c in chunks])

    def embed(self, texts: list[str], *, is_query: bool = False) -> np.ndarray:
        """Embed ``texts`` and return a ``(len(texts), dim)`` float32 array.

        Args:
            texts: The strings to embed. Empty list returns an empty
                ``(0, dim)`` array.
            is_query: When True, apply the BGE ``"query: "`` prefix to each
                string so the model produces a query-shaped embedding.

        Returns:
            ``np.ndarray`` of dtype ``float32`` and shape ``(n, dim)``.
        """

        if not texts:
            return np.zeros((0, self.dim), dtype=np.float32)

        prepared: list[str]
        if is_query and _needs_bge_query_prefix(self.model):
            prepared = [f"{_BGE_QUERY_PREFIX}{t}" for t in texts]
        else:
            prepared = list(texts)

        return self._embed_raw(prepared)

    def get_model_info(self) -> dict[str, object]:
        """Return ``{"model": str, "dim": int}`` for the active model.

        When ``EMBEDDING_REQUIRED=1`` the model is loaded eagerly so a
        failure surfaces during ``health``. Otherwise returns whatever is
        known without triggering a lazy load (``dim`` may be 0 when the
        model has never been loaded).
        """

        if os.environ.get("EMBEDDING_REQUIRED") == "1":
            self.ensure_loaded()
        return {"model": self.model, "dim": int(self._dim or 0)}


class FakeEmbedder:
    """Deterministic embedder used in tests.

    Generates a stable unit vector per text by hashing the text into ``dim``
    bins and normalizing. The same input always produces the same vector, so
    tests can assert exact scores without depending on a model download.

    By default, ``is_query`` changes the hash seed so query and document
    vectors live in separate spaces (matching the real BGE convention).
    Set ``separate_query_space=False`` to share the space (useful for ops
    tests that need query vectors to be close to document vectors).
    """

    def __init__(
        self,
        model: str = "fake-model",
        dim: int = 384,
        *,
        separate_query_space: bool = True,
    ) -> None:
        self.model = model
        self._dim = int(dim)
        self._lock = threading.Lock()
        self._separate_query_space = separate_query_space
        # Track how many times we were called so tests can assert caching /
        # skip behavior.
        self.call_count = 0

    @property
    def dim(self) -> int:
        return self._dim

    def embed(self, texts: list[str], *, is_query: bool = False) -> np.ndarray:
        """Return a deterministic ``(len(texts), dim)`` float32 array.

        Vectors are produced by seeding ``np.random.default_rng`` with a hash
        of the input, drawing ``dim`` values in ``[-1, 1]``, and normalizing
        to unit length. The normalization makes cosine similarity well-defined
        for tests that depend on relative scores.
        """

        if not texts:
            return np.zeros((0, self._dim), dtype=np.float32)

        with self._lock:
            self.call_count += len(texts)

        out = np.empty((len(texts), self._dim), dtype=np.float32)
        for i, text in enumerate(texts):
            if self._separate_query_space:
                seed = abs(hash((self.model, text, is_query))) % (2**32)
            else:
                seed = abs(hash((self.model, text))) % (2**32)
            rng = np.random.default_rng(seed)
            v = rng.standard_normal(self._dim).astype(np.float32)
            norm = float(np.linalg.norm(v))
            if norm > 0:
                v = v / norm
            out[i] = v
        return out

    def get_model_info(self) -> dict[str, object]:
        return {"model": self.model, "dim": int(self._dim)}


def make_embedder(model: str = "BAAI/bge-small-en-v1.5") -> Embedder:
    """Factory: return a :class:`FastEmbedEmbedder` for the named model.

    Exists so :mod:`sidecar.__main__` and tests can both construct the
    configured embedder by name without importing the concrete class
    directly.
    """

    return FastEmbedEmbedder(model=model)
