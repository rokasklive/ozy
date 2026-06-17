"""Per-operation orchestration for the embedding sidecar.

Each public method on :class:`Ops` is one JSONL operation:

- :meth:`Ops.op_health` — report model/dim/backend/vector count.
- :meth:`Ops.op_upsert` — embed and store one or more tools, skipping
  unchanged tools by ``contentHash``.
- :meth:`Ops.op_delete` — remove tools from the store and the vector
  index.
- :meth:`Ops.op_query` — embed a query, search the index, map back to
  ``toolRef``s, and apply any facet filter.
- :meth:`Ops.op_stats` — report index statistics.

The methods return a dict ready to be JSON-serialized into a response.
They never write to stdout themselves — that is :mod:`sidecar.protocol`'s
job. Errors are reported as ``{"ok": False, "error": "..."}``; the dispatch
loop keeps the worker alive on per-request failure.
"""

from __future__ import annotations

import logging
import os
from typing import Any

import numpy as np

from .embedder import Embedder, ModelLoadError
from .store import META_BACKEND, META_DIM, META_MODEL, Store
from .vector import (
    FaissBackend,
    TurbovecBackend,
    VectorBackend,
    file_ext_for,
    index_path,
    make_backend,
)

LOGGER = logging.getLogger(__name__)


_BACKEND_CLS: dict[str, type[VectorBackend]] = {
    "turbovec": TurbovecBackend,
    "faiss": FaissBackend,
}


