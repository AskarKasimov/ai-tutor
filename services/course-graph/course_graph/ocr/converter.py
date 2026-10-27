import os
import tempfile
import traceback
import hashlib
import json
import time
import re
from pathlib import Path

from course_graph.config import get_settings
from typing import Optional

_converter = None


def configured_document_timeout():
    return get_settings().ocr.document_timeout_seconds or None


def get_converter():
    global _converter
    if _converter is None:
        from docling.document_converter import DocumentConverter
        from docling.document_converter import PdfFormatOption
        from docling.datamodel.base_models import InputFormat
        from docling.datamodel.pipeline_options import PdfPipelineOptions
        options = PdfPipelineOptions()
        from docling.datamodel.accelerator_options import AcceleratorDevice
        from ..model_runtime import configured_ocr_device, configured_ocr_threads, require_ocr_device
        require_ocr_device()
        options.accelerator_options.num_threads = configured_ocr_threads()
        # Layout, RapidOCR and the formula/code model all follow this device.
        options.accelerator_options.device = (AcceleratorDevice.CUDA if configured_ocr_device() == "gpu"
                                              else AcceleratorDevice.CPU)
        options.do_code_enrichment = get_settings().ocr.enrich_code
        options.do_formula_enrichment = get_settings().ocr.enrich_formula
        options.generate_picture_images = get_settings().ocr.picture_text
        # The model preset requests bfloat16. On AVX2-only CPUs this is emulated
        # and a small code region took minutes. Use native CPU float32 explicitly.
        from docling.datamodel.vlm_engine_options import TransformersVlmEngineOptions
        options.code_formula_options.engine_options = TransformersVlmEngineOptions(
            torch_dtype="float32", load_in_8bit=False, use_kv_cache=True, compile_model=False)
        options.document_timeout = configured_document_timeout()
        from docling.datamodel.pipeline_options import RapidOcrOptions
        ocr = RapidOcrOptions(backend="torch", force_full_page_ocr=get_settings().ocr.full_page)
        model_dir = Path(get_settings().cache.rapidocr_dir)
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


import logging

logger = logging.getLogger(__name__)


def get_ocr_configuration() -> dict:
    return {
        "version": "ocr-v9-content-cache",
        "audit_version": "formula-audit-v5",
        "with_docling": os.environ.get("COURSE_GRAPH_WITH_DOCLING", "true"),
        "picture_text": str(get_settings().ocr.picture_text).lower(),
        "document_timeout": get_settings().ocr.document_timeout,
        "enrich_code": str(get_settings().ocr.enrich_code).lower(),
        "enrich_formula": str(get_settings().ocr.enrich_formula).lower(),
        "full_page_ocr": str(get_settings().ocr.full_page).lower(),
    }


def document_parser(filename: str) -> str:
    """The one parser choice used by the cache key and the whole pipeline.

    Markdown and TXT share the literal UTF-8 path; a name without an extension
    is a PDF, exactly like a .pdf name.
    """
    suffix = Path(filename).suffix.lower()
    if suffix in {".txt", ".md", ".markdown"}:
        return "text"
    return suffix or ".pdf"


def document_key(content: bytes, filename: str = "document.pdf") -> str:
    # Extensions select different parsers even for identical bytes; PDF names only affect the title.
    parser = document_parser(filename)
    source_hash = hashlib.sha256(content).hexdigest()
    configuration = get_ocr_configuration()
    return hashlib.sha256((parser + source_hash + json.dumps(configuration, sort_keys=True)).encode()).hexdigest()


def get_document_cache_path(content: bytes, filename: str = "document.pdf") -> Optional[Path]:
    cache_dir = get_settings().cache.documents_dir
    if not cache_dir:
        return None
    return Path(cache_dir) / f"{document_key(content, filename)}.json"


