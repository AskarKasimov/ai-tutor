import os

# Set before importing any Hugging Face or Docling modules.
os.environ.setdefault("HF_HUB_DISABLE_XET", "1")
os.environ.setdefault("HF_HUB_DOWNLOAD_TIMEOUT", "30")

from fastapi import FastAPI, File, Header, HTTPException, UploadFile
from pydantic import BaseModel, Field
from contextlib import asynccontextmanager
import asyncio
import logging
import re
import uuid

from course_graph.cleaning.cleaner import clean_markdown
from course_graph.llm.client import ModelClient
from starlette.concurrency import run_in_threadpool

from course_graph.ocr.converter import convert_document as convert_source_document, document_parser
from course_graph.ocr.jobs import JobConflict, OcrJob, ocr_jobs
from course_graph.model_runtime import model_runtime
from course_graph import progress as build_progress
from course_graph.topics.extract import ExtractionSettings, extract_topics
from course_graph.topics.graph import build_links
from course_graph.topics.models import Material, StrictModel, Topic
from course_graph.topics.retrieval import get_encoder

# Every stage asks for the low complexity (decision of 18.10.2026): every
# instance can serve low, and the gateway gives a low request to the free
# instance expected to answer soonest. Each pass still limits concurrency;
# low keeps all compatible instances eligible instead of only the high pool.
# Topic extraction and prerequisite checks keep the model's reasoning
# (reasoning=True): without it Qwen3.6 broke the strict removal format
# (invented reasons, foreign block ids) and materials failed. Cleaning,
# synonym checks and question refreshes ask for low effort.
STAGE_COMPLEXITY = "low"


@asynccontextmanager
async def lifespan(app: FastAPI):
    model_runtime.start()
    yield


app = FastAPI(title="Course Graph Service", version="1.0.0", lifespan=lifespan)
ocr_slots = asyncio.Semaphore(1)
# In-flight computations only: results remain in the bounded job store/disk
# cache. Each HTTP job keeps its own ID, status and filename.
ocr_flights = {}


async def compute_ocr(job, filename, content, requires_models):
    from .ocr.converter import get_cached_document
    if document_parser(filename) == "text":
        ocr_jobs.update(job, state="running", stage="converting")
        return await run_in_threadpool(convert_source_document, filename, content)
    if requires_models and not model_runtime.status()["ready"]:
        ocr_jobs.update(job, state="running", stage="waiting_models")
        if not await run_in_threadpool(model_runtime.wait):
            raise RuntimeError("OCR models are not ready")
    async with ocr_slots:
        # Another document may have populated the cache while we queued.
        cached = await run_in_threadpool(get_cached_document, filename, content)
        if cached is not None:
            return cached
        ocr_jobs.update(job, state="running", stage="converting")
        return await run_in_threadpool(convert_source_document, filename, content)


@app.get("/health")
def health():
    return {"status": "ok", "service": "course-graph"}


@app.get("/ready/models")
def models_readiness():
    return model_runtime.status()


async def run_ocr_job(job: OcrJob, future, filename: str, content: bytes, requires_models: bool):
    safe_error = "Не удалось преобразовать документ; исходный файл сохранён"
    async def heartbeat():
        while True:
            await asyncio.sleep(10)
            ocr_jobs.heartbeat(job)

    heartbeat_task = asyncio.create_task(heartbeat())
    try:
        from .ocr.converter import document_for_filename, document_key, get_cached_document
        cached = await run_in_threadpool(get_cached_document, filename, content)
        if cached is not None:
            ocr_jobs.finish(job, future, cached)
            logging.info("ocr_job_cache_hit job_id=%s request_sha256=%s", job.job_id, job.request_sha256)
            return

        key = document_key(content, filename)
        task = ocr_flights.get(key)
        if task is None:
            task = asyncio.create_task(compute_ocr(job, filename, content, requires_models))
            ocr_flights[key] = task
            task.add_done_callback(lambda done: ocr_flights.pop(key, None))
        else:
            ocr_jobs.update(job, state="running", stage="converting")
        # One cancelled waiter must not cancel computation for other jobs.
        result = document_for_filename(await asyncio.shield(task), filename)
        ocr_jobs.finish(job, future, result)
        logging.info("ocr_job_completed job_id=%s request_sha256=%s", job.job_id, job.request_sha256)
    except Exception as error:
        logging.error("ocr_job_failed job_id=%s error_type=%s", job.job_id, type(error).__name__)
        ocr_jobs.fail(job, future, safe_error)
    finally:
        heartbeat_task.cancel()


