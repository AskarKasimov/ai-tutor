import asyncio
import json

import pytest

from course_graph.cleaning.cleaner import clean_markdown, split_blocks


def test_exact_spans_and_mixed_slide():
    source = "# Поиск\r\n\r\nУчебное объяснение алгоритма.\r\n\r\n- Дедлайн: пятница\r\n- Найдите кратчайший путь\r\n\r\n$$\r\nf(n) = g(n) + h(n)\r\n$$\r\n\r\n```python\r\nprint('Thanks')\r\n```\r\n"

    async def complete(system, payload):
        data = json.loads(payload)
        assert "f(n)" not in str(data["blocks"])
        return json.dumps({"remove": [{"id": b["id"], "reason": "organization"} for b in data["blocks"] if "Дедлайн" in b["text"]]})

    result = asyncio.run(clean_markdown(source, complete))
    assert result["markdown"] == source.replace("- Дедлайн: пятница\r\n", "")
    removed = result["report"]["removed"][0]
    assert source[removed["start"]:removed["end"]] == removed["text"]


@pytest.mark.parametrize("response", ['{"remove":[{"id":999,"reason":"thanks"}]}', '{"remove":[{"id":true,"reason":"thanks"}]}', '{"remove":[{"id":0,"reason":"theory"}]}', '{"remove":[{"id":0,"reason":"thanks"},{"id":0,"reason":"thanks"}]}', '{}', 'broken'])
def test_invalid_decision_fails_instead_of_silent_success(response):
    async def complete(*args):
        return response
    with pytest.raises((ValueError, TypeError)):
        asyncio.run(clean_markdown("Учебное содержание", complete))


def test_dangerous_mass_deletion_keeps_original():
    async def complete(system, payload):
        return json.dumps({"remove": [{"id": b["id"], "reason": "thanks"} for b in json.loads(payload)["blocks"]]})
    result = asyncio.run(clean_markdown("Весь текст лекции", complete))
    assert result["markdown"] == "Весь текст лекции"
    assert result["report"]["warnings"]
    assert not result["report"]["removed"]


def test_parallelism_is_bounded_and_order_preserved():
    async def run():
        active = peak = 0
        async def complete(*args):
            nonlocal active, peak
            active += 1
            peak = max(peak, active)
            await asyncio.sleep(0.005)
            active -= 1
            return '{"remove":[]}'
        source = "\n\n".join(["Объяснение " * 300] * 20)
        result = await clean_markdown(source, complete, concurrency=3)
        assert peak == 3
        assert result["markdown"] == source
    asyncio.run(run())


def test_protected_and_oversized_blocks_do_not_call_model():
    async def complete(*args):
        raise AssertionError("Must not classify incomplete code/math")
    source = "$$\nx^2\n\n+y^2\n$$\n\n```\ncode\n\nmore\n```\n\n" + "a" * 13000
    result = asyncio.run(clean_markdown(source, complete))
    assert result["markdown"] == source
    assert result["report"]["requests"] == 0


def test_split_blocks_keeps_original_offsets():
    text = "\n# Тема\n\nАбзац\nпродолжение\n\n- пункт\n- другой\n"
    for block in split_blocks(text):
        assert text[block.start:block.end] == block.text


def test_clean_endpoint_uses_backend_model_client(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class StubClient:
        async def complete(self, system, payload):
            return '{"remove":[]}'
        def completer(self, complexity, reasoning=False):
            return self.complete
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", StubClient)
    response = TestClient(api.app).post("/clean", json={"markdown": "# Теория\n\n$x^2$"})
    assert response.status_code == 200
    assert response.json()["markdown"] == "# Теория\n\n$x^2$"
    assert response.json()["report"]["version"] == "material-cleaning-v1"


def test_clean_endpoint_does_not_publish_partial_result(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class BrokenClient:
        async def complete(self, *args):
            raise RuntimeError("model unavailable")
        def completer(self, complexity, reasoning=False):
            return self.complete
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", BrokenClient)
    response = TestClient(api.app).post("/clean", json={"markdown": "Теория"})
    assert response.status_code == 503
    assert "markdown" not in response.json()
