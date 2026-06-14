"""Protocol dispatch-loop tests.

Covers: framed round-trips, unknown-op error is non-fatal, malformed JSON,
multiple in-flight requests matched by id, flat args (spread format).

Uses direct Handler call (no subprocess) for speed.
"""

from __future__ import annotations

import io
import json
from typing import Any

import pytest

from sidecar.embedder import FakeEmbedder
from sidecar.ops import Ops
from sidecar.protocol import Handler, _KNOWN_OPS, parse_line, run_dispatch_loop
from sidecar.store import Store
from sidecar.vector import TurbovecBackend


def _make_handler(data_dir: str) -> Handler:
    import os

    store = Store(os.path.join(data_dir, "embeddings.db"))
    embedder = FakeEmbedder(model="test-model", dim=384)
    backend = TurbovecBackend(dim=embedder.dim)
    ops = Ops(
        embedder=embedder,
        store=store,
        backend=backend,
        backend_name="turbovec",
        data_dir=data_dir,
    )
    return Handler(ops)


# ------------------------------------------------------------------ parse_line


def test_parse_line_valid_json() -> None:
    result = parse_line('{"id": "r1", "op": "health"}')
    assert result == {"id": "r1", "op": "health"}


def test_parse_line_invalid_json() -> None:
    assert parse_line("not json") is None


def test_parse_line_empty_string() -> None:
    assert parse_line("") is None


def test_parse_line_whitespace_only() -> None:
    assert parse_line("   \t  ") is None


def test_parse_line_json_array() -> None:
    assert parse_line("[1,2,3]") is None


def test_parse_line_json_primitive() -> None:
    assert parse_line("42") is None


# ------------------------------------------------------------------ handler: known ops


def test_health_op_returns_model_info(data_dir: str) -> None:
    h = _make_handler(data_dir)
    resp = h.handle({"id": "r1", "op": "health"})
    assert resp["ok"] is True
    assert resp["model"] == "test-model"
    assert resp["dim"] == 384
    assert resp["backend"] == "turbovec"
    assert resp["vectorCount"] == 0


def test_stats_op_returns_counts(data_dir: str) -> None:
    h = _make_handler(data_dir)
    resp = h.handle({"id": "r2", "op": "stats"})
    assert resp["ok"] is True
    assert resp["backend"] == "turbovec"
    assert resp["vectorCount"] == 0
    assert resp["toolCount"] == 0


def test_unknown_op_returns_error_but_handler_survives(data_dir: str) -> None:
    h = _make_handler(data_dir)
    resp = h.handle({"id": "r3", "op": "bogus"})
    assert resp["ok"] is False
    assert "unknown op" in resp["error"]
    # Handler is still usable.
    resp2 = h.handle({"id": "r4", "op": "health"})
    assert resp2["ok"] is True


def test_missing_op_returns_error(data_dir: str) -> None:
    h = _make_handler(data_dir)
    resp = h.handle({"id": "r5"})
    assert resp["ok"] is False
    assert "missing op" in resp["error"]


def test_missing_id_defaults_to_zero(data_dir: str) -> None:
    h = _make_handler(data_dir)
    resp = h.handle({"op": "stats"})
    assert resp["id"] == 0
    assert resp["ok"] is True


def test_response_echoes_request_id(data_dir: str) -> None:
    h = _make_handler(data_dir)
    for rid in ["abc", 42, 0, "req-1"]:
        resp = h.handle({"id": rid, "op": "health"})
        assert resp["id"] == rid


# ------------------------------------------------------------------ flat args (spread format)


def test_flat_args_upsert(data_dir: str) -> None:
    """Flat args: all keys except id/op are operation args."""
    h = _make_handler(data_dir)
    resp = h.handle({
        "id": "r10",
        "op": "upsert",
        "items": [
            {
                "toolRef": "test.tool",
                "text": "hello world",
                "contentHash": "abc123",
                "serverId": "srv1",
                "tags": ["search"],
            }
        ],
    })
    assert resp["ok"] is True
    assert resp["upserted"] == 1
    assert resp["skipped"] == 0


def test_nested_args_still_supported(data_dir: str) -> None:
    """Backward compat: nested 'args' key also works."""
    h = _make_handler(data_dir)
    resp = h.handle({
        "id": "r11",
        "op": "health",
        "args": {},
    })
    assert resp["ok"] is True


def test_query_with_filter_flat_args(data_dir: str) -> None:
    """Query with filter using the flat args format."""
    h = _make_handler(data_dir)
    # First upsert a tool.
    h.handle({
        "id": "r12",
        "op": "upsert",
        "items": [
            {
                "toolRef": "srv1.tool",
                "text": "semantic search tool",
                "contentHash": "h1",
                "serverId": "srv1",
                "tags": ["search"],
            }
        ],
    })
    # Query with flat args and filter.
    resp = h.handle({
        "id": "r13",
        "op": "query",
        "text": "search",
        "k": 5,
        "filter": {"serverId": "srv1"},
    })
    assert resp["ok"] is True
    assert len(resp["hits"]) == 1
    assert resp["hits"][0]["toolRef"] == "srv1.tool"


