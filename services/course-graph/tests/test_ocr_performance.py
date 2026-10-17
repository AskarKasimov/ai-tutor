"""Regression checks for shared work and faithful cache reuse."""
import asyncio
import hashlib
import json
import threading
from types import SimpleNamespace as S

import pytest

from course_graph.ocr import converter


def test_cache_separates_parsers_and_preserves_literal_titles(monkeypatch, tmp_path):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    content = b"# Literal title\n\n$x_1 + 2$\n```prolog\np(X) :- q(X).\n```"
    first = converter.convert_document("first.md", content)
    second = converter.convert_document("renamed.txt", content)
    assert first["markdown"] == second["markdown"] == content.decode()
    assert second["report"]["cache_hit"]
    assert converter.get_cached_document("different.pdf", content) is None


@pytest.mark.parametrize("damage", ["json", "report", "digest", "configuration", "unfinished"])
def test_invalid_cache_is_a_miss(monkeypatch, tmp_path, damage):
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    content = b"exact source"
    converter.convert_document("source.md", content)
    path = converter.get_document_cache_path(content, "source.md")
    data = json.loads(path.read_text())
    if damage == "json":
        path.write_text("{")
    else:
        if damage == "report":
            data["report"] = []
        elif damage == "digest":
            data["report"]["source_sha256"] = hashlib.sha256(b"other").hexdigest()
        elif damage == "configuration":
            data["report"]["version"] = "old"
        else:
            data["report"]["work_complete"] = False
        path.write_text(json.dumps(data))
    assert converter.get_cached_document("source.md", content) is None


def test_different_jobs_share_inference_without_disk_cache(monkeypatch):
    import course_graph.api as api
    from course_graph.ocr.jobs import OcrJobs
    monkeypatch.delenv("COURSE_GRAPH_OCR_CACHE_DIR", raising=False)
    monkeypatch.setattr(api, "ocr_jobs", OcrJobs())
    started, release = threading.Event(), threading.Event()
    calls = []

    def convert(filename, content):
        calls.append(filename)
        started.set()
        assert release.wait(5)
        return {"markdown": f"# {filename[:-4]}\n\n$x_1$\n```prolog\np(X).\n```",
                "report": {"engine": "docling", "warnings": [], "cache_hit": False}}

    monkeypatch.setattr(api, "convert_source_document", convert)

    async def run():
        monkeypatch.setattr(api, "ocr_slots", asyncio.Semaphore(1))
        jobs = [api.ocr_jobs.reserve(str(i), name, b"same PDF")
                for i, name in enumerate(["one.pdf", "two.pdf"])]
        first = asyncio.create_task(api.run_ocr_job(jobs[0][0], jobs[0][2], "one.pdf", b"same PDF", False))
        try:
            assert await asyncio.to_thread(started.wait, 2)
            second = asyncio.create_task(api.run_ocr_job(jobs[1][0], jobs[1][2], "two.pdf", b"same PDF", False))
            # Let the second job join the computation while inference is held.
            for _ in range(100):
                if jobs[1][0].state == "running":
                    break
                await asyncio.sleep(.01)
            assert jobs[1][0].state == "running"
        finally:
            release.set()
        await asyncio.gather(first, second)
        results = [job[2].result()[1] for job in jobs]
        assert results[0]["markdown"].startswith("# one\n")
        assert results[1]["markdown"].startswith("# two\n")
        assert results[0]["markdown"].partition("\n")[2] == results[1]["markdown"].partition("\n")[2]
        results[0]["report"]["warnings"].append("local")
        assert results[1]["report"]["warnings"] == []
        assert not api.ocr_flights

    asyncio.run(run())
    assert len(calls) == 1