@app.post("/ocr/convert")
@app.post("/convert")
async def convert_document(
    file: UploadFile = File(...),
    job_id: str | None = Header(default=None, alias="X-Job-ID"),
):
    filename = file.filename or "document.pdf"
    content = await file.read()
    if not content:
        raise HTTPException(status_code=400, detail="Empty file uploaded")

    try:
        stable_id = job_id or str(uuid.uuid4())
        if len(stable_id) > 200 or not re.fullmatch(r"[A-Za-z0-9._:-]+", stable_id):
            raise HTTPException(status_code=400, detail="Invalid X-Job-ID")
        job, owner, future = ocr_jobs.reserve(stable_id, filename, content)
    except JobConflict as error:
        raise HTTPException(status_code=409, detail=str(error))

    if owner:
        requires_models = (
            document_parser(filename) == ".pdf"
            and os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true").lower() != "false"
        )
        asyncio.create_task(run_ocr_job(job, future, filename, content, requires_models))
    elif future is None:
        return ocr_jobs.get(stable_id)["result"]

    # Shield shared CPU work from HTTP cancellation; reconnecting requests join it.
    succeeded, value = await asyncio.shield(asyncio.wrap_future(future))
    if not succeeded:
        raise HTTPException(status_code=500, detail=value)
    return value


@app.get("/ocr/jobs/{job_id}")
def get_ocr_job(job_id: str):
    status = ocr_jobs.get(job_id)
    if status is None:
        raise HTTPException(status_code=404, detail="OCR job not found")
    return status


class CleanRequest(BaseModel):
    markdown: str = Field(min_length=1, max_length=2_000_000)


class ExtractRequest(CleanRequest):
    material_id: str = Field(default="material", min_length=1, max_length=600)
    week: int | None = Field(default=None, gt=0)
    kind: str | None = None
    filename: str | None = None
    kind_label: str | None = None
    material_revision: str | None = None
    max_chars: int = Field(default=ExtractionSettings().max_chars, ge=1500, le=20000)
    max_blocks: int = Field(default=45, ge=1, le=100)


class LinkRequest(StrictModel):
    topics: list[Topic] = Field(max_length=2000)
    per_requirement: int = Field(default=5, ge=1, le=20)
    max_chars: int = Field(default=16000, ge=4000, le=32000)
    evidence_chars: int = Field(default=1400, ge=200, le=4000)


class BuildRequest(StrictModel):
    materials: list[Material] = Field(min_length=1, max_length=500)
    max_chars: int = Field(default=ExtractionSettings().max_chars, ge=1500, le=20000)
    per_requirement: int = Field(default=5, ge=1, le=20)


# Bound documents as well as chunk calls; the shared backend pool bounds model calls.
cleaning_slots = asyncio.Semaphore(2)


@app.post("/clean")
async def clean_document(request: CleanRequest):
    async with cleaning_slots:
        client = ModelClient()
        try:
            return await clean_markdown(request.markdown, client.completer(STAGE_COMPLEXITY))
        except Exception:
            raise HTTPException(status_code=503, detail="Очистка временно недоступна")
        finally:
            await client.close()


@app.post("/topics/extract")
async def extract_document(request: ExtractRequest):
    async with cleaning_slots:
        client = ModelClient()
        try:
            result = await extract_topics(request.markdown, client.completer(STAGE_COMPLEXITY, reasoning=True), request.material_id,
                                         request.week, ExtractionSettings(request.max_chars, request.max_blocks))
            for topic in result["report"]["topics"]:
                for source in topic["sources"]:
                    source.update(kind=request.kind, filename=request.filename, kind_label=request.kind_label,
                                  material_revision=request.material_revision)
            result["report"]["retried_requests"] = client.retries
            return result
        except Exception:
            logging.exception("Topic extraction failed")
            raise HTTPException(status_code=503, detail="Выделение тем не завершено; исходный текст сохранён")
        finally:
            await client.close()


