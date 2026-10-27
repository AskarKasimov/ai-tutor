import pytest

from course_graph.config import get_settings


@pytest.fixture(autouse=True)
def isolate_cached_process_settings():
    """Each test models a process snapshot and must not inherit another test's YAML."""
    get_settings.cache_clear()
    yield
    get_settings.cache_clear()