class Ops:
    """Bundle the live sidecar state (embedder + store + backend).

    Constructed once per process by :mod:`sidecar.__main__`; reused for
    every request. All methods are thread-safe by virtue of the store's
    internal lock and the fact that the dispatch loop is single-threaded.
    """

    def __init__(
        self,
        embedder: Embedder,
        store: Store,
        backend: VectorBackend,
        backend_name: str,
        data_dir: str,
    ) -> None:
        self.embedder = embedder
        self.store = store
        self.backend = backend
        self.backend_name = backend_name
        self.data_dir = data_dir

    def _persist_backend(self) -> None:
        """Write the current vector index to ``<data_dir>/index.<ext>``."""

        path = index_path(self.data_dir, self.backend_name)
        self.backend.write(path)
        LOGGER.debug("persisted index to %s", path)

    # ------------------------------------------------------------------ ops

    def op_health(self, args: dict[str, Any]) -> dict[str, Any]:
        info = self.embedder.get_model_info()
        return {
            "ok": True,
            "model": str(info["model"]),
            "dim": int(info["dim"]),
            "backend": self.backend_name,
            "vectorCount": int(self.store.count_vectors()),
        }

    def op_warmup(self, args: dict[str, Any]) -> dict[str, Any]:  # noqa: ARG002
        """Readiness probe: load the model and prove a query returns.

        Distinct from :meth:`op_health` (liveness), which reports metadata
        without loading the model. The warm-up pays the cold model-download
        cost once and runs one probe embed+search so callers can treat the
        sidecar as "available" only when semantic search actually works. A
        partial/corrupt cache self-heals via :meth:`Embedder.ensure_loaded`;
        a download that is still incomplete after the re-fetch is reported as a
        structured ``model_download_incomplete`` error rather than an opaque
        failure.
        """

        ensure = getattr(self.embedder, "ensure_loaded", None)
        if callable(ensure):
            try:
                ensure()
            except ModelLoadError as exc:
                return {
                    "ok": False,
                    "error": str(exc),
                    "errorKind": "model_download_incomplete",
                }
        # Prove the embed+search path returns end-to-end. An empty store yields
        # zero hits, which still counts as a successful (queryable) probe.
        self.op_query({"text": "probe", "k": 1})
        info = self.embedder.get_model_info()
        return {
            "ok": True,
            "model": str(info["model"]),
            "dim": int(info["dim"]),
            "backend": self.backend_name,
            "vectorCount": int(self.store.count_vectors()),
        }

    def op_upsert(self, args: dict[str, Any]) -> dict[str, Any]:
        items = args.get("items")
        if not isinstance(items, list):
            return {"ok": False, "error": "items must be a list"}

        upserted = 0
        skipped = 0
        errors: list[dict[str, Any]] = []

        for index, item in enumerate(items):
            if not isinstance(item, dict):
                errors.append({"index": index, "error": "item must be an object"})
                continue
            tool_ref = item.get("toolRef")
            text = item.get("text")
            content_hash = item.get("contentHash")
            server_id = item.get("serverId")
            tags = item.get("tags") or []
            if not isinstance(tool_ref, str) or not tool_ref:
                errors.append(
                    {"index": index, "toolRef": tool_ref, "error": "toolRef required"}
                )
                continue
            if not isinstance(text, str):
                errors.append(
                    {"index": index, "toolRef": tool_ref, "error": "text required"}
                )
                continue
            if not isinstance(content_hash, str) or not content_hash:
                errors.append(
                    {
                        "index": index,
                        "toolRef": tool_ref,
                        "error": "contentHash required",
                    }
                )
                continue
            if not isinstance(tags, list):
                tags = []

            try:
                skip_status = self.store.compute_skip(tool_ref, content_hash)
                if skip_status == "unchanged":
                    skipped += 1
                    continue

                # For changed tools: remove the old vector from the index
                # before re-adding (turbovec rejects duplicate ids).
                if skip_status == "changed":
                    existing = self.store.get_by_toolref(tool_ref)
                    if existing is not None:
                        self.backend.remove(
                            np.array([existing.vector_id], dtype=np.uint64)
                        )

                embeddings = self.embedder.embed([text], is_query=False)
                vec = np.asarray(embeddings[0], dtype=np.float32)
                _, vector_id = self.store.upsert_one(
                    tool_ref=tool_ref,
                    text=text,
                    content_hash=content_hash,
                    server_id=server_id,
                    tags=tags,
                    vector=vec,
                    model=self.embedder.model,
                )
                self.backend.add_with_ids(
                    vec.reshape(1, -1), np.array([vector_id], dtype=np.uint64)
                )
                upserted += 1
            except Exception as exc:  # noqa: BLE001
                LOGGER.exception("upsert failed for %s", tool_ref)
                errors.append(
                    {
                        "index": index,
                        "toolRef": tool_ref,
                        "error": str(exc),
                    }
                )

        if upserted:
            self._persist_backend()

        return {
            "ok": True,
            "upserted": int(upserted),
            "skipped": int(skipped),
            "errors": errors,
        }

    def op_delete(self, args: dict[str, Any]) -> dict[str, Any]:
        tool_refs = args.get("toolRefs")
        if not isinstance(tool_refs, list):
            return {"ok": False, "error": "toolRefs must be a list"}
        tool_refs = [t for t in tool_refs if isinstance(t, str) and t]
        if not tool_refs:
            return {"ok": True, "deleted": 0}

        vector_ids = self.store.delete_many(tool_refs)
        if vector_ids:
            self.backend.remove(np.array(vector_ids, dtype=np.uint64))
            self._persist_backend()
        return {"ok": True, "deleted": int(len(vector_ids))}

    def op_query(self, args: dict[str, Any]) -> dict[str, Any]:
        text = args.get("text")
        if not isinstance(text, str) or not text:
            return {"ok": False, "error": "text required"}
        k = int(args.get("k", 10))
        if k <= 0:
            return {"ok": True, "hits": []}

        raw_filter = args.get("filter")
        server_id = ""
        tag_filter: list[str] | None = None
        if isinstance(raw_filter, dict):
            sid = raw_filter.get("serverId")
            if isinstance(sid, str):
                server_id = sid
            tag_field = raw_filter.get("tags")
            if isinstance(tag_field, list):
                tag_filter = [t for t in tag_field if isinstance(t, str)]

        allowlist: np.ndarray | None = None
        if server_id or tag_filter:
            ids = self.store.resolve_facet_allowlist(
                server_id=server_id, tags=tag_filter
            )
            if not ids:
                return {"ok": True, "hits": []}
            allowlist = np.array(ids, dtype=np.uint64)

        embeddings = self.embedder.embed([text], is_query=True)
        query_vec = np.asarray(embeddings[0], dtype=np.float32).reshape(1, -1)
        scores, ids = self.backend.search(query_vec, k=k, allowlist=allowlist)
        id_row = ids[0]
        score_row = scores[0]
        hits: list[dict[str, Any]] = []
        for raw_id, raw_score in zip(id_row, score_row):
            vid = int(raw_id)
            # FAISS sentinel: -1 int64 becomes 18446744073709551615 in uint64.
            # Skip these and any truly invalid ids.
            if vid == 0xFFFFFFFFFFFFFFFF or float(raw_score) <= -1e30:
                continue
            row = self.store.get_by_vector_id(vid)
            if row is None:
                continue
            hits.append({"toolRef": row.tool_ref, "score": float(raw_score)})
            if len(hits) >= k:
                break
        return {"ok": True, "hits": hits}

    def op_stats(self, args: dict[str, Any]) -> dict[str, Any]:  # noqa: ARG002
        info = self.embedder.get_model_info()
        return {
            "ok": True,
            "backend": self.backend_name,
            "model": str(info["model"]),
            "dim": int(info["dim"]),
            "vectorCount": int(self.store.count_vectors()),
            "toolCount": int(self.store.count_vectors()),
        }


