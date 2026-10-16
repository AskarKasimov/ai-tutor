import os
from fastapi import FastAPI, File, HTTPException, UploadFile
from pydantic import BaseModel, Field
import asyncio
import logging

from course_graph.cleaning.cleaner import clean_markdown
from course_graph.llm.client import ModelClient
from fastapi.responses import JSONResponse
from starlette.concurrency import run_in_threadpool

from course_graph.ocr.converter import convert_document as convert_source_document
from course_graph.topics.extract import ExtractionSettings, extract_topics
from course_graph.topics.graph import build_links
from course_graph.topics.models import Material, StrictModel, Topic
from course_graph.topics.retrieval import get_encoder

app = FastAPI(title="Course Graph Service", version="1.0.0")
ocr_slots = asyncio.Semaphore(1)


@app.get("/health")
def health():
    return {"status": "ok", "service": "course-graph"}


@app.post("/ocr/convert")
@app.post("/convert")
async def convert_document(file: UploadFile = File(...)):
    filename = file.filename or "document.pdf"
    content = await file.read()
    if not content:
        raise HTTPException(status_code=400, detail="Empty file uploaded")

    try:
        # Conversion runs CPU models and downloads weights on cold start. Keep
        # health, cleaning and gRPC clients responsive; serialize shared converter.
        async with ocr_slots:
            result = await run_in_threadpool(convert_source_document, filename, content)
        return result
    except Exception as e:
        return JSONResponse(
            status_code=500,
            content={"error": f"Failed to convert document: {str(e)}"},
        )


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
            async with asyncio.timeout(300):
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
            async with asyncio.timeout(300):
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
            async with asyncio.timeout(2400):
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
            async with asyncio.timeout(2400):
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
