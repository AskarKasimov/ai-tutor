"""Helpers for tests that need process settings from temporary YAML files."""
from copy import deepcopy
from pathlib import Path

import yaml

from course_graph.config import load_config


def configure_settings(monkeypatch, tmp_path, **overrides):
    data = yaml.safe_load(Path(__file__).parents[1].joinpath("config/dev.yaml").read_text())
    for key, value in overrides.items():
        parts = key.split("__")
        target = data
        for part in parts[:-1]:
            target = target[part]
        target[parts[-1]] = value
    path = tmp_path.parent / f"{tmp_path.name}-settings.yaml"
    path.write_text(yaml.safe_dump(data, sort_keys=False))
    monkeypatch.setenv("COURSE_GRAPH_CONFIG", str(path))
    from course_graph import config
    config.get_settings.cache_clear()
    settings = load_config(path)
    # The app and runtime intentionally take one process snapshot. Tests replace
    # those snapshots explicitly after selecting a new process config.
    import course_graph.api as api
    import course_graph.model_runtime as model_runtime
    monkeypatch.setattr(api, "SETTINGS", settings)
    monkeypatch.setattr(model_runtime, "SETTINGS", settings)
    return settings
