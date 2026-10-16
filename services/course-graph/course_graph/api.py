import os
from fastapi import FastAPI, File, HTTPException, UploadFile
from pydantic import BaseModel, Field
import asyncio

from course_graph.cleaning.cleaner import clean_markdown
from course_graph.llm.client import ModelClient
from fastapi.responses import JSONResponse

from course_graph.ocr.converter import convert_document_to_markdown

app = FastAPI(title="Course Graph Service", version="1.0.0")


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
        markdown = convert_document_to_markdown(filename, content)
        return {"markdown": markdown}
    except Exception as e:
        return JSONResponse(
            status_code=500,
            content={"error": f"Failed to convert document: {str(e)}"},
        )


class CleanRequest(BaseModel):
    markdown: str = Field(min_length=1, max_length=2_000_000)


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


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("course_graph.api:app", host="0.0.0.0", port=port, reload=False)