def document_for_filename(data: dict, filename: str) -> dict:
    from copy import deepcopy
    data = deepcopy(data)
    if data["report"].get("engine") in {"docling", "pypdf"} and data["markdown"].startswith("# "):
        _, separator, body = data["markdown"].partition("\n")
        data["markdown"] = f"# {os.path.splitext(filename)[0]}" + separator + body
    return data


_INT = "int"
_REPORT_FIELDS = {
    "engine": str, "source_sha256": str, "warnings": list, "work_complete": bool,
    "content_complete": bool, "schema_version": str, "cache_hit": bool, "seconds": (int, float),
    "pages": (_INT, None), "exported_pages": (list, None), "code_blocks": _INT,
    "undecoded_formulas": _INT, "image_placeholders": _INT, "formula_regions": list, "formula_summary": dict,
}
_REQUIRED_REPORT_FIELDS = ("engine", "source_sha256", "warnings", "work_complete", "content_complete")
_SUMMARY_FIELDS = ("detected", "recognized", "unresolved", "suspect", "failed_transient", "marker_count")
_REGION_FIELDS = {"region_id": str, "source_sha256": str, "page": _INT, "bbox": dict, "kind": str,
                  "status": str, "method": str, "verification": str, "attempts": _INT}


def _is(value, kinds) -> bool:
    for kind in kinds if isinstance(kinds, tuple) else (kinds,):
        if kind is None and value is None:
            return True
        # bool is an int in Python; counters and pages must not accept it.
        if kind == _INT and isinstance(value, int) and not isinstance(value, bool):
            return True
        if isinstance(kind, type) and isinstance(value, kind):
            return True
    return False


def valid_cached_report(report: dict) -> bool:
    """A cache hit must satisfy the OcrReport contract; otherwise it is a miss."""
    if any(key not in report for key in _REQUIRED_REPORT_FIELDS):
        return False
    if any(key in report and not _is(report[key], kind) for key, kind in _REPORT_FIELDS.items()):
        return False
    if not all(isinstance(warning, str) for warning in report["warnings"]):
        return False
    if report.get("exported_pages") is not None and not all(_is(page, _INT) for page in report["exported_pages"]):
        return False
    summary = report.get("formula_summary")
    if summary is not None and not all(_is(summary.get(key), _INT) for key in _SUMMARY_FIELDS):
        return False
    return all(isinstance(region, dict) and all(_is(region.get(key), kind) for key, kind in _REGION_FIELDS.items())
               for region in report.get("formula_regions", []))


def get_cached_document(filename: str, content: bytes) -> Optional[dict]:
    cache_path = get_document_cache_path(content, filename)
    if not cache_path or not cache_path.exists():
        return None
    try:
        cached = json.loads(cache_path.read_text(encoding="utf-8"))
        if not isinstance(cached, dict) or not isinstance(cached.get("markdown"), str) or not isinstance(cached.get("report"), dict):
            return None
        report = cached["report"]
        if report.get("source_sha256") != hashlib.sha256(content).hexdigest() or report.get("work_complete") is not True:
            return None
        if any(report.get(key) != value for key, value in get_ocr_configuration().items()):
            return None
        if not valid_cached_report(report):
            return None
        cached = document_for_filename(cached, filename)
        cached["report"]["cache_hit"] = True
        return cached
    except Exception:
        return None


def cache_document_result(content: bytes, data: dict, filename: str = "document.pdf") -> None:
    cache_path = get_document_cache_path(content, filename)
    if not cache_path:
        return
    temporary = None
    try:
        cache_path.parent.mkdir(parents=True, exist_ok=True)
        with tempfile.NamedTemporaryFile(mode="w", dir=cache_path.parent, delete=False, encoding="utf-8") as f:
            # Remember the path first: a failing dump must not leave an orphan either.
            temporary = Path(f.name)
            json.dump(data, f, ensure_ascii=False)
        # The rename is atomic: readers see the whole old or the whole new entry.
        temporary.replace(cache_path)
        temporary = None
    except Exception as err:
        logger.warning("Failed to write document cache: %s", err)
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)