graph_slots = asyncio.Semaphore(1)


async def encoder_for_topics(topics):
    # A single topic needs no embeddings or downloaded weights; any other
    # course is compared topic by topic for synonyms and isolated topics.
    if len(topics) > 1:
        return await asyncio.to_thread(get_encoder)
    return None


@app.post("/graph/links")
async def link_topics(request: LinkRequest):
    async with graph_slots:
        client = ModelClient()
        try:
            encoder = await encoder_for_topics(request.topics)
            graph = await build_links(request.topics, client.completer(STAGE_COMPLEXITY, reasoning=True), encoder, request.per_requirement,
                                      request.max_chars, request.evidence_chars,
                                      light_complete=client.completer(STAGE_COMPLEXITY))
            graph["quality"]["retried_requests"] = client.retries
            return graph
        except Exception:
            logging.exception("Prerequisite generation failed")
            raise HTTPException(status_code=503, detail="Построение связей не завершено")
        finally:
            await client.close()


@app.get("/graph/build/progress")
async def graph_build_progress():
    """Stage and finished batches of the build that runs now; 404 when none runs."""
    if build_progress.current is None:
        raise HTTPException(status_code=404, detail="Сборка не идёт")
    return build_progress.current.as_dict()


@app.post("/graph/build")
async def build_course(request: BuildRequest,
                       build_id: str | None = Header(default=None, alias="X-Build-ID", max_length=200)):
    if len({m.id for m in request.materials}) != len(request.materials):
        raise HTTPException(status_code=422, detail="Повторный ID материала")
    if sum(len(m.markdown) for m in request.materials) > 8_000_000:
        raise HTTPException(status_code=413, detail="Суммарный размер курса превышает лимит")
    async with graph_slots:
        client = ModelClient()
        tracker = build_progress.BuildProgress(build_id or "")
        build_progress.current = tracker
        try:
            topics, reports = [], []
            # Materials without cached cards are extracted first; a step here is a
            # whole material, not a model batch (documented in the contract).
            tracker.start("topics", sum(m.topics is None for m in request.materials))
            materials = sorted(request.materials, key=lambda m: (m.week is None, m.week or 0, m.filename or "", m.id))
            for material in materials:
                if material.topics is not None:
                    for topic in material.topics:
                        for source in topic.sources:
                            if source.material_id != material.id or not 0 <= source.start < source.end <= len(material.markdown) or material.markdown[source.start:source.end] != source.text:
                                raise ValueError("Карточка не соответствует исходнику")
                            source.week = material.week
                            source.kind = material.kind
                            source.filename = material.filename
                            source.kind_label = material.kind_label
                            source.material_revision = material.material_revision
                        if topic.text != "\n\n".join(s.text for s in topic.sources):
                            raise ValueError("Текст темы не соответствует источникам")
                    topics.extend(material.topics)
                else:
                    result = await extract_topics(material.markdown, client.completer(STAGE_COMPLEXITY, reasoning=True), material.id,
                                                  material.week, ExtractionSettings(max_chars=request.max_chars))
                    for item in result["report"]["topics"]:
                        for source in item["sources"]:
                            source.update(kind=material.kind, filename=material.filename,
                                          kind_label=material.kind_label, material_revision=material.material_revision)
                    topics.extend(Topic.model_validate(t) for t in result["report"]["topics"])
                    reports.append({"material_id": material.id, **result["report"]})
                    tracker.done += 1
            encoder = await encoder_for_topics(topics)
            graph = await build_links(topics, client.completer(STAGE_COMPLEXITY, reasoning=True), encoder, request.per_requirement,
                                      light_complete=client.completer(STAGE_COMPLEXITY), progress=tracker)
            graph["extraction_reports"] = reports
            # Repeats of every call this build made, extraction of old reports included.
            graph["quality"]["retried_requests"] = client.retries
            return graph
        except Exception:
            logging.exception("Course build failed")
            raise HTTPException(status_code=503, detail="Граф не опубликован: этап не завершён")
        finally:
            build_progress.current = None
            await client.close()


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("course_graph.api:app", host="0.0.0.0", port=port, reload=False)
