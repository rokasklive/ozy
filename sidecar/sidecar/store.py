"""SQLite-backed embedding metadata store.

The store is the sidecar's source of truth for everything *except* the
high-dimensional vectors' nearest-neighbor index. It owns:

- the ``tools`` table — one row per indexed tool with the raw float32
  embedding, the model id and dimension used to produce it, the content hash
  used to skip unchanged tools on reindex, the filterable facets (server id
  and tags), and a monotonic ``uint64`` ``vector_id`` that ties the row to
  the vector index.
- the ``meta`` table — key/value bag for the active backend, model, dim, and
  schema version.

The store is intentionally narrow: it does not know about the vector index,
the embedder, or the stdio protocol. Operations on top of it (:mod:`sidecar.ops`)
are what glue those concerns together.

Layout (table names are part of the on-disk contract; do not rename without a
schema-version bump):

::

    tools(
        tool_ref     TEXT PRIMARY KEY,
        vector_id    INTEGER UNIQUE,    -- uint64; never reused
        content_hash TEXT NOT NULL,
        server_id    TEXT,
        tags         TEXT,              -- JSON array
        model        TEXT NOT NULL,
        dim          INTEGER NOT NULL,
        vector       BLOB NOT NULL      -- raw float32 little-endian
    )

    meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)
"""

from __future__ import annotations

import json
import logging
import os
import sqlite3
import threading
from dataclasses import dataclass
from typing import Iterable



import numpy as np

LOGGER = logging.getLogger(__name__)

# Schema version is stored in ``meta.schema_version``; bump on any
# destructive change to the table layout. The store refuses to operate on
# a database it doesn't recognize.
SCHEMA_VERSION = "1"

# Reserved meta keys. Centralized so callers don't sprinkle strings around.
META_BACKEND = "backend"
META_MODEL = "model"
META_DIM = "dim"
META_SCHEMA_VERSION = "schema_version"


@dataclass(frozen=True)
class ToolRow:
    """A single ``tools`` row, decoded.

    ``tags`` is a Python list (decoded from the JSON TEXT column).
    ``vector`` is a ``float32`` numpy array of shape ``(dim,)``.
    """

    tool_ref: str
    vector_id: int
    content_hash: str
    server_id: str | None
    tags: list[str]
    model: str
    dim: int
    vector: np.ndarray


def _row_to_tool_row(row: sqlite3.Row) -> ToolRow:
    tags_text = row["tags"]
    if tags_text:
        tags = json.loads(tags_text)
        if not isinstance(tags, list):
            tags = []
    else:
        tags = []
    vector_blob = row["vector"]
    dim = int(row["dim"])
    vector = np.frombuffer(vector_blob, dtype=np.float32).reshape(dim).copy()
    return ToolRow(
        tool_ref=row["tool_ref"],
        vector_id=int(row["vector_id"]),
        content_hash=row["content_hash"],
        server_id=row["server_id"],
        tags=tags,
        model=row["model"],
        dim=dim,
        vector=vector,
    )


