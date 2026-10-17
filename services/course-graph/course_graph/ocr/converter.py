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


def configured_document_timeout():
    raw = os.environ.get("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT", "0")
    try:
        value = float(raw)
    except ValueError as error:
        raise ValueError("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT must be a non-negative number") from error
    if value < 0:
        raise ValueError("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT must be a non-negative number")
    # Docling treats None as no document-wide deadline; zero can mean immediate timeout.
    return value or None


def get_converter():
    global _converter
    if _converter is None:
        from docling.document_converter import DocumentConverter
        from docling.document_converter import PdfFormatOption
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import PdfPipelineOptions
        options = PdfPipelineOptions()
        from ..model_runtime import configured_ocr_threads
        options.accelerator_options.num_threads = configured_ocr_threads()
        options.do_code_enrichment = os.environ.get("COURSE_GRAPH_OCR_ENRICH_CODE", "true").lower() == "true"
        options.do_formula_enrichment = os.environ.get("COURSE_GRAPH_OCR_ENRICH_FORMULA", "true").lower() == "true"
        options.generate_picture_images = os.environ.get("COURSE_GRAPH_OCR_PICTURE_TEXT", "true").lower() == "true"
        # The model preset requests bfloat16. On AVX2-only CPUs this is emulated
        # and a small code region took minutes. Use native CPU float32 explicitly.
        from docling.datamodel.vlm_engine_options import TransformersVlmEngineOptions
        options.code_formula_options.engine_options = TransformersVlmEngineOptions(
            torch_dtype="float32", load_in_8bit=False, use_kv_cache=True, compile_model=False)
        options.document_timeout = configured_document_timeout()
        from docling.datamodel.pipeline_options import RapidOcrOptions
        ocr = RapidOcrOptions(backend="torch", force_full_page_ocr=os.environ.get("COURSE_GRAPH_OCR_FULL_PAGE", "false").lower() == "true")
        model_dir = Path(os.environ.get("COURSE_GRAPH_RAPID_CACHE_DIR", "/var/lib/course-graph/rapidocr"))
        paths = {"det_model_path": "PP-OCRv6_det_small.pth", "cls_model_path": "ch_ppocr_mobile_v2.0_cls_mobile.pth",
                 "rec_model_path": "PP-OCRv6_rec_small.pth", "rec_keys_path": "ppocrv6_dict.txt"}
        for field, name in paths.items():
            if (model_dir/name).exists():
                setattr(ocr, field, str(model_dir/name))
        options.ocr_options = ocr
        from .enrichment import audited_pipeline_class
        _converter = DocumentConverter(format_options={InputFormat.PDF: PdfFormatOption(
            pipeline_options=options, pipeline_cls=audited_pipeline_class())})
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


def get_ocr_configuration() -> dict:
    return {
        "version": "ocr-v6-native-syntax",
        "with_docling": os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true"),
        "picture_text": os.environ.get("COURSE_GRAPH_OCR_PICTURE_TEXT", "true"),
        "document_timeout": os.environ.get("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT", "0"),
        "enrich_code": os.environ.get("COURSE_GRAPH_OCR_ENRICH_CODE", "true"),
        "enrich_formula": os.environ.get("COURSE_GRAPH_OCR_ENRICH_FORMULA", "true"),
        "full_page_ocr": os.environ.get("COURSE_GRAPH_OCR_FULL_PAGE", "false"),
    }


def get_document_cache_path(filename: str, content: bytes) -> Optional[Path]:
    cache_dir = os.environ.get("COURSE_GRAPH_OCR_CACHE_DIR")
    if not cache_dir:
        return None
    source_hash = hashlib.sha256(content).hexdigest()
    configuration = get_ocr_configuration()
    key = hashlib.sha256((source_hash + filename + json.dumps(configuration, sort_keys=True)).encode()).hexdigest()
    return Path(cache_dir) / f"{key}.json"


def cache_document_result(filename: str, content: bytes, data: dict) -> None:
    cache_path = get_document_cache_path(filename, content)
    if not cache_path:
        return
    cache_path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(mode="w", dir=cache_path.parent, delete=False) as f:
        json.dump(data, f, ensure_ascii=False)
        temporary = Path(f.name)
    temporary.replace(cache_path)


