from config_helpers import configure_settings
"""Tests for OCR source faithfulness, formula regions, caching, and absence of LLM guessing."""
import asyncio
import io
import json
import pytest
from types import SimpleNamespace as S

from course_graph.ocr.converter import convert_document, get_cached_document
from course_graph.ocr.regions import CanonicalBBox, FormulaRegionRecord, summarize_formula_regions
from course_graph.ocr.syntax import check_latex_syntax, has_substantive_conflict, normalize_formatting_only


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
    configure_settings(monkeypatch, tmp_path, cache__documents_dir=str(tmp_path))

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
    configure_settings(monkeypatch, tmp_path, cache__documents_dir=str(tmp_path))

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


def test_syntax_allows_arrows_and_audit_preserves_them():
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    # LaTeX arrow commands should not collide with \left or \right delimiter sizing
    for arrow_expr in [
        r"A \rightarrow B",
        r"A \leftarrow B",
        r"A \leftrightarrow B",
        r"\left( A \rightarrow B \right)",
        r"\left[ A \leftarrow B \right]",
        r"f(x) \rightharpoonup g(x)",
        r"X \longrightarrow Y",
        r"X \longleftarrow Y",
    ]:
        ok, err = check_latex_syntax(arrow_expr)
        assert ok is True, f"Failed on {arrow_expr}: {err}"
        assert err is None

    # Verify audit does not clear formulas with arrows
    bbox = BoundingBox(l=10, r=50, t=10, b=50, coord_origin=CoordOrigin.TOPLEFT)
    item = FormulaItem(self_ref="#/texts/0", orig="", text=r"A \rightarrow B",
                       prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document = S(texts=[item])
    regions, warnings = audit_and_retry_formulas(document, b"", None)

    assert len(regions) == 1
    assert regions[0].status == "recognized"
    assert regions[0].latex == r"A \rightarrow B"
    assert item.text == r"A \rightarrow B"
    assert not warnings


def test_syntax_allows_newline_arguments_and_invisible_delimiters():
    # Valid newline spacing in matrix and cases
    valid_expressions = [
        r"\begin{matrix} a \\[2pt] b \end{matrix}",
        r"\begin{cases} a & b \\[6pt] c & d \end{cases}",
        r"\left. x \right)",
        r"\left( x \right.",
        r"\left\{ \begin{aligned} x &= 1 \\[2pt] y &= 2 \end{aligned} \right.",
        r"\left. \frac{df}{dx} \right|_{x=0}",
        r"\left. (a + b) \right)",
        r"\left[ 0, 1 \right.",
        r"\left. [0, 1) \right.",
        r"\sqrt[3]{x + y}",
        r"\begin{array}[t]{c} x \\ y \end{array}",
    ]
    for expr in valid_expressions:
        ok, err = check_latex_syntax(expr)
        assert ok is True, f"Failed on {expr}: {err}"
        assert err is None

    # Invalid expressions with delimiter imbalance
    invalid_expressions = [
        (r"\left. (a + b \right)", "unbalanced_delimiters"),
        (r"\left( a + b) \right.", "unbalanced_delimiters"),
        (r"\right) \left(", "unbalanced_left_right"),
        (r"\left( x", "unbalanced_left_right"),
        (r"x \right)", "unbalanced_left_right"),
        (r"x + \left", "unbalanced_left_right"),
    ]
    for expr, expected_err in invalid_expressions:
        ok, err = check_latex_syntax(expr)
        assert ok is False, f"Expected invalid for {expr}"
        assert err == expected_err


def test_audit_preserves_newline_spacing_and_invisible_delimiters():
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    bbox = BoundingBox(l=10, r=50, t=10, b=50, coord_origin=CoordOrigin.TOPLEFT)
    f1 = r"\begin{matrix} a \\[2pt] b \end{matrix}"
    f2 = r"\left. x \right)"
    f3 = r"\left( x \right."
    f4 = r"\left\{ \begin{aligned} x &= 1 \\[2pt] y &= 2 \end{aligned} \right."

    items = [
        FormulaItem(self_ref=f"#/texts/{i}", orig="", text=expr,
                    prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
        for i, expr in enumerate([f1, f2, f3, f4])
    ]
    document = S(texts=items)
    regions, warnings = audit_and_retry_formulas(document, b"", None)

    assert len(regions) == 4
    for i, expr in enumerate([f1, f2, f3, f4]):
        assert regions[i].status == "recognized"
        assert regions[i].latex == expr
        assert items[i].text == expr
    assert not warnings


def test_native_formula_divergence_triggers_visual_retry_and_sets_suspect(monkeypatch):
    """P3: Valid raw_text that diverges from native PDF text must trigger visual retry.
    If visual retry also diverges from raw_text, status must be 'suspect' with reason_code 'suspect_divergence' (E10).
    """
    import pypdfium2
    from PIL import Image
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    class Bitmap:
        def to_pil(self): return Image.new("RGB", (20, 20))
        def close(self): pass

    class MockTextPage:
        def get_text_bounded(self, **kwargs):
            return "xi2"
        def close(self): pass

    class Page:
        def get_size(self): return 200, 300
        def get_height(self): return 300
        def get_textpage(self): return MockTextPage()
        def close(self): pass
        def render(self, **options): return Bitmap()

    class Pdf:
        def __len__(self): return 1
        def __getitem__(self, index): return Page()
        def close(self): pass

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())

    # Raw OCR text has damaged index ordering x_2^i (syntactically valid LaTeX, but diverges from native xi2)
    raw_formula = r"x_2^i"
    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)
    item = FormulaItem(self_ref="#/texts/0", orig="", text=raw_formula,
                       prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document = S(texts=[item])

    class CodeFormulaVlmModel:
        def __init__(self, engine):
            self.engine = engine
            self._post_process = lambda texts: texts

    # Case 1: Visual retry returns a divergent candidate x_k^3 -> suspect / suspect_divergence
    retry_candidate = r"x_k^3"
    class MockEngine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            return [S(text=retry_candidate, metadata={"num_tokens": 10})]

    vlm_stage = CodeFormulaVlmModel(MockEngine())
    model = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage])})

    regions, warnings = audit_and_retry_formulas(document, b"dummy_pdf", model)

    assert len(regions) == 1
    assert regions[0].status == "suspect"
    assert regions[0].reason_code == "suspect_divergence"
    assert regions[0].latex is None
    assert regions[0].candidate == retry_candidate
    assert regions[0].attempts == 2
    assert item.text == ""

    # Case 2: Visual retry confirms raw_formula (even with formatting variant x_{2}^{i}) -> recognized
    item2 = FormulaItem(self_ref="#/texts/0", orig="", text=raw_formula,
                        prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document2 = S(texts=[item2])
    class MatchingEngine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            return [S(text=r"x_{2}^{i}", metadata={"num_tokens": 10})]

    vlm_stage2 = CodeFormulaVlmModel(MatchingEngine())
    model2 = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage2])})

    regions2, warnings2 = audit_and_retry_formulas(document2, b"dummy_pdf", model2)
    assert len(regions2) == 1
    assert regions2[0].status == "recognized"
    assert regions2[0].latex == r"x_{2}^{i}"
    assert regions2[0].attempts == 2
    assert item2.text == r"x_{2}^{i}"


