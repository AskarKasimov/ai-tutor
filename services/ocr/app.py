import os
import tempfile
import traceback
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse
from docling.document_converter import DocumentConverter

app = FastAPI(title="Docling Document Parser Service", version="2.0.0")

_converter = None


def get_converter() -> DocumentConverter:
    global _converter
    if _converter is None:
        _converter = DocumentConverter()
    return _converter


@app.get("/health")
def health():
    return {"status": "ok", "service": "ocr-docling"}


@app.post("/convert")
async def convert_document(file: UploadFile = File(...)):
    filename = file.filename or "document.pdf"
    content = await file.read()
    if not content:
        raise HTTPException(status_code=400, detail="Empty file uploaded")

    # If it's plain text or markdown, return as-is
    lower = filename.lower()
    if lower.endswith((".txt", ".md", ".markdown")):
        try:
            return {"markdown": content.decode("utf-8")}
        except Exception:
            return {"markdown": content.decode("utf-8", errors="replace")}

    # Save to temp file
    suffix = os.path.splitext(filename)[1] or ".pdf"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(content)
        tmp_path = tmp.name

    try:
        converter = get_converter()
        conv_res = converter.convert(tmp_path)
        markdown_text = conv_res.document.export_to_markdown()
        if markdown_text and markdown_text.strip():
            return {"markdown": markdown_text}

        # Fallback to pypdf if document export was empty
        import pypdf

        reader = pypdf.PdfReader(tmp_path)
        pages = []
        for i, page in enumerate(reader.pages):
            text = page.extract_text() or ""
            if text.strip():
                pages.append(f"## Слайд / Страница {i+1}\n\n{text}")
        return {"markdown": "\n\n".join(pages)}
    except Exception as e:
        traceback.print_exc()
        try:
            import pypdf

            reader = pypdf.PdfReader(tmp_path)
            pages = []
            for i, page in enumerate(reader.pages):
                text = page.extract_text() or ""
                if text.strip():
                    pages.append(f"## Слайд / Страница {i+1}\n\n{text}")
            if pages:
                return {"markdown": "\n\n".join(pages)}
        except Exception:
            pass

        return JSONResponse(
            status_code=500,
            content={"error": f"Failed to convert document: {str(e)}"},
        )
    finally:
        if os.path.exists(tmp_path):
            try:
                os.remove(tmp_path)
            except Exception:
                pass


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("app:app", host="0.0.0.0", port=port, reload=False)
