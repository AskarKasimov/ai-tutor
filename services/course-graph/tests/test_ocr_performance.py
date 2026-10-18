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


def test_incomplete_job_allows_retry_with_same_job_id():
    from course_graph.ocr.jobs import OcrJobs

    jobs = OcrJobs()
    content = b"content"
    job, owner, future = jobs.reserve("job-1", "doc.pdf", content)
    assert owner is True

    # Finish with incomplete work (work_complete=False)
    incomplete_result = {
        "markdown": "# doc\n\nPartial",
        "report": {"engine": "pypdf", "work_complete": False, "warnings": ["partial fallback"]}
    }
    jobs.finish(job, future, incomplete_result)
    assert job.state == "succeeded"
    assert job.stage == "incomplete"

    # Repeat reserve with the same job_id must grant a new attempt (owner=True)
    job2, owner2, future2 = jobs.reserve("job-1", "doc.pdf", content)
    assert owner2 is True
    assert future2 is not None
    assert job2.state == "queued"

    # Now finish job2 with complete work (work_complete=True)
    complete_result = {
        "markdown": "# doc\n\nComplete",
        "report": {"engine": "docling", "work_complete": True, "warnings": []}
    }
    jobs.finish(job2, future2, complete_result)
    assert job2.state == "succeeded"

    # Third call: now that work is complete, reserve returns idempotent cached result
    job3, owner3, future3 = jobs.reserve("job-1", "doc.pdf", content)
    assert owner3 is False
    assert future3 is None
    assert job3.result["markdown"] == "# doc\n\nComplete"


def test_failed_picture_ocr_marks_work_incomplete_and_prevents_cache(monkeypatch, tmp_path):
    from pypdf import PdfWriter
    import io
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")

    writer = PdfWriter()
    writer.add_blank_page(width=100, height=100)
    stream = io.BytesIO()
    writer.write(stream)
    content = stream.getvalue()

    class DummyDoc:
        pages = {1: None}
        texts = []
        pictures = [S(prov=[S(page_no=1)])]
        def export_to_markdown(self, page_no=None):
            return "Some slide text"

    class DummyConverter:
        initialized_pipelines = {}
        def initialize_pipeline(self, *args, **kwargs):
            return S(enrichment_pipe=[])
        def convert(self, path):
            return S(document=DummyDoc(), status="success")

    monkeypatch.setattr(converter, "get_converter", lambda: DummyConverter())
    # Mock picture_texts to simulate image OCR failure
    def mock_picture_texts(*args, **kwargs):
        return {}, {"regions": 1, "lines": 0, "failed_regions": 1}

    import course_graph.ocr.pictures as pictures
    monkeypatch.setattr(pictures, "picture_texts", mock_picture_texts)

    result = converter.convert_document("picture_fail.pdf", content)
    assert result["report"]["work_complete"] is False
    assert result["report"]["content_complete"] is False
    assert result["report"]["picture_ocr"]["failed_regions"] == 1
    # Ensure it was NOT cached
    assert converter.get_cached_document("picture_fail.pdf", content) is None


def test_job_retry_clears_memory_and_tracks_actual_stored_bytes():
    from course_graph.ocr.jobs import OcrJobs

    jobs = OcrJobs(max_jobs=10, max_result_bytes=500)
    content = b"sample pdf content"
    incomplete_result = {
        "markdown": "# A",
        "report": {"work_complete": False, "warnings": ["temp"]},
    }
    encoded_size = len(json.dumps(incomplete_result, ensure_ascii=False, separators=(",", ":")).encode("utf-8"))

    # 6 retry attempts of the same incomplete job
    for _ in range(6):
        job, owner, future = jobs.reserve("job-retry", "doc.pdf", content)
        assert owner is True
        jobs.finish(job, future, incomplete_result)
        # Invariant: tracked _result_bytes must always match the sum of bytes of jobs currently holding results
        assert jobs._result_bytes == sum(j.result_bytes for j in jobs._jobs.values() if j.result is not None)
        assert jobs._result_bytes == encoded_size

    # Complete attempt
    job, owner, future = jobs.reserve("job-retry", "doc.pdf", content)
    complete_result = {
        "markdown": "# A",
        "report": {"work_complete": True, "warnings": []},
    }
    complete_size = len(json.dumps(complete_result, ensure_ascii=False, separators=(",", ":")).encode("utf-8"))
    jobs.finish(job, future, complete_result)
    assert jobs._result_bytes == sum(j.result_bytes for j in jobs._jobs.values() if j.result is not None)
    assert jobs._result_bytes == complete_size
    assert job.result is not None

    # New distinct job also fits in memory without premature eviction
    job_b, owner_b, future_b = jobs.reserve("job-b", "b.pdf", b"other content")
    jobs.finish(job_b, future_b, complete_result)
    assert jobs._result_bytes == complete_size * 2
    assert jobs.get("job-retry")["result"] is not None
    assert jobs.get("job-b")["result"] is not None


