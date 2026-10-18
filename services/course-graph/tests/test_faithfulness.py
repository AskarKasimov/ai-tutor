"""Tests for OCR source faithfulness, formula regions, caching, and absence of LLM guessing."""
import asyncio
import io
import json
import pytest
from types import SimpleNamespace as S

from course_graph.ocr.converter import convert_document, get_cached_document
from course_graph.ocr.regions import CanonicalBBox, FormulaRegionRecord, summarize_formula_regions
from course_graph.ocr.syntax import check_latex_syntax, normalize_formatting_only


def test_syntax_checker_catches_unbalanced_and_truncated_latex():
    # Valid syntax
    ok, err = check_latex_syntax(r"\frac{1}{n} \sum_{i=1}^n (y_i - \hat{y}_i)^2")
    assert ok is True and err is None

    ok, err = check_latex_syntax(r"\begin{matrix} a & b \\ c & d \end{matrix}")
    assert ok is True and err is None

    # Empty
    ok, err = check_latex_syntax("   ")
    assert ok is False and err == "empty_output"

    # Truncated with trailing escape
    ok, err = check_latex_syntax("x + y \\")
    assert ok is False and err == "truncated_trailing_escape"

    # Unbalanced braces
    ok, err = check_latex_syntax(r"\frac{1}{n")
    assert ok is False and err == "unbalanced_braces"

    # Unbalanced environments
    ok, err = check_latex_syntax(r"\begin{matrix} a & b")
    assert ok is False and err == "unbalanced_environments"


def test_formatting_normalization_preserves_mathematical_terms():
    # Cosmetic \left( \right) differences
    a = r"\left( x + y \right)^2 = x^2 + 2xy + y^2"
    b = r"(x + y)^2 = x^2 + 2xy + y^2"
    assert normalize_formatting_only(a) == normalize_formatting_only(b)

    # Substantive difference in coefficient or sign must NOT match
    diff_coeff = r"(2x + y)^2 = 4x^2 + 4xy + y^2"
    assert normalize_formatting_only(a) != normalize_formatting_only(diff_coeff)

    diff_sign = r"(x - y)^2 = x^2 - 2xy + y^2"
    assert normalize_formatting_only(a) != normalize_formatting_only(diff_sign)


def test_canonical_bbox_coordinate_transform():
    class DummyDoclingBBox:
        def __init__(self, l, t, r, b, origin):
            self.l = l
            self.t = t
            self.r = r
            self.b = b
            self.coord_origin = origin

    # Bottom-left to top-left on 600x800 page
    bbox_bl = DummyDoclingBBox(l=50, t=750, r=250, b=700, origin="BOTTOMLEFT")
    cbbox = CanonicalBBox.from_docling(bbox_bl, page_width=600, page_height=800)
    assert cbbox.coord_origin == "TOPLEFT"
    assert cbbox.left == 50.0
    assert cbbox.right == 250.0
    assert cbbox.top == 50.0   # 800 - 750
    assert cbbox.bottom == 100.0 # 800 - 700
    assert cbbox.width == 200.0
    assert cbbox.height == 50.0


def test_ocr_never_invokes_llm_completer_and_preserves_markers(monkeypatch, tmp_path):
    """B01 & B04: Verify zero calls to LLM model client during OCR conversion."""
    import course_graph.api as api

    class ForbiddenModelClient:
        def __init__(self, *args, **kwargs):
            raise AssertionError("CRITICAL: OCR must NEVER instantiate or call external LLM ModelClient!")

    monkeypatch.setattr(api, "ModelClient", ForbiddenModelClient)
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))

    md_content = (
        "# Лекция 1\n\n"
        "## Слайд 1\n\n"
        "Теорема 1: x + y = z.\n\n"
        "Формула ошибки:\n\n"
        "<!-- formula-not-decoded -->\n\n"
        "## Слайд 2\n\n"
        "Конец темы.\n"
    ).encode("utf-8")

    job, owner, future = api.ocr_jobs.reserve("test-job-no-llm", "lecture.md", md_content)
    asyncio.run(api.run_ocr_job(job, future, "lecture.md", md_content, requires_models=False))

    success, result = future.result()
    assert success is True
    # B02: Unresolved formula marker is preserved intact
    assert "<!-- formula-not-decoded -->" in result["markdown"]
    # B05: Neighboring text unchanged
    assert "Теорема 1: x + y = z." in result["markdown"]
    # B06: Slide boundary intact
    assert "## Слайд 2\n\nКонец темы." in result["markdown"]
    # C08: Schema version and report fields
    report = result["report"]
    assert report["schema_version"] == "ocr-report-v2"
    assert report["undecoded_formulas"] == 1
    assert "formula_summary" in report
    assert report["formula_summary"]["marker_count"] == 1
    assert report["content_complete"] is False
    assert report["work_complete"] is True


