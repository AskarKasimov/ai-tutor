"""Process-wide model bootstrap and truthful OCR readiness."""

from __future__ import annotations

import logging
import os
import re
import threading
import time
import traceback
from datetime import datetime, timezone

from course_graph.config import configure_library_environment, get_settings

SETTINGS = get_settings()
configure_library_environment(SETTINGS)

logger = logging.getLogger(__name__)


def configured_ocr_threads() -> int:
    return SETTINGS.ocr.threads


def configured_ocr_device() -> str:
    """OCR device chosen in .env: cpu or gpu (CUDA)."""
    value = os.environ.get("COURSE_GRAPH_DEVICE", "cpu").strip().lower()
    if value not in ("cpu", "gpu"):
        raise ValueError("COURSE_GRAPH_DEVICE must be cpu or gpu")
    return value


def require_ocr_device() -> None:
    # Docling silently falls back to CPU without CUDA; a chosen GPU must fail loudly.
    if configured_ocr_device() != "gpu":
        return
    import torch
    if not torch.cuda.is_available():
        raise RuntimeError("COURSE_GRAPH_DEVICE=gpu, but CUDA is not available: build the image with "
                           "the gpu service and give the container a GPU")




class RedactRemoteURLs(logging.Filter):
    _url = re.compile(r"https?://[^\s\"'<>]+")

    def filter(self, record: logging.LogRecord) -> bool:
        record.msg = self._url.sub("<remote-url-redacted>", record.getMessage())
        record.args = ()
        if record.exc_info:
            rendered = "".join(traceback.format_exception(*record.exc_info))
            record.exc_text = self._url.sub("<remote-url-redacted>", rendered)
            record.exc_info = None
        return True


def install_url_redaction() -> None:
    filt = RedactRemoteURLs()
    prefixes = ("huggingface_hub", "httpx", "httpcore", "requests", "urllib3", "docling", "transformers")
    for name in prefixes:
        logging.getLogger(name).addFilter(filt)
    for name, candidate in logging.root.manager.loggerDict.items():
        if isinstance(candidate, logging.Logger) and name.startswith(prefixes):
            candidate.addFilter(filt)
    for handler in logging.getLogger().handlers:
        handler.addFilter(filt)


