import os
from fastapi import FastAPI, File, HTTPException, UploadFile
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


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("course_graph.api:app", host="0.0.0.0", port=port, reload=False)
