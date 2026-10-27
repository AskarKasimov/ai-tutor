from pathlib import Path

import pytest

from course_graph.config import load_config


def valid_yaml():
    return '''server: {port: 8000}
llm_gateway: {address: "llm-gateway:8011", timeout: 300s}
embedding: {model: "intfloat/multilingual-e5-small", threads: 4}
topics: {max_chars: 6000}
models: {bootstrap: {enabled: true, attempts: 3}}
ocr: {threads: 4, document_timeout: 0s, enrich_code: false, enrich_formula: true, picture_text: false, full_page: false}
cache: {documents_dir: "/tmp/ocr", rapidocr_dir: "/tmp/rapid"}
'''


def write_config(tmp_path: Path, body: str) -> Path:
    path = tmp_path / "config.yaml"
    path.write_text(body)
    return path


def test_load_config_preserves_false_zero_and_parses_duration(tmp_path, monkeypatch):
    monkeypatch.setenv("COURSE_GRAPH_OCR_THREADS", "99")
    settings = load_config(write_config(tmp_path, valid_yaml()))
    assert settings.ocr.enrich_code is False
    assert settings.ocr.picture_text is False
    assert settings.ocr.document_timeout_seconds == 0
    assert settings.llm_gateway.timeout_seconds == 300
    assert settings.ocr.threads == 4


def test_load_config_rejects_unknown_duplicate_multiple_documents_and_types(tmp_path):
    cases = [
        valid_yaml() + "unknown: true\n",
        valid_yaml().replace("server:", "server:\nserver:", 1),
        valid_yaml() + "---\n" + valid_yaml(),
        valid_yaml().replace("threads: 4", "threads: many", 1),
        valid_yaml().replace("timeout: 300s", "timeout: 0", 1),
    ]
    for body in cases:
        with pytest.raises(ValueError):
            load_config(write_config(tmp_path, body))


def test_load_config_checks_ranges_and_new_settings(tmp_path):
    config = valid_yaml().replace("attempts: 3", "attempts: 5").replace("picture_text: false", "picture_text: true")
    config = config.replace("rapidocr_dir: \"/tmp/rapid\"", "rapidocr_dir: \"/var/cache/rapid\"")
    settings = load_config(write_config(tmp_path, config))
    assert settings.models.bootstrap.attempts == 5
    assert settings.ocr.picture_text is True
    assert settings.cache.rapidocr_dir == "/var/cache/rapid"
    with pytest.raises(ValueError):
        load_config(write_config(tmp_path, valid_yaml().replace("attempts: 3", "attempts: 0")))