# --------------------------------------------------------------------- helpers


def load_or_rebuild(
    store: Store,
    *,
    backend_name: str,
    model: str,
    dim: int,
    data_dir: str,
) -> VectorBackend:
    """Load the persisted index or rebuild it from SQLite.

    Triggers a rebuild when:

    - the persisted index file is missing,
    - the recorded ``backend``/``model``/``dim`` in ``meta`` doesn't match
      the current configuration, or
    - the persisted file's extension doesn't match the active backend
      (refusing to cross-read between backends).
    """

    if backend_name not in _BACKEND_CLS:
        raise ValueError(f"unknown vector backend {backend_name!r}")

    path = os.path.join(data_dir, f"index{file_ext_for(backend_name)}")
    meta_backend = store.get_meta(META_BACKEND)
    meta_model = store.get_meta(META_MODEL)
    meta_dim = store.get_meta(META_DIM)

    needs_rebuild = False
    reason = ""
    if not os.path.exists(path):
        needs_rebuild = True
        reason = "no persisted index"
    elif meta_backend != backend_name:
        needs_rebuild = True
        reason = f"backend mismatch (meta={meta_backend!r}, config={backend_name!r})"
    elif meta_model != model:
        needs_rebuild = True
        reason = f"model mismatch (meta={meta_model!r}, config={model!r})"
    elif meta_dim is not None and int(meta_dim) != dim:
        needs_rebuild = True
        reason = f"dim mismatch (meta={meta_dim!r}, config={dim!r})"

    if needs_rebuild:
        LOGGER.info("rebuilding index from SQLite: %s", reason)
        backend = make_backend(backend_name, dim=dim)
        rows = list(store.iter_all())
        if rows:
            vectors = np.vstack(
                [
                    np.asarray(r.vector, dtype=np.float32).reshape(1, -1)
                    for r in rows
                ]
            )
            ids = np.array([r.vector_id for r in rows], dtype=np.uint64)
            backend.add_with_ids(vectors, ids)
        backend.write(path)
        store.set_meta(META_BACKEND, backend_name)
        store.set_meta(META_MODEL, model)
        store.set_meta(META_DIM, str(dim))
        return backend

    LOGGER.info("loading persisted index from %s", path)
    return _BACKEND_CLS[backend_name].load(path)
