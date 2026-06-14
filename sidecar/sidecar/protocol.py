"""Newline-delimited JSON dispatch loop for the embedding sidecar.

The protocol is the contract between the Go daemon and this Python worker.
One JSON object per line on stdin; one JSON object per line on stdout,
correlated by ``id``. Logs go to stderr only.

This module exposes two layers:

- :class:`Handler` — a tiny class that maps a parsed request to a response
  via the live :class:`Ops` instance. Easy to unit-test by calling
  :meth:`Handler.handle` directly.
- :func:`run_dispatch_loop` — the production loop. Reads stdin line-by-line,
  hands each line to the handler, writes the response, flushes, and never
  exits on a per-request error.
"""

from __future__ import annotations

import json
import logging
import sys
from typing import Any, Callable, TextIO

from .ops import Ops

LOGGER = logging.getLogger(__name__)

# Operations the protocol understands. Anything else gets a structured
# error and the loop keeps running.
_KNOWN_OPS = {"health", "upsert", "delete", "query", "stats"}


class Handler:
    """Map one JSONL request to one JSONL response.

    Stateless beyond the :class:`Ops` reference. Constructed once and
    reused for every request in the dispatch loop.
    """

    def __init__(self, ops: Ops) -> None:
        self.ops = ops

    def handle(self, request: dict[str, Any]) -> dict[str, Any]:
        """Dispatch a parsed request and return a response dict.

        ``request`` must contain at least ``id`` (any JSON-coercible type
        — the spec says string, but we accept anything for resilience) and
        ``op``. Unknown ops and per-op exceptions are caught and turned
        into a structured error response so the loop never dies.
        """

        request_id = request.get("id", 0)
        op = request.get("op")
        if not isinstance(op, str) or not op:
            return {"id": request_id, "ok": False, "error": "missing op"}

        if op not in _KNOWN_OPS:
            return {
                "id": request_id,
                "ok": False,
                "error": f"unknown op: {op}",
            }

        method = getattr(self.ops, f"op_{op}", None)
        if method is None:
            # Defensive: _KNOWN_OPS and op_* should stay in sync.
            return {
                "id": request_id,
                "ok": False,
                "error": f"unimplemented op: {op}",
            }

        # Extract operation arguments from the request.
        # Supports both flat ("...args" spread per the spec) and nested
        # ("args": {...}) formats so the Go side can evolve independently.
        args = request.get("args")
        if args is None:
            # Flat format: all keys except id and op are operation args.
            args = {
                k: v
                for k, v in request.items()
                if k not in ("id", "op")
            }
        if not isinstance(args, dict):
            return {
                "id": request_id,
                "ok": False,
                "error": "args must be an object (flat keys or nested under 'args')",
            }

        try:
            response = method(args)
        except Exception as exc:  # noqa: BLE001
            LOGGER.exception("op %s failed", op)
            return {"id": request_id, "ok": False, "error": str(exc)}

        if not isinstance(response, dict):
            return {
                "id": request_id,
                "ok": False,
                "error": f"op {op!r} returned non-object response",
            }
        # Always echo the request id back so the Go side can correlate.
        response.setdefault("id", request_id)
        response.setdefault("ok", True)
        return response


def _parse_line(line: str) -> dict[str, Any] | None:
    """Parse one JSONL line, returning ``None`` on any error.

    The dispatch loop turns ``None`` into a structured error response.
    """

    line = line.strip()
    if not line:
        return None
    try:
        obj = json.loads(line)
    except json.JSONDecodeError:
        return None
    if not isinstance(obj, dict):
        return None
    return obj


def run_dispatch_loop(
    handler: Handler,
    *,
    stdin: TextIO | None = None,
    stdout: TextIO | None = None,
    stderr: TextIO | None = None,
) -> None:
    """Run the JSONL dispatch loop until stdin reaches EOF.

    Each input line produces exactly one output line. The function never
    raises on a per-request failure — a malformed JSON line or an op that
    raises is reported as ``{ok: false, error: "..."}`` and the loop
    continues.

    Args:
        handler: The :class:`Handler` to dispatch into.
        stdin: Read from this stream (default: ``sys.stdin``).
        stdout: Write responses to this stream (default: ``sys.stdout``).
        stderr: Write logs to this stream (default: ``sys.stderr``).
    """

    stdin = stdin if stdin is not None else sys.stdin
    stdout = stdout if stdout is not None else sys.stdout
    stderr = stderr if stderr is not None else sys.stderr

    if hasattr(stdin, "reconfigure"):
        try:
            stdin.reconfigure(encoding="utf-8", newline="\n")
        except (ValueError, OSError):
            pass
    if hasattr(stdout, "reconfigure"):
        try:
            stdout.reconfigure(encoding="utf-8", newline="\n", line_buffering=True)
        except (ValueError, OSError):
            pass

    LOGGER.info("sidecar dispatch loop started")

    for raw_line in stdin:
        line = raw_line.strip()
        if not line:
            continue

        request = _parse_line(line)
        if request is None:
            response = {"id": 0, "ok": False, "error": "invalid JSON"}
        else:
            try:
                response = handler.handle(request)
            except Exception as exc:  # noqa: BLE001
                # Belt-and-suspenders: Handler.handle already catches
                # per-op exceptions, but a failure in dispatch itself
                # (e.g. a KeyError in the dispatcher) must not kill the
                # worker.
                LOGGER.exception("dispatcher failed")
                response = {
                    "id": request.get("id", 0),
                    "ok": False,
                    "error": f"dispatch failed: {exc}",
                }

        try:
            stdout.write(json.dumps(response, ensure_ascii=False) + "\n")
            stdout.flush()
        except (BrokenPipeError, ValueError) as exc:
            # The Go side closed early; nothing we can do but exit
            # cleanly.
            print(
                f"sidecar: stdout closed: {exc}",
                file=stderr,
            )
            break
        except Exception as exc:  # noqa: BLE001
            # Last-resort: log and keep going. The Go side will see the
            # missing response and time out.
            print(
                f"sidecar: write failed: {exc}",
                file=stderr,
            )

    LOGGER.info("sidecar dispatch loop exited (stdin closed)")


def install_default_stdio() -> tuple[TextIO, TextIO, TextIO]:
    """Bind stdin/stdout to line-buffered text streams.

    Idempotent: a second call returns the same triple. Used by
    :mod:`sidecar.__main__` so tests that call :func:`run_dispatch_loop`
    with explicit streams don't have to think about buffering.
    """

    sys.stdin.reconfigure(encoding="utf-8", newline="\n")
    sys.stdout.reconfigure(encoding="utf-8", newline="\n", line_buffering=True)
    sys.stderr.reconfigure(encoding="utf-8", newline="\n", line_buffering=True)
    return sys.stdin, sys.stdout, sys.stderr


def make_handler(ops: Ops) -> Handler:
    """Convenience factory used by :mod:`sidecar.__main__` and tests."""

    return Handler(ops)


# Re-export for test convenience.
parse_line: Callable[[str], dict[str, Any] | None] = _parse_line
