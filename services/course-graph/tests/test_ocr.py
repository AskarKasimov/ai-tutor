from fastapi.testclient import TestClient
from course_graph.api import app
from course_graph.ocr.converter import convert_document_to_markdown


def test_health():
    client = TestClient(app)
    response = client.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok", "service": "course-graph"}


def test_convert_markdown_text():
    content = b"# Test Lecture\n\nFormula: $E = mc^2$"
    md = convert_document_to_markdown("lecture.md", content)
    assert "# Test Lecture" in md
    assert "$E = mc^2$" in md


def test_api_convert_markdown():
    client = TestClient(app)
    response = client.post(
        "/ocr/convert",
        files={"file": ("lecture.md", b"# Markdown Lecture", "text/markdown")},
    )
    assert response.status_code == 200
    data = response.json()
    assert "markdown" in data
    assert "# Markdown Lecture" in data["markdown"]


def test_light_build_extracts_text_pdf_without_docling(monkeypatch):
    import io
    from pypdf import PdfWriter
    from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject
    import course_graph.ocr.converter as converter

    writer = PdfWriter()
    page = writer.add_blank_page(width=300, height=300)
    font = DictionaryObject({NameObject("/Type"): NameObject("/Font"), NameObject("/Subtype"): NameObject("/Type1"), NameObject("/BaseFont"): NameObject("/Helvetica")})
    page[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"): DictionaryObject({NameObject("/F1"): writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b"BT /F1 12 Tf 20 20 Td (A star search) Tj ET")
    page[NameObject("/Contents")] = writer._add_object(stream)
    content = io.BytesIO()
    writer.write(content)
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "false")
    def unexpected_docling():
        raise AssertionError("Light build must not initialize Docling")
    monkeypatch.setattr(converter, "get_converter", unexpected_docling)
    markdown = converter.convert_document_to_markdown("lecture.pdf", content.getvalue())
    assert "A star search" in markdown
    assert "## Слайд 1" in markdown


def test_light_build_reports_pdf_without_extractable_text(monkeypatch):
    import io
    import pytest
    from pypdf import PdfWriter

    writer = PdfWriter()
    writer.add_blank_page(width=300, height=300)
    content = io.BytesIO()
    writer.write(content)
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "false")
    with pytest.raises(RuntimeError, match="для сканов нужна полная сборка"):
        convert_document_to_markdown("scan.pdf", content.getvalue())


def test_text_ocr_cache_records_exact_source_digest(monkeypatch, tmp_path):
    import hashlib
    from course_graph.ocr.converter import convert_document
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    content = b"# Source\n\nA definition."
    first = convert_document("lecture.md", content)
    second = convert_document("lecture.md", content)
    assert not first["report"]["cache_hit"] and second["report"]["cache_hit"]
    assert second["markdown"] == content.decode()
    assert second["report"]["source_sha256"] == hashlib.sha256(content).hexdigest()


def test_docling_ocr_caches_result_with_undecoded_formulas(monkeypatch, tmp_path):
    import course_graph.ocr.converter as converter
    from types import SimpleNamespace as S

    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", str(tmp_path))
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")

    class DummyDoc:
        pages = {1: None}
        texts = []
        def export_to_markdown(self, page_no=None):
            return "Определение: <!-- formula-not-decoded --> в формуле."

    class DummyConverter:
        initialized_pipelines = {}
        def initialize_pipeline(self, *args, **kwargs):
            return S(enrichment_pipe=[])
        def convert(self, path):
            return S(document=DummyDoc(), status="success")

    monkeypatch.setattr(converter, "get_converter", lambda: DummyConverter())
    import io
    from pypdf import PdfWriter
    writer = PdfWriter()
    writer.add_blank_page(width=72, height=72)
    stream = io.BytesIO()
    writer.write(stream)
    content = stream.getvalue()

    first = converter.convert_document("lecture.pdf", content)
    assert not first["report"]["cache_hit"]
    assert first["report"]["undecoded_formulas"] == 1

    second = converter.convert_document("lecture.pdf", content)
    assert second["report"]["cache_hit"]
    assert second["report"]["undecoded_formulas"] == 1


