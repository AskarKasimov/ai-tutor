"""JSON logs on stdout with the field names shared by the services.

Every line has ts, level, msg and logger; work started by a request also has
trace_id, so one search finds it in backend, course-graph and the gateway.
Pass other fields through ``extra``. Model prompts and answers go only to the
debug level.
"""
import contextvars
import json
import logging
import re
import secrets
import sys
import time
import traceback
from datetime import datetime, timezone

trace_id = contextvars.ContextVar("trace_id", default="")

_RECORD_FIELDS = set(vars(logging.makeLogRecord({}))) | {"message", "asctime", "taskName", "color_message"}
_TRACEPARENT = re.compile(r"[0-9a-f]{2}-([0-9a-f]{32})-[0-9a-f]{16}-[0-9a-f]{2}")
# Probes run every few seconds and would bury the real requests.
_PROBES = {"/health", "/ready/models", "/graph/build/progress"}

logger = logging.getLogger(__name__)


class JsonFormatter(logging.Formatter):
    def format(self, record: logging.LogRecord) -> str:
        entry = {
            "ts": datetime.fromtimestamp(record.created, timezone.utc).isoformat(),
            "level": "warn" if record.levelno == logging.WARNING else record.levelname.lower(),
            "msg": record.getMessage(),
            "logger": record.name,
        }
        if trace_id.get():
            entry["trace_id"] = trace_id.get()
        entry.update((key, value) for key, value in vars(record).items() if key not in _RECORD_FIELDS)
        # The URL redaction filter renders the traceback into exc_text.
        if record.exc_info:
            entry["stacktrace"] = "".join(traceback.format_exception(*record.exc_info))
        elif record.exc_text:
            entry["stacktrace"] = record.exc_text
        return json.dumps(entry, ensure_ascii=False, default=str)


def configure_logging(level: str) -> None:
    """Send every logger to stdout as JSON; level applies to this service's code."""
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JsonFormatter())
    root = logging.getLogger()
    root.handlers[:] = [handler]
    numeric = {"debug": logging.DEBUG, "info": logging.INFO, "warn": logging.WARNING, "error": logging.ERROR}[level]
    # Debug of the libraries is not useful and may contain request bodies.
    root.setLevel(max(logging.INFO, numeric))
    logging.getLogger("course_graph").setLevel(numeric)
    # Replaced by TraceMiddleware, which also knows the trace.
    logging.getLogger("uvicorn.access").disabled = True
    for name in ("uvicorn", "uvicorn.error"):
        logging.getLogger(name).handlers[:] = []
        logging.getLogger(name).propagate = True


def traceparent() -> str | None:
    """The W3C header value that continues the current trace, if there is one."""
    current = trace_id.get()
    return f"00-{current}-{secrets.token_hex(8)}-01" if current else None


def trace_from_header(value: str | None) -> str:
    match = _TRACEPARENT.fullmatch((value or "").strip().lower())
    if match and match.group(1) != "0" * 32:
        return match.group(1)
    return secrets.token_hex(16)


class TraceMiddleware:
    """Continue the caller's trace and log each request once it has finished."""

    def __init__(self, app):
        self.app = app

    async def __call__(self, scope, receive, send):
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return
        headers = dict(scope.get("headers") or [])
        token = trace_id.set(trace_from_header(headers.get(b"traceparent", b"").decode("latin-1")))
        started = time.monotonic()
        status = 500

        async def send_status(message):
            nonlocal status
            if message["type"] == "http.response.start":
                status = message["status"]
            await send(message)

        try:
            await self.app(scope, receive, send_status)
        finally:
            path = scope.get("path", "")
            if status >= 500:
                level = logging.ERROR
            elif path in _PROBES:
                level = logging.DEBUG
            else:
                level = logging.INFO
            logger.log(level, "http request", extra={"method": scope.get("method"), "path": path, "status": status,
                                                      "duration": round(time.monotonic() - started, 3)})
            trace_id.reset(token)