def test_legacy_cache_prior_to_formula_audit_fix_is_a_cache_miss(monkeypatch, tmp_path):
    """P2: Cache entries created before formula audit fixes (e.g. ocr-v8) must result in a cache miss.

    Simulates the exact scenario:
    1. A legacy version (ocr-v8-content-cache) created a cache file where a valid formula
       was stripped to empty or marker.
    2. The old cache file was stored with work_complete=True and version="ocr-v8-content-cache".
    3. The updated system must NOT serve this stale entry (get_cached_document returns None).
    4. Old cache files are safely ignored without needing deletion.
    """
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    content = b"# Document\n\n$$\\left. x \\right)$$"
    source_sha = hashlib.sha256(content).hexdigest()

    # Calculate the old key under ocr-v8 configuration
    legacy_config = {
        "version": "ocr-v8-content-cache",
        "with_docling": "true",
        "picture_text": "true",
        "document_timeout": "0",
        "enrich_code": "true",
        "enrich_formula": "true",
        "full_page_ocr": "false",
    }
    legacy_key = hashlib.sha256(("text" + source_sha + json.dumps(legacy_config, sort_keys=True)).encode()).hexdigest()
    legacy_path = tmp_path / f"{legacy_key}.json"

    # Write legacy cached document where formula was mistakenly marked unresolved / erased
    legacy_data = {
        "markdown": "# Document\n\n<!-- formula-not-decoded -->",
        "report": {
            "source_sha256": source_sha,
            "work_complete": True,
            "content_complete": False,
            "engine": "text",
            "warnings": [],
            "undecoded_formulas": 1,
            **legacy_config,
        }
    }
    legacy_path.write_text(json.dumps(legacy_data), encoding="utf-8")

    # 1. get_cached_document must return None (cache miss, old file ignored)
    assert converter.get_cached_document("source.md", content) is None

    # 2. Even if legacy file was copied to current path, version mismatch triggers cache miss
    current_path = converter.get_document_cache_path(content, "source.md")
    current_path.write_text(json.dumps(legacy_data), encoding="utf-8")
    assert converter.get_cached_document("source.md", content) is None

    # 3. New conversion produces fresh result and writes to the updated cache version
    new_result = converter.convert_document("source.md", content)
    assert new_result["report"]["version"] == "ocr-v9-content-cache"
    assert new_result["report"]["audit_version"] == "formula-audit-v2"
    assert converter.get_cached_document("source.md", content) is not None


def test_deterministic_crop_retry_failure_is_unresolved_and_allows_caching(monkeypatch, tmp_path):
    """P2: Deterministic empty output or token limit during crop retry must be marked unresolved,
    must NOT pollute warnings with transient 'частично', and must keep work_complete=True so it caches to disk.
    """
    import pypdfium2
    from PIL import Image
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import audit_and_retry_formulas, AuditedEngine
    import course_graph.ocr.converter as converter

    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))

    class Bitmap:
        def to_pil(self): return Image.new("RGB", (20, 20))
        def close(self): pass

    class Page:
        def get_size(self): return 200, 300
        def close(self): pass
        def render(self, **options): return Bitmap()

    class Pdf:
        def __len__(self): return 1
        def __getitem__(self, index): return Page()
        def close(self): pass

    monkeypatch.setattr(pypdfium2, "PdfDocument", lambda _: Pdf())

    class CodeFormulaVlmModel:
        def __init__(self, engine):
            self.engine = engine
            self._post_process = lambda texts: texts

    # Case 1: Empty output on retry crop
    class EmptyEngine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            return [S(text="", metadata={"num_tokens": 5})]

    global_warnings = []
    audited = AuditedEngine(EmptyEngine(), global_warnings)

    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)
    item1 = FormulaItem(self_ref="#/texts/0", orig="", text="",
                        prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document1 = S(texts=[item1])
    vlm_stage1 = CodeFormulaVlmModel(audited)
    model1 = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage1])})

    regions1, warnings1 = audit_and_retry_formulas(document1, b"dummy_pdf", model1)

    assert len(regions1) == 1
    assert regions1[0].status == "unresolved"
    assert regions1[0].reason_code == "empty_output"
    assert regions1[0].attempts == 2
    assert not any("частично" in w for w in global_warnings)
    assert not any("частично" in w for w in warnings1)

    # Case 2: Token limit reached on retry crop
    class TokenLimitEngine:
        def initialize(self): pass
        def predict_batch(self, inputs):
            return [S(text=r"\frac{1}{x} + ...", metadata={"num_tokens": 2048})]

    audited2 = AuditedEngine(TokenLimitEngine(), global_warnings)
    item2 = FormulaItem(self_ref="#/texts/0", orig="", text="",
                        prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])
    document2 = S(texts=[item2])
    vlm_stage2 = CodeFormulaVlmModel(audited2)
    model2 = S(initialized_pipelines={1: S(enrichment_pipe=[vlm_stage2])})

    regions2, warnings2 = audit_and_retry_formulas(document2, b"dummy_pdf", model2)

    assert len(regions2) == 1
    assert regions2[0].status == "unresolved"
    assert regions2[0].reason_code == "token_limit"
    assert regions2[0].attempts == 2
    assert not any("частично" in w for w in global_warnings)
    assert not any("частично" in w for w in warnings2)