def test_cpu_enrichment_uses_native_precision(monkeypatch):
    import docling.document_converter as docling
    import course_graph.ocr.converter as converter
    captured = {}
    def constructor(**kwargs):
        captured.update(kwargs)
        return object()
    monkeypatch.setattr(docling, "DocumentConverter", constructor)
    monkeypatch.setattr(converter, "_converter", None)
    converter.get_converter()
    options = next(iter(captured["format_options"].values())).pipeline_options
    assert options.do_code_enrichment and options.do_formula_enrichment
    assert options.code_formula_options.engine_options.torch_dtype == "float32"
    assert options.code_formula_options.engine_options.use_kv_cache


def test_zero_document_timeout_means_unlimited_and_negative_is_invalid(monkeypatch):
    import pytest
    from course_graph.ocr.converter import configured_document_timeout
    monkeypatch.setenv("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT", "0")
    assert configured_document_timeout() is None
    monkeypatch.setenv("COURSE_GRAPH_OCR_DOCUMENT_TIMEOUT", "-1")
    with pytest.raises(ValueError, match="non-negative"):
        configured_document_timeout()


def test_slow_ocr_keeps_health_responsive(monkeypatch):
    import asyncio
    import threading
    import httpx
    import course_graph.api as api
    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "false")
    started, release = threading.Event(), threading.Event()
    def slow(*args):
        started.set()
        assert release.wait(3)
        return {"markdown": "text", "report": {"engine": "test"}}
    monkeypatch.setattr(api, "convert_source_document", slow)
    async def run():
        async with httpx.AsyncClient(transport=httpx.ASGITransport(app=api.app), base_url="http://test") as client:
            job = asyncio.create_task(client.post("/ocr/convert", files={"file": ("real.pdf", b"PDF")}))
            try:
                assert await asyncio.to_thread(started.wait, 2)
                health = await asyncio.wait_for(client.get("/health"), .5)
                assert health.status_code == 200
            finally:
                release.set()
            assert (await job).status_code == 200
    asyncio.run(run())


def test_ocr_jobs_are_singleflight_and_reject_changed_content():
    import pytest
    from course_graph.ocr.jobs import JobConflict, OcrJobs

    jobs = OcrJobs()
    job, owner, future = jobs.reserve("material-1", "source.pdf", b"same source")
    assert owner and future is not None
    duplicate, duplicate_owner, duplicate_future = jobs.reserve("material-1", "source.pdf", b"same source")
    assert duplicate is job and not duplicate_owner and duplicate_future is future
    jobs.update(job, state="running", stage="converting")
    status = jobs.get("material-1")
    assert status["state"] == "running"
    assert status["progress"] == {"pages_done": None, "pages_total": None}
    with pytest.raises(JobConflict):
        jobs.reserve("material-1", "source.pdf", b"different source")


def test_oversized_ocr_result_is_not_counted_in_cache_budget():
    from course_graph.ocr.jobs import OcrJobs

    jobs = OcrJobs(max_jobs=2, max_result_bytes=40)
    a, _, future = jobs.reserve("a", "a.md", b"a")
    jobs.finish(a, future, {"text": "x" * 100})
    assert future.result() == (True, {"text": "x" * 100})
    b, _, fb = jobs.reserve("b", "b.md", b"b")
    c, _, fc = jobs.reserve("c", "c.md", b"c")  # evicts a
    jobs.finish(b, fb, {"text": "b" * 20})
    jobs.finish(c, fc, {"text": "c" * 20})
    retained = sum(j.result_bytes for j in jobs._jobs.values() if j.result is not None)
    assert jobs._result_bytes == retained >= 0
    assert retained <= 40


def test_model_bootstrap_runs_once_and_readiness_does_not_reinitialize(monkeypatch):
    from course_graph.model_runtime import ModelRuntime

    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    monkeypatch.setenv("COURSE_GRAPH_MODEL_BOOTSTRAP", "true")
    runtime = ModelRuntime()
    calls = []
    runtime._prepare_models = lambda: calls.append("prepared")
    runtime.start()
    assert runtime.wait()
    status = runtime.status()
    assert status["ready"] is True and status["state"] == status["stage"] == "ready"
    assert "updated_at" in status and "progress" in status
    assert calls == ["prepared"]