class ModelRuntime:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._done = threading.Event()
        self._state = "loading"
        self._stage = "waiting"
        self._detail: str | None = None
        self._updated_at = self._now()
        self._progress = {"cached_bytes": None, "cached_models_seen": []}
        self._started = False

    @staticmethod
    def _now() -> str:
        return datetime.now(timezone.utc).isoformat()

    def start(self) -> None:
        """Start exactly one background preload; never download in a probe."""
        with self._lock:
            if self._started:
                return
            self._started = True

            with_docling = os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true").lower() != "false"
            enabled = SETTINGS.models.bootstrap.enabled
            if not with_docling:
                self._state = "ready"
                self._stage = "ready"
                self._updated_at = self._now()
                self._done.set()
                return
            if not enabled:
                self._state = "failed"
                self._stage = "failed"
                self._detail = "OCR model bootstrap is disabled"
                self._updated_at = self._now()
                self._done.set()
                logger.error("model_bootstrap_disabled")
                return

            threading.Thread(target=self._bootstrap, name="course-graph-model-bootstrap", daemon=True).start()

    def _bootstrap(self) -> None:
        install_url_redaction()
        started = time.monotonic()
        cache_root = os.environ.get("HF_HOME", "/root/.cache/huggingface")
        logger.info("model_bootstrap_started cache=%s", cache_root)
        with self._lock:
            self._stage = "loading_models"
            self._updated_at = self._now()
        stop_progress = threading.Event()

        def report_cache_progress() -> None:
            while not stop_progress.wait(15):
                total_bytes = 0
                model_dirs = []
                observed_files = set()
                try:
                    for entry in os.scandir(os.path.join(cache_root, "hub")):
                        if not entry.is_dir() or not entry.name.startswith("models--"):
                            continue
                        model_dirs.append(entry.name.removeprefix("models--"))
                        for root, _, files in os.walk(entry.path):
                            for name in files:
                                try:
                                    stat = os.stat(os.path.join(root, name), follow_symlinks=True)
                                    identity = (stat.st_dev, stat.st_ino)
                                    if identity not in observed_files:
                                        observed_files.add(identity)
                                        total_bytes += stat.st_size
                                except OSError:
                                    pass
                except OSError:
                    pass
                with self._lock:
                    self._progress = {
                        "cached_bytes": total_bytes,
                        "cached_models_seen": sorted(model_dirs),
                    }
                    self._updated_at = self._now()
                logger.info(
                    "model_bootstrap_progress cached_bytes=%d models=%s elapsed_seconds=%.1f",
                    total_bytes, ",".join(sorted(model_dirs)), time.monotonic() - started,
                )

        progress_thread = threading.Thread(
            target=report_cache_progress, name="model-bootstrap-progress", daemon=True,
        )
        progress_thread.start()
        try:
            attempts = SETTINGS.models.bootstrap.attempts
            last_error: Exception | None = None
            for attempt in range(1, attempts + 1):
                try:
                    self._prepare_models()
                    last_error = None
                    break
                except Exception as error:
                    last_error = error
                    logger.error(
                        "model_bootstrap_attempt_failed attempt=%d attempts=%d error_type=%s",
                        attempt, attempts, type(error).__name__,
                    )
                    if attempt < attempts:
                        with self._lock:
                            self._stage = "retry_wait"
                            self._updated_at = self._now()
                        time.sleep(min(10 * attempt, 30))
                        with self._lock:
                            self._stage = "loading_models"
                            self._updated_at = self._now()
                        continue

            if last_error is not None:
                detail = f"OCR model preparation failed ({type(last_error).__name__})"
                with self._lock:
                    self._state = "failed"
                    self._stage = "failed"
                    self._detail = detail
                    self._updated_at = self._now()
                    self._done.set()
                logger.error(
                    "model_bootstrap_failed error_type=%s elapsed_seconds=%.1f",
                    type(last_error).__name__, time.monotonic() - started,
                )
                return

            with self._lock:
                self._state = "ready"
                self._stage = "ready"
                self._detail = None
                self._updated_at = self._now()
                self._done.set()
            logger.info("model_bootstrap_completed elapsed_seconds=%.1f", time.monotonic() - started)
        except Exception as exc:
            with self._lock:
                self._state = "failed"
                self._stage = "failed"
                self._detail = f"OCR model bootstrap crashed ({type(exc).__name__})"
                self._updated_at = self._now()
                self._done.set()
            logger.exception("model_bootstrap_crashed elapsed_seconds=%.1f", time.monotonic() - started)
        finally:
            stop_progress.set()
            self._done.set()

    def _prepare_models(self) -> None:
        # Import the heavyweight stack only in this one-time bootstrap worker.
        import torch
        from docling.datamodel.base_models import InputFormat
        from course_graph.ocr.converter import get_converter
        from course_graph.ocr.enrichment import NativeSyntaxGuard

        torch.set_num_threads(configured_ocr_threads())
        require_ocr_device()

        converter = get_converter()
        # StandardPdfPipeline initializes layout/OCR models in its
        # constructor. The VLM engine is lazy, so initialize it explicitly
        # here instead of making the first uploaded lecture pay cold-start.
        # initialize_pipeline returns None in Docling 2.139; the pipeline is kept
        # in initialized_pipelines.
        converter.initialize_pipeline(InputFormat.PDF)
        if SETTINGS.ocr.enrich_code or SETTINGS.ocr.enrich_formula:
            for pipeline in converter.initialized_pipelines.values():
                for stage in getattr(pipeline, "enrichment_pipe", []):
                    if isinstance(stage, NativeSyntaxGuard):
                        stage.engine.initialize()

    def status(self) -> dict:
        self.start()
        with self._lock:
            result = {
                "ready": self._state == "ready",
                "state": self._state,
                "stage": self._stage,
                "updated_at": self._updated_at,
                "progress": self._progress,
            }
            if self._detail:
                result["detail"] = self._detail
            return result

    def wait(self) -> bool:
        self.start()
        self._done.wait()
        with self._lock:
            return self._state == "ready"


model_runtime = ModelRuntime()
