"""CLI entrypoint for the Ozy embedding sidecar.

Run as ``python -m sidecar``. The worker reads newline-delimited JSON
requests on stdin, writes id-matched JSON responses on stdout, and keeps
logs on stderr.

The startup sequence is:

1. Parse args and resolve the data dir (``--data-dir`` /
   ``$OZY_SIDECAR_STATE_DIR`` / ``~/.local/state/ozy/sidecar``).
2. Open the SQLite store at ``<data_dir>/embeddings.db``.
3. Construct the configured embedder (FastEmbed, default model
   ``BAAI/bge-small-en-v1.5``). Eagerly load it when
   ``EMBEDDING_REQUIRED=1`` so a missing model fails fast.
4. Load the configured vector backend, or rebuild it from SQLite when the
   persisted file is missing / mismatched.
5. Hand the live state to :func:`sidecar.protocol.run_dispatch_loop` and
   serve until stdin closes.
"""

from __future__ import annotations

import argparse
import logging
import os
import sys
from typing import Sequence

from .embedder import FastEmbedEmbedder
from .ops import Ops, load_or_rebuild
from .protocol import Handler, run_dispatch_loop
from .store import Store
from .vector import make_backend

LOGGER = logging.getLogger(__name__)

DEFAULT_DATA_DIR = "~/.local/state/ozy/sidecar"
DEFAULT_BACKEND = "turbovec"
DEFAULT_MODEL = "BAAI/bge-small-en-v1.5"
DB_FILENAME = "embeddings.db"


def _resolve_data_dir(arg: str | None) -> str:
    """Resolve the data dir from flag, env, or default."""

    if arg:
        return os.path.expanduser(arg)
    env = os.environ.get("OZY_SIDECAR_STATE_DIR")
    if env:
        return os.path.expanduser(env)
    return os.path.expanduser(DEFAULT_DATA_DIR)


def _setup_logging(verbose: bool) -> None:
    """Configure stderr logging.

    The Go side reads only stdout for protocol messages, so every log
    line must land on stderr. We avoid touching stdout's buffering.
    """

    level = logging.DEBUG if verbose else logging.INFO
    handler = logging.StreamHandler(stream=sys.stderr)
    handler.setFormatter(
        logging.Formatter("%(asctime)s %(levelname)s %(name)s %(message)s")
    )
    root = logging.getLogger()
    root.handlers.clear()
    root.addHandler(handler)
    root.setLevel(level)


def _build_arg_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m sidecar",
        description=(
            "Ozy embedding sidecar. Reads newline-delimited JSON on stdin, "
            "writes id-matched JSON on stdout, logs to stderr."
        ),
    )
    parser.add_argument(
        "--data-dir",
        default=None,
        help=(
            "Directory for the SQLite DB and the persisted vector index. "
            "Defaults to $OZY_SIDECAR_STATE_DIR or "
            "~/.local/state/ozy/sidecar."
        ),
    )
    parser.add_argument(
        "--backend",
        default=DEFAULT_BACKEND,
        choices=["turbovec", "faiss"],
        help="Vector backend to use (default: turbovec).",
    )
    parser.add_argument(
        "--model",
        default=DEFAULT_MODEL,
        help="FastEmbed model id (default: BAAI/bge-small-en-v1.5).",
    )
    parser.add_argument(
        "--required",
        action="store_true",
        help=(
            "Eagerly load the embedding model at startup so any model "
            "failure surfaces during health. Equivalent to "
            "EMBEDDING_REQUIRED=1."
        ),
    )
    parser.add_argument(
        "--verbose",
        "-v",
        action="store_true",
        help="Enable debug logging to stderr.",
    )
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = _build_arg_parser().parse_args(argv)
    _setup_logging(verbose=args.verbose)
    if args.required:
        # Mirror the CLI flag into the env so embedder code can read it.
        os.environ["EMBEDDING_REQUIRED"] = "1"

    data_dir = _resolve_data_dir(args.data_dir)
    os.makedirs(data_dir, exist_ok=True)
    db_path = os.path.join(data_dir, DB_FILENAME)

    LOGGER.info(
        "starting sidecar: data_dir=%s backend=%s model=%s",
        data_dir,
        args.backend,
        args.model,
    )

    store = Store(db_path)
    # Own the model cache under the data dir so a partial/corrupt download has a
    # known location the embedder can clear and re-fetch (self-heal).
    model_cache_dir = os.path.join(data_dir, "models")
    embedder = FastEmbedEmbedder(model=args.model, cache_dir=model_cache_dir)
    if os.environ.get("EMBEDDING_REQUIRED") == "1":
        LOGGER.info("EMBEDDING_REQUIRED=1: loading model eagerly")
        embedder.ensure_loaded()

    dim = int(embedder.dim)
    backend_name = args.backend

    backend = load_or_rebuild(
        store,
        backend_name=backend_name,
        model=args.model,
        dim=dim,
        data_dir=data_dir,
    )

    ops = Ops(
        embedder=embedder,
        store=store,
        backend=backend,
        backend_name=backend_name,
        data_dir=data_dir,
    )
    handler = Handler(ops)

    try:
        run_dispatch_loop(handler)
    except KeyboardInterrupt:
        LOGGER.info("interrupted; shutting down")
        return 130
    finally:
        store.close()

    return 0


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
