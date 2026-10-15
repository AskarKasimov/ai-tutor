import os
import tempfile
import traceback
from fastapi import FastAPI, File, UploadFile, HTTPException
from fastapi.responses import JSONResponse

app = FastAPI(title="Marker OCR Service", version="1.0.0")

# Lazy-loaded converter
converter = None


def get_converter():
    global converter
    if converter is None:
        try:
            # Marker 0.3+ API
            from marker.converters.pdf import PdfConverter
            from marker.models import create_model_dict
            from marker.config.parser import ConfigParser

            config_parser = ConfigParser({"output_format": "markdown"})
            models = create_model_dict()
            converter = PdfConverter(
                config=config_parser.generate_config_dict(),
                artifact_dict=models,
                processor_list=config_parser.get_processors(),
                renderer=config_parser.get_renderer(),
            )
        except Exception as e:
            print(f"Warning: could not initialize full Marker converter: {e}")
            converter = "fallback"
    return converter


@app.get("/health")
def health():
    return {"status": "ok", "service": "ocr-marker"}


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
        # 1. Try Marker OCR if available
        conv = get_converter()
        if conv and conv != "fallback":
            try:
                rendered = conv(tmp_path)
                markdown_text = (
                    rendered.markdown if hasattr(rendered, "markdown") else str(rendered)
                )
                if markdown_text and markdown_text.strip():
                    return {"markdown": markdown_text}
            except Exception as e:
                print(f"Marker conversion failed, falling back to pypdf: {e}")

        # 2. Fallback using pypdf
        try:
            import pypdf

            reader = pypdf.PdfReader(tmp_path)
            pages_text = []
            for i, page in enumerate(reader.pages):
                page_content = page.extract_text() or ""
                if page_content.strip():
                    pages_text.append(f"## Слайд / Страница {i+1}\n\n{page_content}")
            if pages_text:
                return {"markdown": "\n\n".join(pages_text)}
        except Exception as e:
            print(f"pypdf extraction failed, falling back to pdftotext: {e}")

        # 3. Fallback using pdftotext
        try:
            import subprocess

            res = subprocess.run(
                ["pdftotext", tmp_path, "-"],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            if res.returncode == 0 and res.stdout.strip():
                return {"markdown": res.stdout}
        except Exception as e:
            print(f"pdftotext extraction failed: {e}")

        return JSONResponse(
            status_code=500,
            content={"error": "Failed to extract text from document by all available converters"},
        )
    finally:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)


if __name__ == "__main__":
    import uvicorn

    port = int(os.environ.get("PORT", 8000))
    uvicorn.run("app:app", host="0.0.0.0", port=port, reload=False)