def test_main_pass_token_limit_caches_document_and_second_call_hits_cache(monkeypatch, tmp_path):
    """P2: Token limit reached during Docling's main enrichment pass must not mark the document transient.
    The document must be completed with work_complete=True, saved to disk cache, and subsequent calls
    must return a 0-inference cache hit.
    """
    from types import SimpleNamespace as S
    from docling_core.types.doc import BoundingBox, CoordOrigin, FormulaItem, ProvenanceItem
    from course_graph.ocr.enrichment import NativeSyntaxGuard
    import course_graph.ocr.converter as converter

    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")

    inference_calls = 0

    class MainEngine:
        options = S(use_kv_cache=True)
        vlm_model = S(generation_config=S(use_cache=False))
        def initialize(self): pass
        def predict_batch(self, inputs):
            nonlocal inference_calls
            inference_calls += len(inputs)
            # Reached token limit in main pass
            return [S(text="truncated formula...", metadata={"num_tokens": 2048})]

    bbox = BoundingBox(l=20, r=80, t=40, b=80, coord_origin=CoordOrigin.TOPLEFT)
    item = FormulaItem(self_ref="#/texts/0", orig="", text="",
                       prov=[ProvenanceItem(page_no=1, charspan=(0, 0), bbox=bbox)])

    class DummyDoc:
        pages = {1: None}
        texts = [item]
        def export_to_markdown(self, page_no=None):
            return "# Lecture\n\n$$\\frac{1}{2}$$"

    class MockPipeline:
        def __init__(self):
            raw_stage = S(engine=MainEngine(), _post_process=lambda texts: texts,
                          options=S(engine_options=S(engine_type="local")),
                          _repetition_stopper=None, enabled=True)
            self.enrichment_pipe = [NativeSyntaxGuard(raw_stage)]

    class MockConverter:
        initialized_pipelines = {}
        def initialize_pipeline(self, *args, **kwargs):
            return None
        def convert(self, path):
            stage = self.initialized_pipelines[1].enrichment_pipe[0]
            try:
                stage.engine.predict_batch([S(max_new_tokens=2048)])
            except ValueError:
                pass
            return S(document=DummyDoc(), status="success")

    mock_converter = MockConverter()
    mock_converter.initialized_pipelines = {1: MockPipeline()}

    monkeypatch.setattr("course_graph.ocr.converter.get_converter", lambda: mock_converter)

    from pypdf import PdfWriter
    import io
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)
    pdf_content = stream.getvalue()

    # First run: executes OCR main pass + 1 targeted crop retry
    first = converter.convert_document("lecture_token_limit.pdf", pdf_content)
    assert first["report"]["cache_hit"] is False
    assert first["report"]["work_complete"] is True
    # The warning from token limit must NOT contain "частично"
    assert not any("частично" in w.lower() for w in first["report"]["warnings"])
    assert inference_calls == 2

    # Second run: must hit disk cache with ZERO additional inference calls
    second = converter.convert_document("lecture_token_limit.pdf", pdf_content)
    assert second["report"]["cache_hit"] is True
    assert second["report"]["work_complete"] is True
    assert inference_calls == 2
