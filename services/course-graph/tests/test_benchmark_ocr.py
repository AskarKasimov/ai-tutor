import importlib.util
import builtins
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


def test_benchmark_configures_environment_before_ocr_imports(tmp_path, monkeypatch):
    from course_graph.config import configure_library_environment
    script = Path(__file__).parents[1] / "scripts" / "benchmark_ocr.py"
    spec = importlib.util.spec_from_file_location("benchmark_ocr_env", script)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    config = Path(__file__).parents[1] / "config" / "dev.yaml"
    cache = tmp_path / "cache"
    cache.mkdir()
    module.prepare_benchmark_config(config, cache)
    monkeypatch.setenv("OMP_NUM_THREADS", "11")
    monkeypatch.setenv("MKL_NUM_THREADS", "12")
    events = []
    def configure(settings):
        configure_library_environment(settings)
        events.append("configure")
    original_import = builtins.__import__
    def tracked_import(name, *args, **kwargs):
        if name == "course_graph.ocr.converter":
            events.append("ocr_import")
        return original_import(name, *args, **kwargs)
    monkeypatch.setattr("course_graph.config.configure_library_environment", configure)
    monkeypatch.setattr(builtins, "__import__", tracked_import)
    settings, *_ = module.load_benchmark_ocr()
    assert events == ["configure", "ocr_import"]
    assert settings.ocr.threads == 4
    assert os.environ["OMP_NUM_THREADS"] == "11"
    assert os.environ["MKL_NUM_THREADS"] == "12"
