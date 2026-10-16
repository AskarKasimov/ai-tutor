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