def test_stable_incomplete_result_is_cached_and_served_fast(monkeypatch, tmp_path):
    """F05 & G01: Stable incomplete results are cached and second request is a 0-inference hit."""
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))

    content = (
        "# Лекция по ML\n\n"
        "## Слайд 1\n\n"
        "Формула: <!-- formula-not-decoded -->"
    ).encode("utf-8")

    first = convert_document("lecture_v1.md", content)
    assert first["report"]["cache_hit"] is False
    assert first["report"]["undecoded_formulas"] == 1

    # Second call with same content uses cached content
    second = convert_document("lecture_v1.md", content)
    assert second["report"]["cache_hit"] is True
    assert second["report"]["undecoded_formulas"] == 1

    # F02: Adapt markdown lecture title to current filename if needed for converted PDFs
    from types import SimpleNamespace as S
    import io
    from pypdf import PdfWriter

    class DummyDoc:
        pages = {1: None}
        texts = []
        def export_to_markdown(self, page_no=None):
            return "Формула: <!-- formula-not-decoded -->"

    class DummyConverter:
        initialized_pipelines = {}
        def initialize_pipeline(self, *args, **kwargs):
            return S(enrichment_pipe=[])
        def convert(self, path):
            return S(document=DummyDoc(), status="success")

    monkeypatch.setattr("course_graph.ocr.converter.get_converter", lambda: DummyConverter())
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)
    pdf_content = stream.getvalue()

    pdf_first = convert_document("lecture_v1.pdf", pdf_content)
    assert pdf_first["report"]["cache_hit"] is False
    assert "# lecture_v1" in pdf_first["markdown"]

    pdf_second = convert_document("lecture_renamed.pdf", pdf_content)
    assert pdf_second["report"]["cache_hit"] is True
    assert "# lecture_renamed" in pdf_second["markdown"]


def test_syntax_allows_half_open_intervals_and_nested_environments():
    # Half-open intervals: [a, b), (a, b], \left[ 0, 1 \right)
    ok, err = check_latex_syntax(r"x \in [0, 1)")
    assert ok is True and err is None

    ok, err = check_latex_syntax(r"x \in (0, 1]")
    assert ok is True and err is None

    ok, err = check_latex_syntax(r"\left[ 0, 1 \right)")
    assert ok is True and err is None

    ok, err = check_latex_syntax(r"x \in [0, f(y))")
    assert ok is True and err is None

    # Nested environments
    nested = r"\begin{equation}\begin{aligned} x &= 1 \\ y &= 2 \end{aligned}\end{equation}"
    ok, err = check_latex_syntax(nested)
    assert ok is True and err is None

    # Mismatched environment
    mismatched = r"\begin{equation}\begin{aligned} x &= 1 \end{equation}"
    ok, err = check_latex_syntax(mismatched)
    assert ok is False and err == "unbalanced_environments"


def test_audit_preserves_valid_interval_and_nested_aligned():
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    bbox = BoundingBox(l=10, r=50, t=10, b=50, coord_origin=CoordOrigin.TOPLEFT)
    item1 = FormulaItem(self_ref="#/texts/0", orig="", text=r"x \in [0, 1)",
                        prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    item2 = FormulaItem(self_ref="#/texts/1", orig="",
                        text=r"\begin{equation}\begin{aligned} x &= 1 \end{aligned}\end{equation}",
                        prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document = S(texts=[item1, item2])
    regions, warnings = audit_and_retry_formulas(document, b"", None)

    assert len(regions) == 2
    assert regions[0].status == "recognized"
    assert regions[0].latex == r"x \in [0, 1)"
    assert item1.text == r"x \in [0, 1)"

    assert regions[1].status == "recognized"
    assert "aligned" in regions[1].latex
    assert "aligned" in item2.text
