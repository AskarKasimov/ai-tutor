"""Bounded, process-local idempotency for in-flight OCR HTTP requests."""

from __future__ import annotations

from collections import OrderedDict
from concurrent.futures import Future
from dataclasses import dataclass, field
from datetime import datetime, timezone
import hashlib
import json
import threading


def _now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class OcrJob:
    job_id: str
    request_sha256: str
    state: str = "queued"
    stage: str = "opening"
    created_at: str = field(default_factory=_now)
    updated_at: str = field(default_factory=_now)
    pages_done: int | None = None
    pages_total: int | None = None
    result: dict | None = None
    result_bytes: int = 0
    error: str | None = None
    future: Future | None = None


class JobConflict(Exception):
    pass


class OcrJobs:
    """Singleflight OCR by stable ID and source digest, with bounded results."""

    def __init__(self, max_jobs: int = 128, max_result_bytes: int = 64 * 1024 * 1024) -> None:
        self._lock = threading.RLock()
        self._jobs: OrderedDict[str, OcrJob] = OrderedDict()
        self._max_jobs = max_jobs
        self._max_result_bytes = max_result_bytes
        self._result_bytes = 0

    def _clear_job_result(self, job: OcrJob) -> None:
        if job.result_bytes:
            self._result_bytes = max(0, self._result_bytes - job.result_bytes)
            job.result_bytes = 0
        job.result = None

    def reserve(self, job_id: str, filename: str, content: bytes) -> tuple[OcrJob, bool, Future | None]:
        digest = hashlib.sha256(filename.lower().encode("utf-8") + b"\0" + content).hexdigest()
        with self._lock:
            job = self._jobs.get(job_id)
            if job is not None:
                if job.request_sha256 != digest:
                    raise JobConflict("This job ID was already used with different content")
                self._jobs.move_to_end(job_id)
                if job.future is not None:
                    return job, False, job.future
                report = job.result.get("report", {}) if isinstance(job.result, dict) else {}
                if job.state == "succeeded" and job.result is not None and report.get("work_complete", True) is not False:
                    return job, False, None
                job.state, job.stage, job.error = "queued", "opening", None
                self._clear_job_result(job)
                job.updated_at = _now()
            else:
                job = OcrJob(job_id=job_id, request_sha256=digest)
                self._jobs[job_id] = job

            future: Future = Future()
            job.future = future
            self._prune()
            return job, True, future

    def update(self, job: OcrJob, *, state: str | None = None, stage: str | None = None,
               pages_done: int | None = None, pages_total: int | None = None) -> None:
        with self._lock:
            if state is not None:
                job.state = state
            if stage is not None:
                job.stage = stage
            if pages_done is not None:
                job.pages_done = pages_done
            if pages_total is not None:
                job.pages_total = pages_total
            job.updated_at = _now()

    def heartbeat(self, job: OcrJob) -> None:
        with self._lock:
            job.updated_at = _now()

    def finish(self, job: OcrJob, future: Future, result: dict) -> None:
        report = result.get("report", {}) if isinstance(result, dict) else {}
        work_complete = report.get("work_complete", True)
        encoded_size = len(json.dumps(result, ensure_ascii=False, separators=(",", ":")).encode("utf-8"))
        with self._lock:
            if work_complete is False:
                job.state, job.stage = "failed", "incomplete"
                job.error = "; ".join(report.get("warnings", [])) or "Work incomplete"
            else:
                job.state, job.stage, job.error = "succeeded", "completed", None
            job.future = None
            job.updated_at = _now()
            self._clear_job_result(job)
            # Only a retained result counts against the budget; an oversized one
            # is returned to the caller but not kept.
            if encoded_size <= self._max_result_bytes:
                job.result = result
                job.result_bytes = encoded_size
                self._result_bytes += encoded_size
            future.set_result((True, result))
            self._evict_results()
            self._prune()

    def fail(self, job: OcrJob, future: Future, safe_error: str) -> None:
        with self._lock:
            job.state, job.stage = "failed", "failed"
            job.error = safe_error
            job.future = None
            self._clear_job_result(job)
            job.updated_at = _now()
            future.set_result((False, safe_error))
            self._prune()

    def get(self, job_id: str) -> dict | None:
        with self._lock:
            job = self._jobs.get(job_id)
            if job is None:
                return None
            self._jobs.move_to_end(job_id)
            return self._dto(job)

    @staticmethod
    def _dto(job: OcrJob) -> dict:
        dto = {
            "job_id": job.job_id,
            "state": job.state,
            "stage": job.stage,
            "progress": {"pages_done": job.pages_done, "pages_total": job.pages_total},
            "updated_at": job.updated_at,
        }
        if job.result is not None:
            dto["result"] = job.result
        if job.error:
            dto["error"] = job.error
        return dto

    def _evict_results(self) -> None:
        for job in self._jobs.values():
            if self._result_bytes <= self._max_result_bytes:
                break
            if job.result is not None and job.future is None:
                self._clear_job_result(job)

    def _prune(self) -> None:
        while len(self._jobs) > self._max_jobs:
            removable = next((key for key, job in self._jobs.items() if job.future is None), None)
            if removable is None:
                return
            job = self._jobs.pop(removable)
            self._clear_job_result(job)


ocr_jobs = OcrJobs()
