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


def test_slow_ocr_keeps_health_responsive(monkeypatch):
    import asyncio
    import threading
    import httpx
    import course_graph.api as api
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
            for element in elements:
                element.item.text = "corrupted output"
                yield element.item
    code = S(text="pet(X) :- cat(X), not(wild(X)).", label=DocItemLabel.CODE)
    formula = S(text="animal(X) :- cat(X).", label=DocItemLabel.FORMULA)
    stage = NativeSyntaxGuard(Stage())
    result = list(stage(None, [S(item=code), S(item=formula)]))
    assert result[0].text == "pet(X) :- cat(X), not(wild(X))."
    assert result[1].text == "animal(X) :- cat(X)."
    assert result[1].label == DocItemLabel.TEXT
    assert len(stage.warnings) == 2


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


def test_formula_misclassification_recovers_native_pdf_rectangle():
    import io
    from types import SimpleNamespace as S
    from pypdf import PdfWriter
    from pypdf.generic import DictionaryObject, NameObject, DecodedStreamObject
    from docling_core.types.doc import DocItemLabel, BoundingBox, CoordOrigin, FormulaItem, TextItem, ProvenanceItem
    from course_graph.ocr.enrichment import recover_native_programs
    writer = PdfWriter()
    page = writer.add_blank_page(width=300, height=300)
    font = DictionaryObject({NameObject("/Type"):NameObject("/Font"),NameObject("/Subtype"):NameObject("/Type1"),NameObject("/BaseFont"):NameObject("/Helvetica")})
    page[NameObject("/Resources")] = DictionaryObject({NameObject("/Font"):DictionaryObject({NameObject("/F1"):writer._add_object(font)})})
    stream = DecodedStreamObject()
    stream.set_data(b"BT /F1 12 Tf 20 20 Td (animal\\(X\\) :- cat\\(X\\).) Tj ET")
    page[NameObject("/Contents")] = writer._add_object(stream)
    content = io.BytesIO()
    writer.write(content)
    item = FormulaItem(self_ref="#/texts/0",text="wrong colon",orig="wrong colon",prov=[ProvenanceItem(page_no=1,charspan=(0,11),bbox=BoundingBox(l=10,t=50,r=290,b=0,coord_origin=CoordOrigin.BOTTOMLEFT))])
    document = S(texts=[item])
    warnings = recover_native_programs(document, content.getvalue())
    assert document.texts[0].text == "animal(X) :- cat(X)."
    assert type(document.texts[0]) is TextItem
    assert document.texts[0].label == DocItemLabel.TEXT
    assert len(warnings) == 1
