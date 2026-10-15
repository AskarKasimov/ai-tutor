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
