import os
import tempfile
import traceback
from typing import Optional

_converter = None


def get_converter():
    global _converter
    if _converter is None:
        from docling.document_converter import DocumentConverter

        _converter = DocumentConverter()
    return _converter


def _extract_with_pypdf(tmp_path: str, lecture_title: str) -> Optional[str]:
    try:
        import pypdf

        reader = pypdf.PdfReader(tmp_path)
        slides_text = [f"# {lecture_title}\n"]
        for i, page in enumerate(reader.pages):
            text = page.extract_text() or ""
            if text.strip():
                slides_text.append(f"## Слайд {i + 1}\n\n{text}")
        if len(slides_text) > 1:
            return "\n\n".join(slides_text)
    except Exception:
        traceback.print_exc()
    return None


def convert_document_to_markdown(filename: str, content: bytes) -> str:
    """Convert a document (PDF, Markdown, text) to Markdown with LaTeX math."""
    if not content:
        raise ValueError("Empty file content")

    lower = filename.lower()
    if lower.endswith((".txt", ".md", ".markdown")):
        try:
            return content.decode("utf-8")
        except Exception:
            return content.decode("utf-8", errors="replace")

    suffix = os.path.splitext(filename)[1] or ".pdf"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(content)
        tmp_path = tmp.name

    lecture_title = os.path.splitext(filename)[0]

    try:
        if os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true").lower() == "false":
            markdown = _extract_with_pypdf(tmp_path, lecture_title)
            if markdown:
                return markdown
            raise RuntimeError("Лёгкая сборка извлекает текстовый PDF; для сканов нужна полная сборка с Docling")

        # 1. Try Docling
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
                return markdown_text
        except Exception:
            traceback.print_exc()

        # 2. Fallback to pypdf without code duplication
        pypdf_text = _extract_with_pypdf(tmp_path, lecture_title)
        if pypdf_text:
            return pypdf_text

        raise RuntimeError(f"Не удалось извлечь текст из файла {filename}")
    finally:
        if os.path.exists(tmp_path):
            try:
                os.remove(tmp_path)
            except Exception:
                pass