def test_model_bootstrap_failure_records_error_and_unblocks_wait(monkeypatch):
    from course_graph.model_runtime import ModelRuntime

    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "true")
    monkeypatch.setenv("COURSE_GRAPH_MODEL_BOOTSTRAP", "true")
    monkeypatch.setenv("COURSE_GRAPH_MODEL_BOOTSTRAP_ATTEMPTS", "1")
    runtime = ModelRuntime()
    def fail():
        raise RuntimeError("download failed")
    runtime._prepare_models = fail
    runtime.start()
    assert not runtime.wait()
    status = runtime.status()
    assert status["ready"] is False
    assert status["state"] == "failed"
    assert status["stage"] == "failed"
    assert status["detail"] == "OCR model preparation failed (RuntimeError)"


def test_remote_url_log_redaction_removes_signed_query():
    import logging
    from course_graph.model_runtime import RedactRemoteURLs

    record = logging.LogRecord("test", logging.WARNING, "", 1, "download failed: https://cdn.example/model?token=secret", (), None)
    RedactRemoteURLs().filter(record)
    assert "secret" not in record.getMessage()
    assert "<remote-url-redacted>" in record.getMessage()


def test_ocr_http_reconnect_joins_same_job_and_status_adopts_result(monkeypatch):
    import asyncio
    import threading
    import httpx
    import course_graph.api as api

    monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", "false")
    started, release = threading.Event(), threading.Event()
    calls = []

    def slow(filename, content):
        calls.append(content)
        started.set()
        assert release.wait(3)
        return {"markdown": content.decode(), "report": {"engine": "test"}}

    monkeypatch.setattr(api, "convert_source_document", slow)

    async def run():
        async with httpx.AsyncClient(transport=httpx.ASGITransport(app=api.app), base_url="http://test") as client:
            headers = {"X-Job-ID": "material-reconnect-test"}
            first = asyncio.create_task(client.post(
                "/ocr/convert", files={"file": ("source.md", b"same source")}, headers=headers,
            ))
            try:
                assert await asyncio.to_thread(started.wait, 2)
                status = await client.get("/ocr/jobs/material-reconnect-test")
                assert status.status_code == 200
                assert status.json()["state"] == "running"
                reconnect = asyncio.create_task(client.post(
                    "/ocr/convert", files={"file": ("source.md", b"same source")}, headers=headers,
                ))
                await asyncio.sleep(0.05)
            finally:
                release.set()
            first_response, reconnect_response = await asyncio.gather(first, reconnect)
            assert first_response.status_code == reconnect_response.status_code == 200
            assert first_response.json() == reconnect_response.json()
            adopted = await client.get("/ocr/jobs/material-reconnect-test")
            assert adopted.json()["state"] == "succeeded"
            assert adopted.json()["result"] == first_response.json()
            cached = await client.post(
                "/ocr/convert", files={"file": ("source.md", b"same source")}, headers=headers,
            )
            assert cached.status_code == 200
            conflict = await client.post(
                "/ocr/convert", files={"file": ("source.md", b"changed")}, headers=headers,
            )
            assert conflict.status_code == 409
            assert len(calls) == 1

    asyncio.run(run())


def test_picture_labels_recovered_without_inventing_diagram_relations():
    from types import SimpleNamespace as S
    from course_graph.ocr.pictures import picture_texts
    picture = S(prov=[S(page_no=8)], get_image=lambda doc: "crop")
    document = S(pictures=[picture])
    reader = lambda crop: S(txts=("Here you can add", "Facts", "SWI-Prolog", "noisy"), scores=(.99,.98,.99,.6))
    converter = S(initialized_pipelines={1:S(ocr_model=S(reader=reader))})
    recovered, metrics = picture_texts(document,converter,{8:"## SWI-Prolog\n<!-- image -->"})
    assert recovered == {8:[(1,["Here you can add","Facts"])]}
    assert metrics == {"regions":1,"lines":2,"failed_regions":0}


def test_picture_ocr_failure_is_explicit_and_keeps_document():
    from types import SimpleNamespace as S
    from course_graph.ocr.pictures import picture_texts
    document = S(pictures=[S(prov=[S(page_no=3)],get_image=lambda doc:None)])
    converter = S(initialized_pipelines={1:S(ocr_model=S(reader=lambda _:None))})
    recovered, metrics = picture_texts(document,converter,{3:"Source text"})
    assert not recovered and metrics["failed_regions"] == 1