def test_realistic_native_text_layer_does_not_trigger_unneeded_retry(monkeypatch):
    """P2: Realistic flattened PDF text layers ('x1 + 2', '∑i=1n xi', 'α2') must NOT cause false divergence.
    They must be recognized on attempt 1 with zero retry inference calls.
    """
    import pypdfium2
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    class MockTextPage:
        def __init__(self, text):
            self._text = text
        def get_text_bounded(self, **kwargs):
            return self._text
        def close(self): pass

    class Page:
        def __init__(self, text):
            self._text = text
        def get_size(self): return 200, 300
        def get_height(self): return 300
        def get_textpage(self): return MockTextPage(self._text)
        def close(self): pass

    class Pdf:
        def __init__(self, text):
            self._text = text
        def __len__(self): return 1
        def __getitem__(self, index): return Page(self._text)
        def close(self): pass

    class UncalledEngine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            raise AssertionError("Retry inference must NOT be called for matching realistic native text!")

    class CodeFormulaVlmModel:
        def __init__(self, engine):
            self.engine = engine
            self._post_process = lambda texts: texts

    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)
    cases = [
        ("x_1 + 2", "x1 + 2"),
        (r"\sum_{i=1}^n x_i", "∑i=1n xi"),
        (r"\alpha^2", "α2"),
    ]

    for raw_latex, native_text in cases:
        monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _, t=native_text: Pdf(t))
        item = FormulaItem(self_ref="#/texts/0", orig="", text=raw_latex,
                           prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
        document = S(texts=[item])
        vlm_stage = CodeFormulaVlmModel(UncalledEngine())
        model = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage])})

        regions, warnings = audit_and_retry_formulas(document, b"dummy_pdf", model)
        assert len(regions) == 1
        assert regions[0].status == "recognized"
        assert regions[0].attempts == 1
        assert regions[0].latex == raw_latex
        assert item.text == raw_latex
        assert not warnings