def test_native_reader_reuses_pages_and_closes_evicted_resources(monkeypatch):
    import pypdfium2
    from course_graph.ocr.enrichment import NativePdfTextReader
    events = []

    class Text:
        def get_text_bounded(self, **bounds):
            assert bounds == dict(left=.5, bottom=1.5, right=3.5, top=4.5)
            return "p(X) :- q(X)."
        def close(self):
            events.append("close_text")

    class Page:
        def get_height(self): return 100
        def get_textpage(self):
            events.append("open_text")
            return Text()
        def close(self): events.append("close_page")

    class Pdf:
        def __len__(self): return 5
        def __getitem__(self, index):
            events.append("open_page")
            return Page()
        def close(self): events.append("close_pdf")

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())
    reader = NativePdfTextReader(b"pdf")
    box = S(to_bottom_left_origin=lambda _: S(l=1, b=2, r=3, t=4))
    for page_no in [1, 1, 2, 3, 4, 5]:
        assert reader(S(prov=[S(page_no=page_no, bbox=box)])) == "p(X) :- q(X)."
    assert len(reader._pages) == 4
    assert events.count("open_page") == events.count("open_text") == 5
    assert events.count("close_page") == events.count("close_text") == 1
    reader.close()
    assert events.count("close_page") == events.count("close_text") == 5
    assert events[-1] == "close_pdf"


def test_plain_text_does_not_wait_for_docling_slot(monkeypatch):
    import course_graph.api as api
    from course_graph.ocr.jobs import OcrJobs
    monkeypatch.setattr(api, "ocr_jobs", OcrJobs())
    monkeypatch.delenv("COURSE_GRAPH_OCR_CACHE_DIR", raising=False)

    async def run():
        monkeypatch.setattr(api, "ocr_slots", asyncio.Semaphore(1))
        async with api.ocr_slots:
            job, _, future = api.ocr_jobs.reserve("text", "source.md", b"# Source\n$x_1$")
            await asyncio.wait_for(api.run_ocr_job(job, future, "source.md", b"# Source\n$x_1$", False), 2)
            success, result = future.result()
            assert success and result["markdown"] == "# Source\n$x_1$"

    asyncio.run(run())