def test_prolog_literals_are_fenced_without_changing_operators():
    from course_graph.ocr.converter import protect_prolog_literals
    source = "## Prolog\nFor example: tiger(X) :- cat(X), big(X).\n## ?- cat(X).\ncat(tom). [Tom is cat]\n```\npet(X) :- cat(X), not(wild(X)).\n```"
    result = protect_prolog_literals(source)
    assert "```prolog\ntiger(X) :- cat(X), big(X).\n```" in result
    assert "```prolog\n?- cat(X).\n```" in result
    assert "```prolog\ncat(tom).\n```" in result
    assert result.count("pet(X) :- cat(X), not(wild(X)).") == 1
    assert protect_prolog_literals(result) == result


def test_prolog_fencing_survives_light_mode_and_docling_failure(monkeypatch):
    import course_graph.ocr.converter as converter
    monkeypatch.setattr(converter, "_extract_with_pypdf", lambda *_: "# Prolog\npet(X) :- cat(X), not(wild(X)).")
    def broken_docling():
        raise RuntimeError("model unavailable")
    monkeypatch.setattr(converter, "get_converter", broken_docling)
    for with_docling in ("false", "true"):
        monkeypatch.setenv("COURSE_GRAPH_WITH_DOCLING", with_docling)
        result = converter.convert_document("source.pdf", b"fixture")
        assert "```prolog\npet(X) :- cat(X), not(wild(X)).\n```" in result["markdown"]
        assert result["report"]["engine"] == "pypdf"
        assert result["report"]["warnings"]


def test_native_program_text_survives_generative_enrichment():
    from types import SimpleNamespace as S
    from docling_core.types.doc import DocItemLabel
    from course_graph.ocr.enrichment import NativeSyntaxGuard
    class Stage:
        engine = object()
        def __call__(self, doc, elements):
            assert all(element.item.label != DocItemLabel.CODE for element in elements)
            for element in elements:
                element.item.text = "corrupted output"
                yield element.item
    code = S(text="pet(X) :- cat(X), not(wild(X)).", label=DocItemLabel.CODE)
    formula = S(text="animal(X) :- cat(X).", label=DocItemLabel.FORMULA)
    stage = NativeSyntaxGuard(Stage(), native_text_for=lambda item: item.text)
    result = list(stage(None, [S(item=code), S(item=formula)]))
    assert result[0].text == "pet(X) :- cat(X), not(wild(X))."
    assert result[1].text == "animal(X) :- cat(X)."
    assert result[1].label == DocItemLabel.TEXT
    assert len(stage.warnings) == 1


def test_code_enrichment_is_bypassed_only_for_exact_native_pdf_text():
    from types import SimpleNamespace as S
    from docling_core.types.doc import DocItemLabel
    from course_graph.ocr.enrichment import NativeSyntaxGuard

    source = "def search():\n    return True"
    scanned = S(text=source, label=DocItemLabel.CODE)
    mismatch = S(text=source, label=DocItemLabel.CODE)
    matching = S(text=source, label=DocItemLabel.CODE)
    missing_provenance = S(text=source, label=DocItemLabel.CODE)

    class Stage:
        engine = object()
        def __init__(self):
            self.seen = []
        def __call__(self, _, elements):
            self.seen.extend(elements)
            for element in elements:
                element.item.text = "enriched result"
                yield element.item

    stage_impl = Stage()
    native = {
        id(scanned): None,
        id(mismatch): "def search(): return False",
        id(matching): source,
        id(missing_provenance): None,
    }
    guard = NativeSyntaxGuard(stage_impl, native_text_for=lambda item: native[id(item)])
    result = list(guard(None, [S(item=scanned), S(item=mismatch), S(item=matching), S(item=missing_provenance)]))
    assert result[0].text == "enriched result"
    assert result[1].text == "enriched result"
    assert result[2].text == source
    assert result[3].text == "enriched result"
    assert [entry.item for entry in stage_impl.seen] == [scanned, mismatch, missing_provenance]


