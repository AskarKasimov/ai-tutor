import os
import tempfile
import traceback
import hashlib
import json
import time
import re
from pathlib import Path
from typing import Optional

_converter = None


def get_converter():
    global _converter
    if _converter is None:
        from docling.document_converter import DocumentConverter
        from docling.document_converter import PdfFormatOption
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import PdfPipelineOptions
        options = PdfPipelineOptions()
        options.do_code_enrichment = os.environ.get("COURSE_GRAPH_OCR_ENRICH_CODE", "true").lower() == "true"
        options.do_formula_enrichment = os.environ.get("COURSE_GRAPH_OCR_ENRICH_FORMULA", "true").lower() == "true"
        # The model preset requests bfloat16. On AVX2-only CPUs this is emulated
        # and a small code region took minutes. Use native CPU float32 explicitly.
        from docling.datamodel.vlm_engine_options import TransformersVlmEngineOptions
        options.code_formula_options.engine_options = TransformersVlmEngineOptions(
            torch_dtype="float32", load_in_8bit=False, use_kv_cache=True, compile_model=False)
        options.document_timeout = float(os.environ.get("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT", "480"))
        from docling.datamodel.pipeline_options import RapidOcrOptions
        ocr = RapidOcrOptions(backend="torch", force_full_page_ocr=os.environ.get("COURSE_GRAPH_OCR_FULL_PAGE", "false").lower() == "true")
        model_dir = Path(os.environ.get("COURSE_GRAPH_RAPID_CACHE_DIR", "/var/lib/course-graph/rapidocr"))
        paths = {"det_model_path": "PP-OCRv6_det_small.pth", "cls_model_path": "ch_ppocr_mobile_v2.0_cls_mobile.pth",
                 "rec_model_path": "PP-OCRv6_rec_small.pth", "rec_keys_path": "ppocrv6_dict.txt"}
        for field, name in paths.items():
            if (model_dir/name).exists():
                setattr(ocr, field, str(model_dir/name))
        options.ocr_options = ocr
        _converter = DocumentConverter(format_options={InputFormat.PDF: PdfFormatOption(pipeline_options=options)})
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


def convert_document(filename: str, content: bytes) -> dict:
    """Convert a document (PDF, Markdown, text) to Markdown with LaTeX math."""
    if not content:
        raise ValueError("Empty file content")
    started = time.monotonic()
    source_hash = hashlib.sha256(content).hexdigest()
    configuration = {"version": "ocr-v3-cpu-fp32", "with_docling": os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true"),
                     "enrich_code": os.environ.get("COURSE_GRAPH_OCR_ENRICH_CODE", "true"),
                     "enrich_formula": os.environ.get("COURSE_GRAPH_OCR_ENRICH_FORMULA", "true"),
                     "full_page_ocr": os.environ.get("COURSE_GRAPH_OCR_FULL_PAGE", "false")}
    cache_path = None
    cache_dir = os.environ.get("COURSE_GRAPH_OCR_CACHE_DIR")
    if cache_dir:
        key = hashlib.sha256((source_hash+filename+json.dumps(configuration, sort_keys=True)).encode()).hexdigest()
        cache_path = Path(cache_dir)/f"{key}.json"
        if cache_path.exists():
            cached = json.loads(cache_path.read_text())
            cached["report"]["cache_hit"] = True
            return cached

    def result(markdown, engine, pages=None, exported_pages=None, warnings=None):
        report = {"engine": engine, "source_sha256": source_hash, "cache_hit": False,
                  "seconds": round(time.monotonic()-started, 3), "pages": pages,
                  "exported_pages": exported_pages, "warnings": warnings or [],
                  "code_blocks": len(re.findall(r"^```", markdown, re.M))//2,
                  "undecoded_formulas": markdown.count("<!-- formula-not-decoded -->"),
                  "image_placeholders": markdown.count("<!-- image -->"), **configuration}
        if report["undecoded_formulas"]:
            report["warnings"].append("Часть формул не распознана; такие маркеры не являются учебным содержанием")
        data = {"markdown": markdown, "report": report}
        # Do not permanently cache a degraded fallback after transient model errors.
        if cache_path and engine in {"docling", "text"} and not any("частично" in w for w in report["warnings"]):
            cache_path.parent.mkdir(parents=True, exist_ok=True)
            with tempfile.NamedTemporaryFile(mode="w", dir=cache_path.parent, delete=False) as f:
                json.dump(data, f, ensure_ascii=False)
                temporary = Path(f.name)
            temporary.replace(cache_path)
        return data

    lower = filename.lower()
    if lower.endswith((".txt", ".md", ".markdown")):
        try:
            return result(content.decode("utf-8"), "text")
        except Exception:
            return result(content.decode("utf-8", errors="replace"), "text", warnings=["Недопустимые UTF-8 символы заменены"])

    suffix = os.path.splitext(filename)[1] or ".pdf"
    with tempfile.NamedTemporaryFile(suffix=suffix, delete=False) as tmp:
        tmp.write(content)
        tmp_path = tmp.name

    lecture_title = os.path.splitext(filename)[0]

    try:
        if os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true").lower() == "false":
            markdown = _extract_with_pypdf(tmp_path, lecture_title)
            if markdown:
                return result(markdown, "pypdf", warnings=["Облегчённое извлечение: код/формулы из изображений не распознаны"])
            raise RuntimeError("Лёгкая сборка извлекает текстовый PDF; для сканов нужна полная сборка с Docling")

        # 1. Try Docling
        fallback_reason = "Docling не вернул текст"
        try:
            converter = get_converter()
            conv_res = converter.convert(tmp_path)
            doc = conv_res.document

            slides_text = [f"# {lecture_title}\n"]
            exported_pages = []
            if hasattr(doc, "pages") and doc.pages:
                for page_no in sorted(doc.pages.keys()):
                    slide_md = doc.export_to_markdown(page_no=page_no).strip()
                    if slide_md:
                        exported_pages.append(page_no)
                        slides_text.append(f"## Слайд {page_no}\n\n{slide_md}")
                markdown_text = "\n\n".join(slides_text) if len(slides_text)>1 else ""
            else:
                raw_md = doc.export_to_markdown().strip()
                markdown_text = f"# {lecture_title}\n\n{raw_md}" if raw_md else ""

            if markdown_text and markdown_text.strip():
                status = str(getattr(conv_res, "status", ""))
                warnings = [] if "partial" not in status.lower() else ["Docling завершил документ частично"]
                if "<!-- image -->" in markdown_text:
                    warnings.append("В документе есть изображения: их смысл и структура схем не восстановлены текстовым OCR")
                return result(markdown_text, "docling", len(doc.pages) if hasattr(doc, "pages") else None, exported_pages, warnings)
        except Exception as error:
            fallback_reason = type(error).__name__ + ": " + str(error)[:300]
            traceback.print_exc()

        # 2. Fallback to pypdf without code duplication
        pypdf_text = _extract_with_pypdf(tmp_path, lecture_title)
        if pypdf_text:
            return result(pypdf_text, "pypdf", warnings=["Docling недоступен: " + fallback_reason,
                                                       "Fallback не гарантирует распознавание кода/формул/текста из изображений"])

        raise RuntimeError(f"Не удалось извлечь текст из файла {filename}")
    finally:
        if os.path.exists(tmp_path):
            try:
                os.remove(tmp_path)
            except Exception:
                pass


def convert_document_to_markdown(filename: str, content: bytes) -> str:
    """Compatibility wrapper for existing consumers expecting a string."""
    return convert_document(filename, content)["markdown"]
