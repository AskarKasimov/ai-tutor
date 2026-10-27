import importlib.util
import os
from pathlib import Path

from course_graph.config import get_settings


def test_benchmark_config_redirects_only_document_cache(tmp_path, monkeypatch):
    script = Path(__file__).parents[1] / "scripts" / "benchmark_ocr.py"
    spec = importlib.util.spec_from_file_location("benchmark_ocr", script)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    source = tmp_path / "dev.yaml"
    source.write_text((Path(__file__).parents[1] / "config" / "dev.yaml").read_text())
    cache = tmp_path / "isolated-cache"
    cache.mkdir()
    monkeypatch.setenv("COURSE_GRAPH_OCR_CACHE_DIR", "/must/not/be/used")
    monkeypatch.setenv("COURSE_GRAPH_CONFIG", str(source))
    get_settings.cache_clear()

    snapshot = module.prepare_benchmark_config(source, cache)
    settings = get_settings()
    assert snapshot.exists()
    assert Path(settings.cache.documents_dir) == cache
    assert settings.ocr.threads == 4
    assert settings.ocr.picture_text is True
    assert settings.cache.rapidocr_dir == "/var/lib/course-graph/rapidocr"
    assert os.environ["COURSE_GRAPH_CONFIG"] == str(snapshot)
