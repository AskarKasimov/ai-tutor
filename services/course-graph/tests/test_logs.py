import io
import json
import logging

from fastapi import FastAPI
from fastapi.testclient import TestClient

from course_graph import logs
from course_graph.model_runtime import RedactRemoteURLs

TRACE = "0123456789abcdef0123456789abcdef"


def captured_logger():
    stream = io.StringIO()
    handler = logging.StreamHandler(stream)
    handler.setFormatter(logs.JsonFormatter())
    logger = logging.getLogger("course_graph.test_logs")
    logger.handlers[:] = [handler]
    logger.propagate = False
    logger.setLevel(logging.DEBUG)
    return logger, stream


def test_json_formatter_uses_the_shared_fields_and_the_current_trace():
    logger, stream = captured_logger()
    token = logs.trace_id.set(TRACE)
    try:
        logger.warning("event", extra={"gateway_request_id": "gateway-1"})
    finally:
        logs.trace_id.reset(token)
    entry = json.loads(stream.getvalue())
    assert entry["level"] == "warn" and entry["msg"] == "event" and entry["ts"].endswith("+00:00")
    assert entry["trace_id"] == TRACE and entry["gateway_request_id"] == "gateway-1"


def test_json_formatter_keeps_a_stacktrace_redacted_by_the_url_filter():
    logger, stream = captured_logger()
    logger.handlers[0].addFilter(RedactRemoteURLs())
    try:
        raise RuntimeError("download failed: https://cdn.example/model?token=secret")
    except RuntimeError:
        logger.exception("bootstrap failed")
    entry = json.loads(stream.getvalue())
    assert "RuntimeError" in entry["stacktrace"] and "token=secret" not in entry["stacktrace"]


def test_trace_from_header_continues_a_valid_trace_and_replaces_a_broken_one():
    assert logs.trace_from_header(f"00-{TRACE}-0123456789abcdef-01") == TRACE
    for broken in (None, "", "garbage", f"00-{'0' * 32}-0123456789abcdef-01"):
        replacement = logs.trace_from_header(broken)
        assert len(replacement) == 32 and replacement != "0" * 32


def test_traceparent_needs_a_current_trace():
    assert logs.traceparent() is None
    token = logs.trace_id.set(TRACE)
    try:
        assert logs.traceparent().startswith(f"00-{TRACE}-")
    finally:
        logs.trace_id.reset(token)


def test_middleware_logs_requests_with_the_trace_and_keeps_probes_at_debug(caplog):
    app = FastAPI()
    app.add_middleware(logs.TraceMiddleware)
    app.get("/health")(lambda: {"status": "ok"})
    app.get("/work")(lambda: logs.trace_id.get())
    client = TestClient(app)
    with caplog.at_level(logging.DEBUG, logger="course_graph.logs"):
        answer = client.get("/work", headers={"traceparent": f"00-{TRACE}-0123456789abcdef-01"})
        client.get("/health")
    assert answer.json() == TRACE
    levels = {record.path: record.levelno for record in caplog.records if record.name == "course_graph.logs"}
    assert levels == {"/work": logging.INFO, "/health": logging.DEBUG}
    assert logs.trace_id.get() == ""