def convert_document(filename: str, content: bytes) -> dict:
    """Convert a document (PDF, Markdown, text) to Markdown with LaTeX math."""
    if not content:
        raise ValueError("Empty file content")

    cached = get_cached_document(filename, content)
    if cached is not None:
        return cached

    started = time.monotonic()
    source_hash = hashlib.sha256(content).hexdigest()
    configuration = get_ocr_configuration()

    def result(markdown, engine, pages=None, exported_pages=None, warnings=None, picture_ocr=None, formula_regions=None):
        from .regions import summarize_formula_regions
        regions_list = formula_regions or []
        summary = summarize_formula_regions(
            regions_list,
            marker_count=markdown.count("<!-- formula-not-decoded"),
            warnings=warnings or [],
            picture_ocr=picture_ocr,
        )
        report = {
            "schema_version": "ocr-report-v2",
            "engine": engine,
            "source_sha256": source_hash,
            "cache_hit": False,
            "seconds": round(time.monotonic()-started, 3),
            "pages": pages,
            "exported_pages": exported_pages,
            "warnings": warnings or [],
            "picture_ocr": picture_ocr,
            "code_blocks": len(re.findall(r"^```", markdown, re.M))//2,
            "undecoded_formulas": markdown.count("<!-- formula-not-decoded"),
            "image_placeholders": markdown.count("<!-- image -->"),
            "formula_regions": [r.to_dict() for r in regions_list],
            **summary,
            **configuration,
        }
        if report["undecoded_formulas"]:
            report["warnings"].append("Часть формул не распознана; такие маркеры не являются учебным содержанием")
        from .enrichment import CODE_LOST_WARNING, CODE_MISMATCH_WARNING
        if CODE_MISMATCH_WARNING in report["warnings"] or CODE_LOST_WARNING in report["warnings"]:
            report["content_complete"] = False
        data = {"markdown": markdown, "report": report}
        # Stable unresolved formulas are reusable; transient engine failures, picture OCR failures,
        # and partial documents must get another opportunity on the next request.
        has_failed_pictures = bool(picture_ocr and picture_ocr.get("failed_regions", 0) > 0)
        has_failed_transient = any(r.get("status") == "failed_transient" for r in report.get("formula_regions", []))
        transient = (
            any("частично" in warning.lower() for warning in report["warnings"])
            or has_failed_pictures
            or has_failed_transient
            or not report.get("work_complete", True)
        )
        fallback = engine == "pypdf" and configuration["with_docling"].lower() != "false"
        if transient or fallback:
            report["work_complete"] = False
            report["content_complete"] = False
        if report["work_complete"] and engine in {"docling", "text", "pypdf"}:
            cache_document_result(content, data, filename)
        return data


    parser = document_parser(filename)
    is_pdf = parser == ".pdf"
    if parser == "text":
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
            if is_pdf:
                from docling.datamodel.base_models import InputFormat

                # Docling 2.139 initializes in place and returns None.
                converter.initialize_pipeline(InputFormat.PDF)
                reader = NativePdfTextReader(content)
                guards = [stage for pipeline in converter.initialized_pipelines.values()
                          for stage in getattr(pipeline, "enrichment_pipe", [])
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
            formula_regions = []
            if is_pdf:
                try:
                    from .enrichment import recover_native_programs
                    native_warnings = recover_native_programs(doc, content)
                except Exception as error:
                    native_warnings = ["Проверка исходного синтаксиса PDF выполнена частично: " + str(error)[:200]]
                try:
                    from .enrichment import audit_and_retry_formulas
                    formula_regions, audit_warnings = audit_and_retry_formulas(doc, content, converter)
                    native_warnings.extend(audit_warnings)
                except Exception as error:
                    native_warnings.append("Проверка и повторное распознавание формул выполнены частично: " + str(error)[:200])

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
                return result(markdown_text, "docling", len(doc.pages) if hasattr(doc, "pages") else None, exported_pages, warnings, picture_ocr, formula_regions=formula_regions)
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