def test_cosmetic_retry_formatting_differences_are_preserved_not_suspect(monkeypatch):
    """P2: If visual retry produces cosmetically different LaTeX (e.g. x_{1} vs x_1, \\dfrac vs \\frac,
    or inverted script order x^2_i vs x_i^2), the formula must NOT be discarded as suspect.
    """
    import pypdfium2
    from PIL import Image
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    class Bitmap:
        def to_pil(self): return Image.new("RGB", (20, 20))
        def close(self): pass

    class MockTextPage:
        def get_text_bounded(self, **kwargs):
            return ""
        def close(self): pass

    class Page:
        def get_size(self): return 200, 300
        def get_height(self): return 300
        def get_textpage(self): return MockTextPage()
        def close(self): pass
        def render(self, **options): return Bitmap()

    class Pdf:
        def __len__(self): return 1
        def __getitem__(self, index): return Page()
        def close(self): pass

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())

    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)

    class CodeFormulaVlmModel:
        def __init__(self, engine):
            self.engine = engine
            self._post_process = lambda texts: texts

    cases = [
        ("x_1 + 2", "x_{1} + 2"),
        (r"\frac{a}{b}", r"\dfrac{a}{b}"),
        ("x_i^2", "x^2_i"),
    ]

    for raw, retry_variant in cases:
        item = FormulaItem(self_ref="#/texts/0", orig="", text="",
                           prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
        document = S(texts=[item])

        class VariantEngine:
            def initialize(self): pass
            def predict_batch(self, inputs):
                return [S(text=retry_variant, metadata={"num_tokens": 10})]

        vlm_stage = CodeFormulaVlmModel(VariantEngine())
        model = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage])})

        regions, warnings = audit_and_retry_formulas(document, b"dummy_pdf", model)
        assert len(regions) == 1
        assert regions[0].status == "recognized"
        assert regions[0].attempts == 2
        assert regions[0].latex == retry_variant
        assert item.text == retry_variant


@pytest.mark.parametrize("raw, native", [
    (r"x_1 + 2", "x1 + 2"),
    (r"\alpha^2", "α2"),
    (r"a - b = c", "a − b = c"),
    (r"f(x) = x^2 - 1", "f(x) = x2 − 1"),
    (r"x \leq y", "x ≤ y"),
    (r"x \le y", "x ≤ y"),
    (r"a \neq b", "a ≠ b"),
    (r"\mu \pm \sigma", "μ ± σ"),
    (r"\varepsilon > 0", "ε > 0"),
    (r"\epsilon > 0", "ϵ > 0"),
    (r"\Sigma = 1", "Σ = 1"),
    (r"\sum_{i=1}^{n} x_i", "n ∑ i=1 xi"),
    (r"\frac{a}{b} = c", "a b = c"),
    (r"\left( x \right) \leq 1", "(x) ≤ 1"),
    (r"\forall x \in A", "∀x ∈ A"),
    (r"x \notin \emptyset", "x ∉ ∅"),
    (r"A \cup B \subseteq C", "A ∪ B ⊆ C"),
    (r"p \Rightarrow q", "p ⇒ q"),
    (r"f: A \to B", "f: A → B"),
    (r"a \cdot b = c", "a ⋅ b = c"),
    (r"X_1 + Y", "X1 + Y"),
])
def test_text_layer_of_a_correct_formula_is_not_a_conflict(raw, native):
    # A PDF text layer flattens scripts, uses U+2212 for minus, Unicode relations and
    # Greek glyphs, and does not keep the order of stacked limits.
    assert not has_substantive_conflict(raw, native)


