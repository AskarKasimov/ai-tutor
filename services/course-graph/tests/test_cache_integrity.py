"""The OCR cache key, its entries and their writes stay consistent with the pipeline."""
import io
import json
from pathlib import Path
from types import SimpleNamespace as S

import pytest

import course_graph.ocr.converter as converter


def blank_pdf():
    from pypdf import PdfWriter
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)
    return stream.getvalue()


@pytest.fixture
def docling(monkeypatch, tmp_path):
    """A controlled Docling result and a recorder of the PDF-only steps."""
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    steps = []

    class Doc:
        pages = {1: None}
        texts = []
        def export_to_markdown(self, page_no=None):
            return "formula"

    class Converter:
        initialized_pipelines = {}
        def initialize_pipeline(self, *args): steps.append("initialize")
        def convert(self, path):
            steps.append("convert")
            return S(document=Doc(), status="success")

    def audit(document, content, model=None):
        steps.append("audit")
        return [], ["audited"]

    monkeypatch.setattr(converter, "get_converter", lambda: Converter())
    monkeypatch.setattr("course_graph.ocr.enrichment.recover_native_programs", lambda document, content: steps.append("recover") or [])
    monkeypatch.setattr("course_graph.ocr.enrichment.audit_and_retry_formulas", audit)
    return steps


def test_name_without_extension_is_audited_like_a_pdf(docling):
    content = blank_pdf()
    bare = converter.convert_document("lecture", content)
    assert docling == ["initialize", "convert", "recover", "audit"]
    assert "audited" in bare["report"]["warnings"]
    named = converter.convert_document("lecture.pdf", content)
    assert named["report"]["cache_hit"] and docling.count("convert") == 1
    assert named["report"]["warnings"] == bare["report"]["warnings"]
    assert named["markdown"].partition("\n")[2] == bare["markdown"].partition("\n")[2]


def test_name_without_extension_shares_the_pdf_parser():
    assert converter.document_parser("lecture") == converter.document_parser("lecture.PDF") == ".pdf"
    assert converter.document_parser("notes.md") == converter.document_parser("notes.txt") == "text"


@pytest.mark.parametrize("damage", [
    lambda report: report.update(warnings="wrong type"),
    lambda report: report.pop("warnings"),
    lambda report: report.update(warnings=[1]),
    lambda report: report.pop("engine"),
    lambda report: report.update(engine=None),
    lambda report: report.update(content_complete="yes"),
    lambda report: report.update(pages=True),
    lambda report: report.update(formula_summary={"detected": 1}),
    lambda report: report.update(formula_regions=[{"region_id": "r"}]),
    lambda report: report.update(formula_regions="none"),
])
def test_cache_entry_that_breaks_the_report_contract_is_a_miss(monkeypatch, tmp_path, damage):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    first = converter.convert_document("a.txt", b"exact source")
    assert not first["report"]["cache_hit"]
    assert converter.get_cached_document("a.txt", b"exact source")["report"]["cache_hit"]
    path = converter.get_document_cache_path(b"exact source", "a.txt")
    entry = json.loads(path.read_text(encoding="utf-8"))
    damage(entry["report"])
    path.write_text(json.dumps(entry), encoding="utf-8")
    assert converter.get_cached_document("a.txt", b"exact source") is None
    again = converter.convert_document("a.txt", b"exact source")
    assert not again["report"]["cache_hit"]
    assert isinstance(again["report"]["warnings"], list)
    assert converter.get_cached_document("a.txt", b"exact source")["report"]["cache_hit"]


def test_unknown_report_fields_are_kept(monkeypatch, tmp_path):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    converter.convert_document("a.txt", b"exact source")
    path = converter.get_document_cache_path(b"exact source", "a.txt")
    entry = json.loads(path.read_text(encoding="utf-8"))
    entry["report"]["future_field"] = {"any": "value"}
    path.write_text(json.dumps(entry), encoding="utf-8")
    assert converter.get_cached_document("a.txt", b"exact source")["report"]["future_field"] == {"any": "value"}


def leftovers(directory):
    return sorted(p.name for p in Path(directory).iterdir() if not p.name.endswith(".json"))


def test_failed_replace_leaves_no_temporary_file_and_keeps_the_old_entry(monkeypatch, tmp_path):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    data = {"markdown": "old", "report": {}}
    converter.cache_document_result(b"source", data, "a.txt")
    path = converter.get_document_cache_path(b"source", "a.txt")
    old = path.read_text(encoding="utf-8")

    def fail(self, target):
        raise OSError("rename failed")

    monkeypatch.setattr(Path, "replace", fail)
    for _ in range(2):
        converter.cache_document_result(b"source", {"markdown": "new", "report": {}}, "a.txt")
    assert leftovers(tmp_path) == []
    assert path.read_text(encoding="utf-8") == old


def test_failed_serialization_leaves_no_temporary_file(monkeypatch, tmp_path):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    converter.cache_document_result(b"source", {"markdown": "x", "report": {"bad": object()}}, "a.txt")
    assert list(Path(tmp_path).iterdir()) == []


def test_failing_cache_write_does_not_break_the_result(monkeypatch, tmp_path):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setattr(Path, "replace", lambda self, target: (_ for _ in ()).throw(OSError("disk full")))
    result = converter.convert_document("a.txt", b"exact source")
    assert result["markdown"].endswith("exact source")
    assert leftovers(tmp_path) == []