@pytest.mark.parametrize("origin", ["TOPLEFT", "BOTTOMLEFT"])
@pytest.mark.parametrize("failed", [False, True])
def test_visual_retry_uses_same_rectangle_and_releases_resources(monkeypatch, origin, failed):
    import pypdfium2
    from PIL import Image
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas
    events = []

    class Bitmap:
        def to_pil(self): return Image.new("RGB", (20, 20))
        def close(self): events.append("close_bitmap")

    class Page:
        def get_size(self): return 200, 300
        def close(self): events.append("close_page")
        def render(self, **options):
            assert options == dict(scale=3.0, crop=(16., 216., 116., 36.))
            return Bitmap()

    class Pdf:
        def __len__(self): return 1
        def __getitem__(self, index): return Page()
        def close(self): events.append("close_pdf")

    class CodeFormulaVlmModel:
        def __init__(self): self.engine = self
        def predict_batch(self, inputs):
            events.append("inference")
            assert len(inputs) == 1
            if failed:
                raise RuntimeError("engine temporarily unavailable")
            return [S(text="x_1 + 2")]

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())
    bbox = BoundingBox(l=20, r=80, t=40 if origin == "TOPLEFT" else 260,
                       b=80 if origin == "TOPLEFT" else 220, coord_origin=CoordOrigin(origin))
    item = FormulaItem(self_ref="#/texts/0", orig="", text="",
                       prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document = S(texts=[item])
    model = S(initialized_pipelines={1: S(enrichment_pipe=[CodeFormulaVlmModel()])})
    regions, warnings = audit_and_retry_formulas(document, b"pdf", model)
    assert regions[0].attempts == 2
    assert regions[0].status == ("failed_transient" if failed else "recognized")
    assert item.text == ("" if failed else "x_1 + 2")
    assert bool(warnings) == failed
    assert events.count("inference") == 1
    assert events.count("close_page") == 2
    assert events.count("close_bitmap") == 1
    assert events[-1] == "close_pdf"


def test_transient_fallback_is_not_a_permanent_cache(monkeypatch, tmp_path):
    from pypdf import PdfWriter
    import io
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    writer = PdfWriter()
    writer.add_blank_page(width=100, height=100)
    stream = io.BytesIO()
    writer.write(stream)
    content = stream.getvalue()
    monkeypatch.setattr(converter, "get_converter", lambda: (_ for _ in ()).throw(RuntimeError("temporary")))
    monkeypatch.setattr(converter, "_extract_with_pypdf", lambda *_: "# source\n\nPartial text")
    result = converter.convert_document("source.pdf", content)
    assert result["report"]["engine"] == "pypdf"
    assert result["report"]["work_complete"] is False
    assert converter.get_cached_document("source.pdf", content) is None


def test_prolog_layout_match_preserves_operators_and_quoted_text():
    from course_graph.ocr.enrichment import NativeSyntaxGuard
    native = "Solution 1.\ngte(X,Y) :- X =:= Y,write('X and Y are same')."
    flat = native.replace("\n", " ")
    matches = NativeSyntaxGuard._prolog_layout_matches
    assert matches(native, flat)
    assert not matches(native, flat.replace("=:=", "==:"))
    assert not matches(native, flat.replace("are same", "are  same"))
    assert not matches("if x:\n    print(x)", "if x: print(x)")
    assert not matches("p(X) :-\n    q(X).", "p(X) :- q(X).")


def test_generated_code_disagreement_with_native_is_visible():
    from course_graph.ocr.enrichment import NativeSyntaxGuard, CODE_MISMATCH_WARNING
    from docling_core.types.doc import DocItemLabel
    code = S(label=DocItemLabel.CODE, text="return route")

    class Stage:
        engine = S()
        def __call__(self, _, elements):
            for element in elements:
                element.item.text = "return on route"
                yield element.item

    # The different native text forces visual OCR; its result changes a token.
    guard = NativeSyntaxGuard(Stage(), lambda _: "route = current\nreturn route")
    assert list(guard(None, [S(item=code)]))[0].text == "return on route"
    assert CODE_MISMATCH_WARNING in guard.warnings


def test_converter_attaches_reader_when_initialize_pipeline_returns_none(monkeypatch):
    import io
    from pypdf import PdfWriter
    from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject
    from docling_core.types.doc import BoundingBox, CoordOrigin, CodeItem, ProvenanceItem
    from course_graph.ocr.enrichment import NativeSyntaxGuard
    writer = PdfWriter()
    page = writer.add_blank_page(width=300, height=300)
    font = DictionaryObject({NameObject("/Type"): NameObject("/Font"), NameObject("/Subtype"): NameObject("/Type1"), NameObject("/BaseFont"): NameObject("/Helvetica")})
    page[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"): DictionaryObject({NameObject("/F1"): writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b"BT /F1 12 Tf 20 20 Td (p\\(X\\) :- q\\(X\\).) Tj ET")
    page[NameObject("/Contents")] = writer._add_object(stream)
    content = io.BytesIO()
    writer.write(content)
    item = CodeItem(self_ref="#/texts/0", orig="p(X) :- q(X).", text="p(X) :- q(X).",
                    prov=[ProvenanceItem(page_no=1, charspan=(0, 13), bbox=BoundingBox(l=10,t=50,r=290,b=0,coord_origin=CoordOrigin.BOTTOMLEFT))])

    class Stage:
        engine = S()
        def __call__(self, doc, elements):
            raise AssertionError("Confirmed native code must skip generation")

    guard = NativeSyntaxGuard(Stage())
    document = S(pages={1: None}, texts=[item], pictures=[], export_to_markdown=lambda **_: item.text)

    class Converter:
        initialized_pipelines = {}
        def initialize_pipeline(self, _):
            self.initialized_pipelines[1] = S(enrichment_pipe=[guard])
            return None  # Actual Docling 2.139 public API.
        def convert(self, path):
            assert list(guard(document, [S(item=item)])) == [item]
            return S(document=document, status="success")

    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    monkeypatch.delenv("COURSE_GRAPH_OCR_CACHE_DIR", raising=False)
    monkeypatch.setattr(converter, "get_converter", Converter)
    result = converter.convert_document("native.pdf", content.getvalue())
    assert result["report"]["engine"] == "docling"
    assert "p(X) :- q(X)." in result["markdown"]
    assert guard.native_text_for is None