def test_query_facet_filter_miss(data_dir: str) -> None:
    """Filter that matches nothing returns empty hits."""
    h = _make_handler(data_dir)
    h.handle({
        "id": "r14",
        "op": "upsert",
        "items": [
            {
                "toolRef": "srv1.tool",
                "text": "a tool",
                "contentHash": "h1",
                "serverId": "srv1",
                "tags": [],
            }
        ],
    })
    resp = h.handle({
        "id": "r15",
        "op": "query",
        "text": "search",
        "k": 5,
        "filter": {"serverId": "srv2"},
    })
    assert resp["ok"] is True
    assert resp["hits"] == []


# ------------------------------------------------------------------ delete


def test_delete_op(data_dir: str) -> None:
    h = _make_handler(data_dir)
    h.handle({
        "id": "r16",
        "op": "upsert",
        "items": [
            {
                "toolRef": "a.b",
                "text": "x",
                "contentHash": "h",
                "serverId": None,
                "tags": [],
            },
            {
                "toolRef": "c.d",
                "text": "y",
                "contentHash": "h2",
                "serverId": None,
                "tags": [],
            },
        ],
    })
    resp = h.handle({"id": "r17", "op": "delete", "toolRefs": ["a.b", "nonexistent"]})
    assert resp["ok"] is True
    assert resp["deleted"] == 1


# ------------------------------------------------------------------ dispatch loop


def test_dispatch_loop_roundtrip(data_dir: str) -> None:
    """Full dispatch loop processes multiple requests."""
    h = _make_handler(data_dir)

    stdin_data = (
        '{"id":"1","op":"health"}\n'
        '{"id":"2","op":"upsert","items":[{"toolRef":"a","text":"hello","contentHash":"h1","serverId":null,"tags":[]}]}\n'
        '{"id":"3","op":"query","text":"hello","k":5}\n'
        '{"id":"4","op":"stats"}\n'
    )
    stdin = io.StringIO(stdin_data)
    stdout = io.StringIO()

    run_dispatch_loop(h, stdin=stdin, stdout=stdout, stderr=io.StringIO())

    lines = [line for line in stdout.getvalue().strip().split("\n") if line]
    assert len(lines) == 4
    responses = [json.loads(line) for line in lines]

    assert responses[0]["id"] == "1"
    assert responses[0]["ok"] is True
    assert responses[0]["model"] == "test-model"

    assert responses[1]["id"] == "2"
    assert responses[1]["upserted"] == 1

    assert responses[2]["id"] == "3"
    assert len(responses[2]["hits"]) == 1

    assert responses[3]["id"] == "4"
    assert responses[3]["vectorCount"] == 1


def test_dispatch_loop_survives_malformed_json(data_dir: str) -> None:
    """Malformed JSON produces an error response but the loop continues."""
    h = _make_handler(data_dir)
    stdin = io.StringIO('garbage\n{"id":"2","op":"health"}\n')
    stdout = io.StringIO()

    run_dispatch_loop(h, stdin=stdin, stdout=stdout, stderr=io.StringIO())

    lines = [line for line in stdout.getvalue().strip().split("\n") if line]
    assert len(lines) == 2
    r1 = json.loads(lines[0])
    assert r1["ok"] is False
    assert "invalid JSON" in r1["error"]
    r2 = json.loads(lines[1])
    assert r2["ok"] is True


def test_dispatch_loop_handles_unknown_op(data_dir: str) -> None:
    """Unknown op returns error but loop continues."""
    h = _make_handler(data_dir)
    stdin = io.StringIO('{"id":"1","op":"nope"}\n{"id":"2","op":"stats"}\n')
    stdout = io.StringIO()

    run_dispatch_loop(h, stdin=stdin, stdout=stdout, stderr=io.StringIO())

    lines = [line for line in stdout.getvalue().strip().split("\n") if line]
    assert len(lines) == 2
    r1 = json.loads(lines[0])
    assert r1["ok"] is False
    assert "unknown op" in r1["error"]
    r2 = json.loads(lines[1])
    assert r2["ok"] is True


def test_dispatch_loop_empty_lines_skipped(data_dir: str) -> None:
    """Empty and whitespace-only lines are silently skipped."""
    h = _make_handler(data_dir)
    stdin = io.StringIO('\n  \n{"id":"1","op":"health"}\n\n')
    stdout = io.StringIO()

    run_dispatch_loop(h, stdin=stdin, stdout=stdout, stderr=io.StringIO())

    lines = [line for line in stdout.getvalue().strip().split("\n") if line]
    assert len(lines) == 1
    r = json.loads(lines[0])
    assert r["ok"] is True