@pytest.mark.parametrize("raw, native", [
    (r"a + b", "a − b"),          # different operator
    (r"x + y = 1", "x = 1"),      # dropped term
    (r"x^2", "x3"),               # different digit
    (r"x \leq y", "x ≥ y"),       # different relation
    (r"\alpha + 1", "β + 1"),     # different letter
    (r"x_2^i", "x_i^2"),          # LaTeX-like text layer keeps structure: inverted indices
    (r"x+y=z", "x=y+z"),          # same symbols, operators elsewhere
    (r"x+1", "X+1"),              # case is a different variable
    (r"\forall x P(x)", "∃x P(x)"),
    (r"x\in A", "x∉A"),
    (r"A \subseteq B", "A ⊇ B"),
    (r"p \land q", "p ∨ q"),
])
def test_text_layer_contradiction_is_still_a_conflict(raw, native):
    assert has_substantive_conflict(raw, native)


@pytest.mark.parametrize("a, b", [(r"x \le y", r"x \leq y"), (r"a \ne b", r"a \neq b"),
                                  (r"f: A \to B", r"f: A \rightarrow B")])
def test_command_spellings_are_formatting_only(a, b):
    assert normalize_formatting_only(a) == normalize_formatting_only(b)


def test_left_is_not_read_as_le():
    assert normalize_formatting_only(r"\left( x \right)") == "(x)"


@pytest.mark.parametrize("raw, native", [
    (r"\sin x = 0", "sin x = 0"),
    (r"\log x = 1", "log x = 1"),
    (r"\ln(x) + 1", "ln(x) + 1"),
    (r"\max_i x_i", "max i xi"),
    (r"a \bmod n", "a mod n"),
])
def test_operator_names_match_the_letters_of_the_text_layer(raw, native):
    assert not has_substantive_conflict(raw, native)


def test_operator_name_contradiction_is_still_a_conflict():
    assert has_substantive_conflict(r"\sin x = 0", "cos x = 0")
    assert has_substantive_conflict(r"\log x = 1", "log x = 2")


def test_braced_operator_argument_is_formatting_only():
    assert normalize_formatting_only(r"\sin{x}=0") == normalize_formatting_only(r"\sin x = 0")
    assert normalize_formatting_only(r"\sin{xy}") != normalize_formatting_only(r"\sin x y")


@pytest.mark.parametrize("latex", ["x^", "x_", "x_ ", "x_}", "{x_}", "a_ & b", r"x_ \\ y", "x_^2"])
def test_script_without_argument_is_truncated(latex):
    assert check_latex_syntax(latex) == (False, "truncated_subscript")


@pytest.mark.parametrize("latex", [r"x_1^2", r"a\_", r"\hat{x}^{2}", r"x'^2", r"\text{a\_b}"])
def test_script_with_argument_or_escaped_is_valid(latex):
    assert check_latex_syntax(latex)[0]