def convert_document(filename: str, content: bytes) -> dict:
    """Convert a document (PDF, Markdown, text) to Markdown with LaTeX math."""
    if not content:
        raise ValueError("Empty file content")
    started = time.monotonic()
    source_hash = hashlib.sha256(content).hexdigest()
    configuration = get_ocr_configuration()
    cache_path = get_document_cache_path(filename, content)
    if cache_path and cache_path.exists():
        cached = json.loads(cache_path.read_text())
        cached["report"]["cache_hit"] = True
        return cached

    def result(markdown, engine, pages=None, exported_pages=None, warnings=None, picture_ocr=None):
        report = {"engine": engine, "source_sha256": source_hash, "cache_hit": False,
                  "seconds": round(time.monotonic()-started, 3), "pages": pages,
                  "exported_pages": exported_pages, "warnings": warnings or [], "picture_ocr": picture_ocr,
                  "code_blocks": len(re.findall(r"^```", markdown, re.M))//2,
                  "undecoded_formulas": markdown.count("<!-- formula-not-decoded -->"),
                  "image_placeholders": markdown.count("<!-- image -->"), **configuration}
        if report["undecoded_formulas"]:
            report["warnings"].append("Часть формул не распознана; такие маркеры не являются учебным содержанием")
        data = {"markdown": markdown, "report": report}
        # Do not permanently cache a degraded fallback after transient model errors.
        degraded = bool(picture_ocr and picture_ocr.get("failed_regions")) or any(
            "частично" in w for w in report["warnings"]
        )
        if cache_path and engine in {"docling", "text"} and not degraded:
            cache_document_result(filename, content, data)
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
                return result(protect_prolog_literals(markdown), "pypdf", warnings=["Облегчённое извлечение: код/формулы из изображений не распознаны"])
            raise RuntimeError("Лёгкая сборка извлекает текстовый PDF; для сканов нужна полная сборка с Docling")

        # 1. Try Docling
        fallback_reason = "Docling не вернул текст"
        try:
            converter = get_converter()
            from .enrichment import NativePdfTextReader, NativeSyntaxGuard, enrichment_warnings
            enrichment_warnings(converter, reset=True)
            native_warnings = []
            if lower.endswith(".pdf"):
                from docling.datamodel.base_models import InputFormat

                pipeline = converter.initialize_pipeline(InputFormat.PDF)
                reader = NativePdfTextReader(content)
                guards = [stage for stage in getattr(pipeline, "enrichment_pipe", [])
                          if isinstance(stage, NativeSyntaxGuard)]
                for guard in guards:
                    guard.native_text_for = reader
                try:
                    conv_res = converter.convert(tmp_path)
                finally:
                    for guard in guards:
                        guard.native_text_for = None
                    reader.close()
            else:
                conv_res = converter.convert(tmp_path)
            doc = conv_res.document
            if lower.endswith(".pdf"):
                try:
                    from .enrichment import recover_native_programs
                    native_warnings = recover_native_programs(doc, content)
                except Exception as error:
                    native_warnings = ["Проверка исходного синтаксиса PDF выполнена частично: " + str(error)[:200]]

            slides_text = [f"# {lecture_title}\n"]
            exported_pages = []
            picture_ocr = None
            if hasattr(doc, "pages") and doc.pages:
                page_markdown = {n: doc.export_to_markdown(page_no=n).strip() for n in sorted(doc.pages)}
                recovered = {}
                if configuration["picture_text"].lower() == "true":
                    from .pictures import picture_texts
                    recovered, picture_ocr = picture_texts(doc, converter, page_markdown)
                for page_no in sorted(doc.pages.keys()):
                    slide_md = page_markdown[page_no]
                    for picture_id, lines in recovered.get(page_no, []):
                        slide_md += f"\n\n### Текст изображения {picture_id} (OCR)\n\n" + "\n".join(lines)
                    if slide_md:
                        exported_pages.append(page_no)
                        slides_text.append(f"## Слайд {page_no}\n\n{slide_md}")
                markdown_text = "\n\n".join(slides_text) if len(slides_text)>1 else ""
            else:
                raw_md = doc.export_to_markdown().strip()
                markdown_text = f"# {lecture_title}\n\n{raw_md}" if raw_md else ""

            if markdown_text and markdown_text.strip():
                status = str(getattr(conv_res, "status", ""))
                warnings = enrichment_warnings(converter) + native_warnings
                if "partial" in status.lower():
                    warnings.append("Docling завершил документ частично")
                markdown_text = protect_prolog_literals(markdown_text)
                if "<!-- image -->" in markdown_text:
                    warnings.append("Подписи изображений извлечены отдельно, если распознаны; смысл и пространственная структура схем не восстановлены текстовым OCR")
                if picture_ocr and picture_ocr["failed_regions"]:
                    warnings.append("Часть изображений не удалось обработать текстовым OCR")
                return result(markdown_text, "docling", len(doc.pages) if hasattr(doc, "pages") else None, exported_pages, warnings, picture_ocr)
        except Exception as error:
            fallback_reason = type(error).__name__ + ": " + str(error)[:300]
            traceback.print_exc()

        # 2. Fallback to pypdf without code duplication
        pypdf_text = _extract_with_pypdf(tmp_path, lecture_title)
        if pypdf_text:
            return result(protect_prolog_literals(pypdf_text), "pypdf", warnings=["Docling недоступен: " + fallback_reason,
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


def protect_prolog_literals(markdown: str) -> str:
    """Fence literal rules/queries so translation cannot rename predicates."""
    if not re.search(r"prolog|:-", markdown, re.I):
        return markdown
    pattern = re.compile(r"(?:[a-z]\w*\([^\n]*?\)\s*:-\s*[^\n]+?\.|\?-\s*[^\n]+?\.|[A-Z]\w*\s*:-\s*[A-Z]\w*\.)")
    fact = re.compile(r"^(?:#+\s*)?([a-z]\w*\([^\n]*?\)\.)(\s*(?:\[.*\])?)$")
    lines = []
    fence = None
    for line in markdown.splitlines():
        marker = re.match(r"^\s*(`{3,}|~{3,})", line)
        if marker:
            if fence is None:
                fence = marker.group(1)[0]
            elif marker.group(1)[0] == fence:
                fence = None
            lines.append(line)
            continue
        if fence:
            lines.append(line)
            continue
        match = pattern.search(line) or fact.fullmatch(line)
        if not match:
            lines.append(line)
            continue
        start, end = match.span(1) if match.re is fact else match.span()
        # A heading marker on a native query/fact is layout, not code syntax.
        prefix = re.sub(r"^#+\s*", "", line[:start]).strip()
        if prefix and prefix not in {"-", "*", "+"}:
            lines.append(prefix)
        lines.extend(["", "```prolog", line[start:end], "```", ""])
        if line[end:].strip():
            lines.append(line[end:].strip())
    return "\n".join(lines)