def test_formula_enrichment_not_bypassed_unless_confirmed_prolog():
    from types import SimpleNamespace as S
    from docling_core.types.doc import DocItemLabel
    from course_graph.ocr.enrichment import NativeSyntaxGuard

    formula = S(text="x^2 + y^2 = z^2", label=DocItemLabel.FORMULA)
    prolog_formula = S(text="cat(X) :- feline(X).", label=DocItemLabel.FORMULA)

    class Stage:
        engine = object()
        def __init__(self):
            self.seen = []
        def __call__(self, _, elements):
            self.seen.extend(elements)
            for element in elements:
                element.item.text = "enriched formula"
                yield element.item

    stage_impl = Stage()
    native = {
        id(formula): "x^2 + y^2 = z^2",
        id(prolog_formula): "cat(X) :- feline(X).",
    }
    guard = NativeSyntaxGuard(stage_impl, native_text_for=lambda item: native[id(item)])
    result = list(guard(None, [S(item=formula), S(item=prolog_formula)]))

    # Standard formula goes through enrichment despite matching native text
    assert result[0].text == "enriched formula"
    assert result[0].label == DocItemLabel.FORMULA
    assert [e.item for e in stage_impl.seen] == [formula]

    # Prolog rule classified as formula bypasses enrichment and is relabeled as TEXT
    assert result[1].text == "cat(X) :- feline(X)."
    assert result[1].label == DocItemLabel.TEXT
    assert len(guard.warnings) == 1


def test_enrichment_sets_model_cache_and_rejects_truncation():
    import pytest
    from types import SimpleNamespace as S
    from course_graph.ocr.enrichment import AuditedEngine
    class Engine:
        options = S(use_kv_cache=True)
        vlm_model = S(generation_config=S(use_cache=False))
        def initialize(self):
            pass
        def predict_batch(self, inputs):
            assert self.vlm_model.generation_config.use_cache
            return [S(text="truncated", metadata={"num_tokens":2048})]
    warnings = []
    with pytest.raises(ValueError, match="token limit"):
        AuditedEngine(Engine(), warnings).predict_batch([S(max_new_tokens=2048)])
    assert warnings and "частично" in warnings[0]


def test_bootstrap_can_initialize_the_guarded_enrichment_engine():
    from types import SimpleNamespace as S
    from docling_core.types.doc import DocItemLabel
    from course_graph.ocr.enrichment import NativeSyntaxGuard

    class Engine:
        options = S(use_kv_cache=True)
        def __init__(self):
            self.calls = 0
            self.vlm_model = S(generation_config=S(use_cache=False))
        def initialize(self):
            self.calls += 1
        def predict_batch(self, _):
            return []

    class Stage:
        def __init__(self):
            self.engine = Engine()
        def __call__(self, _, elements):
            return elements

    stage = NativeSyntaxGuard(Stage())
    stage.engine.initialize()
    assert stage.engine.engine.calls == 1
    assert stage.engine.engine.vlm_model.generation_config.use_cache is True


def test_formula_misclassification_recovers_native_pdf_rectangle():
    import io
    from types import SimpleNamespace as S
    from pypdf import PdfWriter
    from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject
    from docling_core.types.doc import DocItemLabel, BoundingBox, CoordOrigin, FormulaItem, TextItem, ProvenanceItem
    from course_graph.ocr.enrichment import NativePdfTextReader, recover_native_programs
    writer = PdfWriter()
    page = writer.add_blank_page(width=300, height=300)
    font = DictionaryObject({NameObject("/Type"):NameObject("/Font"),NameObject("/Subtype"):NameObject("/Type1"),NameObject("/BaseFont"):NameObject("/Helvetica")})
    page[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"):DictionaryObject({NameObject("/F1"):writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b"BT /F1 12 Tf 20 20 Td (animal\\(X\\) :- cat\\(X\\).) Tj ET")
    page[NameObject("/Contents")] = writer._add_object(stream)
    content = io.BytesIO()
    writer.write(content)
    provenance = [ProvenanceItem(page_no=1,charspan=(0,11),bbox=BoundingBox(l=10,t=50,r=290,b=0,coord_origin=CoordOrigin.BOTTOMLEFT))]
    wrong = FormulaItem(self_ref="#/texts/0",text="wrong colon",orig="wrong colon",prov=provenance)
    exact = FormulaItem(self_ref="#/texts/1",text="animal(X) :- cat(X).",orig="animal(X) :- cat(X).",prov=provenance)
    reader = NativePdfTextReader(content.getvalue())
    assert reader(exact) == "animal(X) :- cat(X)."
    reader.close()
    document = S(texts=[wrong, exact])
    warnings = recover_native_programs(document, content.getvalue())
    assert document.texts[0].text == "wrong colon"
    assert type(document.texts[0]) is FormulaItem
    assert document.texts[1].text == "animal(X) :- cat(X)."
    assert type(document.texts[1]) is TextItem
    assert document.texts[1].label == DocItemLabel.TEXT
    assert len(warnings) == 2
