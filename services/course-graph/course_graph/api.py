import os

# Set before importing any Hugging Face or Docling modules.
os.environ.setdefault("HF_HUB_DISABLE_XET", "1")
os.environ.setdefault("HF_HUB_DOWNLOAD_TIMEOUT", "30")

from fastapi import FastAPI, File, Header, HTTPException, UploadFile
from pydantic import BaseModel, Field
import asyncio
import logging
import re
import uuid

from course_graph.cleaning.cleaner import clean_markdown
from course_graph.llm.client import ModelClient
from starlette.concurrency import run_in_threadpool

from course_graph.ocr.converter import convert_document as convert_source_document
from course_graph.ocr.jobs import JobConflict, OcrJob, ocr_jobs
from course_graph.model_runtime import model_runtime
from course_graph.topics.extract import ExtractionSettings, extract_topics
from course_graph.topics.graph import build_links
from course_graph.topics.models import Material, StrictModel, Topic
from course_graph.topics.retrieval import get_encoder

app = FastAPI(title="Course Graph Service", version="1.0.0")
ocr_slots = asyncio.Semaphore(1)


@app.on_event("startup")
def start_model_bootstrap():
    model_runtime.start()


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
        if requires_models:
            status = model_runtime.status()
            if not status["ready"]:
                ocr_jobs.update(job, state="running", stage="waiting_models")
                ready = await run_in_threadpool(model_runtime.wait)
                if not ready:
                    raise RuntimeError("OCR models are not ready")
        ocr_jobs.update(job, state="running", stage="opening")
        async with ocr_slots:
            ocr_jobs.update(job, state="running", stage="converting")
            result = await run_in_threadpool(convert_source_document, filename, content)
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
            filename.lower().endswith(".pdf")
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
    week: int | None = None
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
            return await clean_markdown(request.markdown, client.complete)
        except Exception:
            raise HTTPException(status_code=503, detail="Очистка временно недоступна")
        finally:
            await client.close()


@app.post("/topics/extract")
async def extract_document(request: ExtractRequest):
    async with cleaning_slots:
        client = ModelClient()
        try:
            return await extract_topics(request.markdown, client.complete, request.material_id,
                                        request.week, ExtractionSettings(request.max_chars, request.max_blocks))
        except Exception:
            logging.exception("Topic extraction failed")
            raise HTTPException(status_code=503, detail="Выделение тем не завершено; исходный текст сохранён")
        finally:
            await client.close()


graph_slots = asyncio.Semaphore(1)


async def encoder_for_topics(topics):
    # A course without dependencies needs no embeddings or downloaded weights.
    if len(topics) > 1 and any(t.requires for t in topics):
        return await asyncio.to_thread(get_encoder)
    return None


@app.post("/graph/links")
async def link_topics(request: LinkRequest):
    async with graph_slots:
        client = ModelClient()
        try:
            encoder = await encoder_for_topics(request.topics)
            return await build_links(request.topics, client.complete, encoder, request.per_requirement,
                                     request.max_chars, request.evidence_chars)
        except Exception:
            logging.exception("Prerequisite generation failed")
            raise HTTPException(status_code=503, detail="Построение связей не завершено")
        finally:
            await client.close()


@app.post("/graph/build")
async def build_course(request: BuildRequest):
    if len({m.id for m in request.materials}) != len(request.materials):
        raise HTTPException(status_code=422, detail="Повторный ID материала")
    if sum(len(m.markdown) for m in request.materials) > 8_000_000:
        raise HTTPException(status_code=413, detail="Суммарный размер курса превышает лимит")
    async with graph_slots:
        client = ModelClient()
        try:
            topics, reports = [], []
            for material in request.materials:
                if material.topics is not None:
                    for topic in material.topics:
                        for source in topic.sources:
                            if source.material_id != material.id or not 0 <= source.start < source.end <= len(material.markdown) or material.markdown[source.start:source.end] != source.text:
                                raise ValueError("Карточка не соответствует исходнику")
                        if topic.text != "\n\n".join(s.text for s in topic.sources):
                            raise ValueError("Текст темы не соответствует источникам")
                    topics.extend(material.topics)
                else:
                    result = await extract_topics(material.markdown, client.complete, material.id,
                                                  material.week, ExtractionSettings(max_chars=request.max_chars))
                    topics.extend(Topic.model_validate(t) for t in result["report"]["topics"])
                    reports.append({"material_id": material.id, **result["report"]})
            encoder = await encoder_for_topics(topics)
            graph = await build_links(topics, client.complete, encoder, request.per_requirement)
            graph["extraction_reports"] = reports
            return graph
        except Exception:
            logging.exception("Course build failed")
            raise HTTPException(status_code=503, detail="Граф не опубликован: этап не завершён")
        finally:
            await client.close()


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("course_graph.api:app", host="0.0.0.0", port=port, reload=False)