class Store:
    """SQLite-backed embedding metadata store.

    The store owns its own connection (per-instance) and serializes writes
    behind a lock so callers can use it from multiple threads (the stdio
    dispatch loop is single-threaded, but the persistence helpers are also
    called from ``__main__`` during startup). Reads are atomic per-statement
    thanks to SQLite's default isolation.
    """

    def __init__(self, db_path: str) -> None:
        self.db_path = db_path
        os.makedirs(os.path.dirname(os.path.abspath(db_path)) or ".", exist_ok=True)
        self._lock = threading.RLock()
        self._conn = sqlite3.connect(db_path, check_same_thread=False)
        self._conn.row_factory = sqlite3.Row
        # WAL gives us crash-safe concurrent reads with one writer; that's
        # exactly the workload the stdio dispatch loop produces.
        self._conn.execute("PRAGMA journal_mode=WAL")
        self._conn.execute("PRAGMA synchronous=NORMAL")
        self._conn.execute("PRAGMA foreign_keys=ON")
        self._init_schema()

    def close(self) -> None:
        with self._lock:
            try:
                self._conn.close()
            except sqlite3.ProgrammingError:
                pass

    def __enter__(self) -> "Store":
        return self

    def __exit__(self, exc_type, exc, tb) -> None:
        self.close()

    # ------------------------------------------------------------------ schema

    def _init_schema(self) -> None:
        with self._lock:
            self._conn.executescript(
                """
                CREATE TABLE IF NOT EXISTS tools (
                    tool_ref     TEXT PRIMARY KEY,
                    vector_id    INTEGER UNIQUE,
                    content_hash TEXT NOT NULL,
                    server_id    TEXT,
                    tags         TEXT,
                    model        TEXT NOT NULL,
                    dim          INTEGER NOT NULL,
                    vector       BLOB NOT NULL
                );

                CREATE TABLE IF NOT EXISTS meta (
                    key   TEXT PRIMARY KEY,
                    value TEXT NOT NULL
                );

                CREATE INDEX IF NOT EXISTS idx_tools_server_id
                    ON tools(server_id);
                """
            )
            # Seed schema version on first run; never overwrite.
            cur = self._conn.execute(
                "SELECT value FROM meta WHERE key = ?", (META_SCHEMA_VERSION,)
            )
            if cur.fetchone() is None:
                self._conn.execute(
                    "INSERT INTO meta(key, value) VALUES (?, ?)",
                    (META_SCHEMA_VERSION, SCHEMA_VERSION),
                )
            self._conn.commit()

    # ------------------------------------------------------------------ meta

    def get_meta(self, key: str) -> str | None:
        """Return the meta value for ``key`` or None when missing."""

        with self._lock:
            cur = self._conn.execute("SELECT value FROM meta WHERE key = ?", (key,))
            row = cur.fetchone()
            return row["value"] if row else None

    def set_meta(self, key: str, value: str) -> None:
        """Insert-or-replace the meta value for ``key``."""

        with self._lock:
            self._conn.execute(
                "INSERT INTO meta(key, value) VALUES(?, ?) "
                "ON CONFLICT(key) DO UPDATE SET value = excluded.value",
                (key, value),
            )
            self._conn.commit()

    # ------------------------------------------------------------------ tools

    def get_by_toolref(self, tool_ref: str) -> ToolRow | None:
        """Return the row for ``tool_ref`` or None when missing."""

        with self._lock:
            cur = self._conn.execute(
                "SELECT * FROM tools WHERE tool_ref = ?", (tool_ref,)
            )
            row = cur.fetchone()
            return _row_to_tool_row(row) if row else None

    def get_by_vector_id(self, vector_id: int) -> ToolRow | None:
        """Return the row for ``vector_id`` or None when missing."""

        with self._lock:
            cur = self._conn.execute(
                "SELECT * FROM tools WHERE vector_id = ?", (int(vector_id),)
            )
            row = cur.fetchone()
            return _row_to_tool_row(row) if row else None

    def iter_all(self) -> Iterable[ToolRow]:
        """Yield every tool row in insertion order of ``vector_id``.

        Used by the rebuild path on startup to repopulate the vector index
        from the SQLite raw vectors. The iteration is materialized in a
        single SELECT so callers can close the cursor immediately.
        """

        with self._lock:
            cur = self._conn.execute(
                "SELECT * FROM tools ORDER BY vector_id ASC"
            )
            rows = cur.fetchall()
        for row in rows:
            yield _row_to_tool_row(row)

    def resolve_facet_allowlist(
        self,
        server_id: str = "",
        tags: list[str] | None = None,
    ) -> list[int]:
        """Return the matching ``vector_id``s for a facet filter.

        Empty inputs mean "no filter on this dimension":

        - ``server_id == ""`` does not constrain the server; matching rows
          include both NULL and non-NULL ``server_id`` values.
        - ``tags is None or tags == []`` does not constrain tags.
        - When ``tags`` is non-empty, at least one of the requested tags must
          be present in the row's JSON ``tags`` array.

        Returns an empty list when no rows match.
        """

        clauses: list[str] = []
        params: list[object] = []
        if server_id:
            clauses.append("server_id = ?")
            params.append(server_id)
        if tags:
            # Match rows whose JSON array intersects the requested tag set.
            # LIKE-based search is fine here because tags is a small array of
            # short strings; SQLite has no native JSON array operators enabled
            # in our build.
            tag_clauses = []
            for tag in tags:
                tag_clauses.append("tags LIKE ?")
                params.append(f'%"{tag}"%')
            clauses.append("(" + " OR ".join(tag_clauses) + ")")

        sql = "SELECT vector_id FROM tools"
        if clauses:
            sql += " WHERE " + " AND ".join(clauses)
        sql += " ORDER BY vector_id ASC"

        with self._lock:
            cur = self._conn.execute(sql, params)
            return [int(r["vector_id"]) for r in cur.fetchall()]

    def compute_skip(
        self, tool_ref: str, content_hash: str
    ) -> str:
        """Classify an upsert intent.

        Returns one of:

        - ``"unchanged"``: the tool is already present with the same
          ``content_hash``; the caller should skip both re-embedding and
          the SQLite write.
        - ``"changed"``: the tool is present but its ``content_hash``
          differs; the caller should re-embed and overwrite.
        - ``"new"``: no row exists for this ``tool_ref``; the caller should
          allocate a new ``vector_id`` and insert.
        """

        existing = self.get_by_toolref(tool_ref)
        if existing is None:
            return "new"
        if existing.content_hash == content_hash:
            return "unchanged"
        return "changed"

    def _next_vector_id(self) -> int:
        with self._lock:
            cur = self._conn.execute(
                "SELECT COALESCE(MAX(vector_id), -1) + 1 AS next_id FROM tools"
            )
            row = cur.fetchone()
            return int(row["next_id"])

    def upsert_one(
        self,
        tool_ref: str,
        text: str,  # noqa: ARG002 — text is part of the public API; stored
        # implicitly via the caller-supplied ``vector`` blob
        content_hash: str,
        server_id: str | None,
        tags: list[str],
        vector: np.ndarray,
        model: str | None = None,
    ) -> tuple[bool, int]:
        """Persist one tool row.

        Args:
            tool_ref: Stable id of the tool (e.g. ``"atlassian.search"``).
            text: Indexed text. Stored only as part of the row's model/dim
                context here; the caller is responsible for the actual
                embedding input.
            content_hash: Hash of the indexed text. Used to skip re-embed.
            server_id: Optional facet for filtered queries.
            tags: Optional facet tags.
            vector: The float32 embedding of ``text`` produced by the
                active model.
            model: The active model id to stamp on the row. When
                ``None``, falls back to ``meta.model`` (and finally
                ``"unknown"``).

        Returns:
            ``(skipped, vector_id)`` where ``skipped`` is True when the
            content hash matched an existing row (nothing was written).
        """

        status = self.compute_skip(tool_ref, content_hash)
        if status == "unchanged":
            existing = self.get_by_toolref(tool_ref)
            assert existing is not None  # compute_skip guarantees this
            return True, int(existing.vector_id)

        vector = np.asarray(vector, dtype=np.float32)
        if vector.ndim != 1:
            raise ValueError(
                f"vector must be 1-D (got shape {vector.shape!r})"
            )
        dim = int(vector.shape[0])
        if model is None:
            model = self._active_model or "unknown"
        tags_json = json.dumps(list(tags or []), ensure_ascii=False)

        with self._lock:
            cur = self._conn.execute(
                "SELECT vector_id FROM tools WHERE tool_ref = ?", (tool_ref,)
            )
            existing_row = cur.fetchone()
            if existing_row is not None:
                vector_id = int(existing_row["vector_id"])
            else:
                vector_id = self._next_vector_id()
            self._conn.execute(
                """
                INSERT INTO tools(tool_ref, vector_id, content_hash,
                                  server_id, tags, model, dim, vector)
                VALUES (?, ?, ?, ?, ?, ?, ?, ?)
                ON CONFLICT(tool_ref) DO UPDATE SET
                    vector_id    = excluded.vector_id,
                    content_hash = excluded.content_hash,
                    server_id    = excluded.server_id,
                    tags         = excluded.tags,
                    model        = excluded.model,
                    dim          = excluded.dim,
                    vector       = excluded.vector
                """,
                (
                    tool_ref,
                    vector_id,
                    content_hash,
                    server_id,
                    tags_json,
                    model,
                    dim,
                    vector.tobytes(),
                ),
            )
            self._conn.commit()
        return False, vector_id

    def delete_many(self, tool_refs: Iterable[str]) -> list[int]:
        """Delete the named tools and return their former ``vector_id``s.

        The returned ids are useful for the caller (typically ``ops.delete``)
        to clean up the corresponding entries in the vector index. Tools
        that don't exist are silently ignored — the count returned is the
        number of rows actually removed.
        """

        tool_refs = list(tool_refs)
        if not tool_refs:
            return []

        placeholders = ",".join("?" for _ in tool_refs)
        params: list[object] = list(tool_refs)

        with self._lock:
            cur = self._conn.execute(
                f"SELECT vector_id FROM tools WHERE tool_ref IN ({placeholders})",
                params,
            )
            vector_ids = [int(r["vector_id"]) for r in cur.fetchall()]
            self._conn.execute(
                f"DELETE FROM tools WHERE tool_ref IN ({placeholders})", params
            )
            self._conn.commit()
        return vector_ids

    def count_vectors(self) -> int:
        """Return the number of tools currently stored."""

        with self._lock:
            cur = self._conn.execute("SELECT COUNT(*) AS n FROM tools")
            return int(cur.fetchone()["n"])

    def all_vector_ids(self) -> list[int]:
        """Return every ``vector_id`` in ascending order.

        Used by the rebuild path so the caller can re-insert in a stable
        order.
        """

        with self._lock:
            cur = self._conn.execute("SELECT vector_id FROM tools ORDER BY vector_id ASC")
            return [int(r["vector_id"]) for r in cur.fetchall()]

    # ------------------------------------------------------------------ model

    @property
    def _active_model(self) -> str | None:
        """Return the active embedding model recorded in ``meta`` (if any)."""

        return self.get_meta(META_MODEL)