def _formula_audit(monkeypatch, raw, native, retry):
    import pypdfium2
    from PIL import Image
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas

    class Bitmap:
        def to_pil(self): return Image.new("RGB", (20, 20))
        def close(self): pass

    class TextPage:
        def get_text_bounded(self, **kwargs): return native
        def close(self): pass

    class Page:
        def get_size(self): return 200, 300
        def get_height(self): return 300
        def get_textpage(self): return TextPage()
        def close(self): pass
        def render(self, **options): return Bitmap()

    class Pdf:
        def __len__(self): return 1
        def __getitem__(self, index): return Page()
        def close(self): pass

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())
    calls = []

    class Engine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            calls.append(len(inputs))
            return [S(text=retry, metadata={"num_tokens": 10})]

    class CodeFormulaVlmModel:
        def __init__(self, engine):
            self.engine = engine
            self._post_process = lambda texts: texts

    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)
    item = FormulaItem(self_ref="#/texts/0", orig="", text=raw,
                       prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    model = S(initialized_pipelines={1: S(enrichment_pipe=[CodeFormulaVlmModel(Engine())])})
    regions, _ = audit_and_retry_formulas(S(texts=[item]), b"dummy_pdf", model)
    return regions, calls, item


def test_formula_matching_the_text_layer_with_an_operator_name_needs_no_retry(monkeypatch):
    regions, calls, item = _formula_audit(monkeypatch, r"\sin x = 0", "sin x = 0", r"\sin{x}=0")
    assert calls == []
    assert regions[0].status == "recognized" and regions[0].attempts == 1
    assert item.text == r"\sin x = 0"


@pytest.mark.parametrize("raw", ["x^", "x_"])
def test_dangling_script_is_retried_and_unresolved_when_the_retry_fails(monkeypatch, raw):
    regions, calls, item = _formula_audit(monkeypatch, raw, "", raw)
    assert calls == [1]
    assert regions[0].status == "unresolved" and regions[0].attempts == 2
    assert item.text == ""


def test_dangling_script_is_recognized_after_a_valid_retry(monkeypatch):
    regions, calls, item = _formula_audit(monkeypatch, "x^", "", "x^2")
    assert calls == [1]
    assert regions[0].status == "recognized" and item.text == "x^2"


def _real_code_stage(answers, engine=None):
    from docling.models.stages.code_formula.code_formula_vlm_model import CodeFormulaVlmModel
    from course_graph.ocr.enrichment import NativeSyntaxGuard

    class Engine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            return [S(text=text, metadata={"num_tokens": 5}) for text in answers[:len(inputs)]]

    stage = CodeFormulaVlmModel.__new__(CodeFormulaVlmModel)
    stage.enabled = True
    stage.engine = engine or Engine()
    stage.options = S(engine_options=S(engine_type="transformers"))
    stage._repetition_stopper = None
    return NativeSyntaxGuard(stage)


def _code_elements(texts):
    from PIL import Image
    from docling.datamodel.base_models import ItemAndImageEnrichmentElement
    from docling_core.types.doc import CodeItem, DocItemLabel
    return [ItemAndImageEnrichmentElement(item=CodeItem(self_ref=f"#/texts/{i}", label=DocItemLabel.CODE, orig=text, text=text),
                                          image=Image.new("RGB", (20, 20))) for i, text in enumerate(texts)]


def test_one_empty_code_answer_keeps_its_neighbours_answers():
    from course_graph.ocr.enrichment import CODE_LOST_WARNING
    guard = _real_code_stage(["", "print(42)"])
    items = list(guard(None, _code_elements(["", ""])))
    assert [item.text for item in items] == ["", "print(42)"]
    assert CODE_LOST_WARNING in guard.warnings
    assert not any("частично" in warning for warning in guard.warnings)


def test_rejected_code_answer_keeps_the_text_docling_had():
    guard = _real_code_stage(["", "print(42)"])
    items = list(guard(None, _code_elements(["pr1nt(1)", ""])))
    assert [item.text for item in items] == ["pr1nt(1)", "print(42)"]


def test_lost_code_is_cached_as_incomplete_content(monkeypatch, tmp_path):
    import course_graph.ocr.converter as converter
    from pypdf import PdfWriter

    configure_settings(monkeypatch, tmp_path, cache__documents_dir=str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    guard = _real_code_stage(["", "print(42)"])
    calls = []

    class Doc:
        pages = {1: None}
        texts = []
        def export_to_markdown(self, page_no=None):
            return "\n\n".join(f"```\n{item.text}\n```" for item in self.texts)

    class Converter:
        initialized_pipelines = {1: S(enrichment_pipe=[guard])}
        def initialize_pipeline(self, *args, **kwargs): return None
        def convert(self, path):
            calls.append(path)
            doc = Doc()
            doc.texts = list(guard(doc, _code_elements(["", ""])))
            return S(document=doc, status="success")

    monkeypatch.setattr(converter, "get_converter", lambda: Converter())
    monkeypatch.setattr("course_graph.ocr.enrichment.recover_native_programs", lambda document, content: [])
    monkeypatch.setattr("course_graph.ocr.enrichment.audit_and_retry_formulas", lambda document, content, model=None: ([], []))
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)

    first = converter.convert_document("code.pdf", stream.getvalue())
    assert "print(42)" in first["markdown"]
    assert first["report"]["work_complete"] is True
    assert first["report"]["content_complete"] is False
    second = converter.convert_document("code.pdf", stream.getvalue())
    assert second["report"]["cache_hit"] and second["report"]["content_complete"] is False
    assert len(calls) == 1


def test_failed_engine_initialization_is_not_cached_and_recovers(monkeypatch, tmp_path):
    import course_graph.ocr.converter as converter
    from pypdf import PdfWriter

    configure_settings(monkeypatch, tmp_path, cache__documents_dir=str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    inference = []

    class Engine:
        # Loading the weights fails on the first request and works afterwards.
        loads = 0
        def initialize(self):
            Engine.loads += 1
            if Engine.loads == 1:
                raise RuntimeError("weights are not readable")
        def predict_batch(self, inputs):
            inference.append(len(inputs))
            return [S(text="print(42)", metadata={"num_tokens": 5}) for _ in inputs]

    guard = _real_code_stage([], Engine())
    calls = []

    class Doc:
        pages = {1: None}
        texts = []
        def export_to_markdown(self, page_no=None):
            return "\n\n".join(f"```\n{item.text}\n```" for item in self.texts)

    class Converter:
        initialized_pipelines = {1: S(enrichment_pipe=[guard])}
        def initialize_pipeline(self, *args, **kwargs): return None
        def convert(self, path):
            calls.append(path)
            doc = Doc()
            doc.texts = list(guard(doc, _code_elements([""])))
            return S(document=doc, status="success")

    monkeypatch.setattr(converter, "get_converter", lambda: Converter())
    monkeypatch.setattr("course_graph.ocr.enrichment.recover_native_programs", lambda document, content: [])
    monkeypatch.setattr("course_graph.ocr.enrichment.audit_and_retry_formulas", lambda document, content, model=None: ([], []))
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)

    first = converter.convert_document("code.pdf", stream.getvalue())
    assert any("частично" in warning for warning in first["report"]["warnings"])
    assert first["report"]["work_complete"] is False
    assert first["report"]["content_complete"] is False
    assert inference == []

    second = converter.convert_document("code.pdf", stream.getvalue())
    assert second["report"]["cache_hit"] is False
    assert inference == [1]
    assert "print(42)" in second["markdown"]
    assert second["report"]["work_complete"] is True
    assert len(calls) == 2


@pytest.mark.parametrize("answer", ["<_Python_></code><end_of_utterance>", "</code><end_of_utterance>"])
def test_answer_of_only_special_tokens_is_rejected(answer):
    from course_graph.ocr.enrichment import CODE_LOST_WARNING
    guard = _real_code_stage([answer, "print(42)"])
    items = list(guard(None, _code_elements(["pr1nt(1)", ""])))
    assert [item.text for item in items] == ["pr1nt(1)", "print(42)"]
    assert CODE_LOST_WARNING in guard.warnings


def test_failed_inference_leaves_items_to_docling_without_code_loss():
    from course_graph.ocr.enrichment import CODE_LOST_WARNING

    class Engine:
        def initialize(self): pass
        def predict_batch(self, inputs): raise RuntimeError("CUDA out of memory")

    guard = _real_code_stage([], Engine())
    items = list(guard(None, _code_elements(["pr1nt(1)"])))
    assert [item.text for item in items] == ["pr1nt(1)"]
    assert CODE_LOST_WARNING not in guard.warnings
    assert any("частично" in warning for warning in guard.warnings)


@pytest.mark.parametrize("raw, native", [("x+y=z", "x=y+z"), ("x+1", "X+1"), ("P(x)=1", "p(x)=1")])
def test_text_layer_contradiction_sends_the_formula_to_a_retry(monkeypatch, raw, native):
    regions, calls, item = _formula_audit(monkeypatch, raw, native, raw)
    assert calls == [1]
    assert regions[0].attempts == 2
