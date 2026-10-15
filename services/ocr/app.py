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

    lecture_title = os.path.splitext(filename)[0]

    try:
        converter = get_converter()
        conv_res = converter.convert(tmp_path)
        doc = conv_res.document

        slides_text = [f"# {lecture_title}\n"]

        if hasattr(doc, "pages") and doc.pages:
            for page_no in sorted(doc.pages.keys()):
                slide_md = doc.export_to_markdown(page_no=page_no).strip()
                if slide_md:
                    slides_text.append(f"## Слайд {page_no}\n\n{slide_md}")
            markdown_text = "\n\n".join(slides_text)
        else:
            raw_md = doc.export_to_markdown().strip()
            markdown_text = f"# {lecture_title}\n\n{raw_md}" if raw_md else ""

        if markdown_text and markdown_text.strip():
            return {"markdown": markdown_text}

        # Fallback to pypdf if document export was empty
        import pypdf

        reader = pypdf.PdfReader(tmp_path)
        slides_text = [f"# {lecture_title}\n"]
        for i, page in enumerate(reader.pages):
            text = page.extract_text() or ""
            if text.strip():
                slides_text.append(f"## Слайд {i+1}\n\n{text}")
        return {"markdown": "\n\n".join(slides_text)}
    except Exception as e:
        traceback.print_exc()
        try:
            import pypdf

            reader = pypdf.PdfReader(tmp_path)
            slides_text = [f"# {lecture_title}\n"]
            for i, page in enumerate(reader.pages):
                text = page.extract_text() or ""
                if text.strip():
                    slides_text.append(f"## Слайд {i+1}\n\n{text}")
            if len(slides_text) > 1:
                return {"markdown": "\n\n".join(slides_text)}
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
