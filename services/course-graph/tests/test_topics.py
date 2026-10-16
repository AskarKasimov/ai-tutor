import asyncio
import json

import numpy as np
import pytest

from course_graph.topics.extract import ExtractionSettings, extract_topics, topic_blocks
from course_graph.topics.graph import build_links, valid_evidence
from course_graph.topics.models import Evidence, Source, Topic
from course_graph.topics.retrieval import CandidateIndex


def run(task):
    return asyncio.run(task)


def make_topic(id, title, provides, requires=(), text=None):
    text = text or title
    return Topic(id=id, title=title, provides=provides, requires=list(requires),
                 question="Объясните тему", text=text,
                 sources=[Source(material_id=id, block_id=0, start=0, end=len(text), text=text)])


class Encoder:
    # Controlled semantic vectors; integration checks run the real multilingual encoder.
    def encode(self, texts, query=False):
        return np.array([[float("производн" in t.lower()), float("матриц" in t.lower()), .01] for t in texts])


async def extraction_stub(system, payload):
    blocks = json.loads(payload)["blocks"]
    remove = [{"id": b["id"], "reason": "thanks"} for b in blocks if b["text"].strip() == "Спасибо за внимание!"]
    retained = [b["id"] for b in blocks if b["id"] not in {r["id"] for r in remove}]
    return json.dumps({"remove": remove, "topics": [{"title": "Производная", "requires": [],
        "provides": ["Вычислять производную"], "question": "Как вычислить производную?",
        "block_ids": retained}] if retained else []})


def test_joint_extraction_preserves_exact_text_code_math_and_sources():
    source = "# Производная\r\n\r\nПроизводная — скорость изменения функции.\r\n\r\n$$f'(x)=2x$$\r\n\r\n```python\r\nx=2\r\n```\r\n\r\nСпасибо за внимание!"
    result = run(extract_topics(source, extraction_stub, "lecture"))
    report = result["report"]
    assert result["markdown"] == source[:source.index("\r\n\r\nСпасибо")+2]
    assert report["quality"]["unassigned_blocks"] == 0
    for topic in report["topics"]:
        assert "f'(x)=2x" in topic["text"] and "x=2" in topic["text"]
        for s in topic["sources"]:
            assert s["material_id"] == "lecture"
            assert s["text"] == source[s["start"]:s["end"]]


def test_adaptive_split_recovers_incomplete_coverage():
    async def incomplete(system, payload):
        data = json.loads(payload)
        if len(data["blocks"]) > 1:
            return '{"remove":[],"topics":[]}'
        return await extraction_stub(system, payload)
    result = run(extract_topics("Производная\n\nОбъяснение", incomplete))
    assert result["report"]["adaptive_splits"] == 1
    assert len(result["report"]["topics"]) == 1
    assert len(result["report"]["topics"][0]["sources"]) == 2


@pytest.mark.parametrize("response", [
    '{"remove":[],"topics":[]}',
    '{"remove":[{"id":999,"reason":"thanks"}],"topics":[]}',
    '{"remove":[{"id":true,"reason":"thanks"}],"topics":[]}',
    '{"remove":[{"id":0,"reason":"thanks"}],"topics":[]}',
])
def test_incomplete_or_protected_deletion_is_never_published(response):
    async def complete(*args):
        return response
    with pytest.raises(ValueError):
        run(extract_topics("$$x^2$$", complete))


def test_long_plain_blocks_preserve_offsets_and_protected_blocks_fail_explicitly():
    source = "Длинный текст. " * 1500
    blocks = topic_blocks(source, 1500)
    assert "".join(b.text for b in blocks) == source
    assert all(len(b.text) <= 1500 and source[b.start:b.end] == b.text for b in blocks)
    with pytest.raises(ValueError, match="Неделимый"):
        run(extract_topics("```\n" + "x"*7000 + "\n```", extraction_stub))


def test_short_slide_bullets_are_grouped_without_losing_code_or_offsets():
    source = "## Слайд 1\n\nТема\n\n- Первый пункт\n- Второй пункт\n\n## Слайд 2\n\n$$x^2$$\n"
    blocks = topic_blocks(source, 6000)
    assert len(blocks) == 2
    assert not blocks[0].protected
    assert blocks[1].protected
    cursor = 0
    for block in blocks:
        assert not source[cursor:block.start].strip()
        assert source[block.start:block.end] == block.text
        cursor = block.end
    assert not source[cursor:].strip()


def test_slide_grouping_keeps_administrative_paragraph_deletable():
    source = "## Слайд 1\n\nПроизводная — скорость изменения функции.\n\nСпасибо за внимание!"
    result = run(extract_topics(source, extraction_stub))
    assert "скорость изменения" in result["markdown"]
    assert "Спасибо" not in result["markdown"]
    assert result["report"]["removed"][0]["reason"] == "thanks"


def test_global_retrieval_finds_distant_topic_and_excludes_self():
    topics = [make_topic("T2", "Производная", ["Вычисление производной"])]
    topics += [make_topic(f"T{i}", "Другая тема " + str(i), ["Работа с матрицами"]) for i in range(3, 20)]
    target = make_topic("T20", "Оптимизация", ["Производная функции потерь"], ["Вычисление производной"])
    topics.append(target)
    result = CandidateIndex(topics, Encoder()).search_all(topics, 2)
    ids = [c["topic_id"] for c in result[("T20", "Вычисление производной")]]
    assert ids[0] == "T2" and "T20" not in ids


async def grounded_judge(system, payload):
    links, decisions = [], []
    for task in json.loads(payload)["tasks"]:
        candidate = task["candidates"][0]
        links.append({"target_id": task["target_id"], "prerequisite_id": candidate["id"],
                      "requirement": task["requirement"], "reason": "Использует это знание",
                      "target_evidence": {**{k: task["evidence"][0][k] for k in ("material_id", "block_id")}, "quote": task["evidence"][0]["text"]},
                      "prerequisite_evidence": {**{k: candidate["evidence"][0][k] for k in ("material_id", "block_id")}, "quote": candidate["evidence"][0]["text"]}})
        decisions.append({"target_id": task["target_id"], "requirement": task["requirement"],
                          "prerequisite_ids": [candidate["id"]], "reason": "Подтверждено"})
    return json.dumps({"links": links, "requirements": decisions}, ensure_ascii=False)


def test_links_have_direction_quotes_and_unresolved_requirements():
    topics = [make_topic("T2", "Производная", ["Вычисление производной"]),
              make_topic("T20", "Спуск", ["Обновление параметров"], ["Вычисление производной"])]
    graph = run(build_links(topics, grounded_judge, Encoder()))
    assert [(e["source"], e["target"]) for e in graph["prerequisites"]] == [("T2", "T20")]
    assert graph["quality"]["requests"] == 1


def test_fabricated_quote_fails_instead_of_publishing_edge():
    async def bad(system, payload):
        result = json.loads(await grounded_judge(system, payload))
        result["links"][0]["target_evidence"]["quote"] = "Невстречающаяся цитата"
        return json.dumps(result)
    topics = [make_topic("a", "Производная", ["Производная"]),
              make_topic("b", "Спуск", ["Обновление"], ["Производная"])]
    with pytest.raises(ValueError, match="цитата"):
        run(build_links(topics, bad, Encoder()))


def test_ocr_line_wrapping_is_canonicalized_but_words_cannot_change():
    excerpts = [{"material_id": "a", "block_id": 0, "text": "Some important\nconcept is used."}]
    evidence = Evidence(material_id="a", block_id=0, quote="important concept")
    assert valid_evidence(evidence, excerpts)
    assert evidence.quote == "important\nconcept"
    assert not valid_evidence(Evidence(material_id="a", block_id=0, quote="important different concept"), excerpts)


def test_cycle_is_reported_and_never_enters_published_graph():
    topics = [make_topic("a", "A", ["Знание A"], ["Знание B"]),
              make_topic("b", "B", ["Знание B"], ["Знание A"])]
    graph = run(build_links(topics, grounded_judge, Encoder()))
    assert len(graph["prerequisites"]) == 1
    assert len(graph["rejected_links"]) == 1
    assert graph["quality"]["unresolved_requirements"] == 1


def test_empty_candidates_are_explicit_not_invented_topics():
    async def never(*args):
        raise AssertionError("No model call without candidates")
    graph = run(build_links([make_topic("a", "A", ["Знание A"], ["Внешнее знание"])], never, Encoder()))
    assert not graph["prerequisites"]
    assert graph["quality"]["unresolved_requirements"] == 1


def test_duplicate_titles_merge_sources_but_similar_titles_do_not():
    topics = [make_topic("a", "Градиент", ["Частные производные"]),
              make_topic("b", "Градиент", ["Направление роста"]),
              make_topic("c", "Градиентный спуск", ["Обновление параметров"])]
    graph = run(build_links(topics, grounded_judge, Encoder()))
    assert len(graph["topics"]) == 2
    assert graph["aliases"] == {"b": "a"}
    assert len(graph["topics"][0]["sources"]) == 2


def test_build_endpoint_reuses_cached_cards_and_validates_source_text(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class Client:
        async def complete(self, system, payload):
            assert "tasks" in json.loads(payload), "Cached cards must not be extracted again"
            return await grounded_judge(system, payload)
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", Client)
    monkeypatch.setattr(api, "get_encoder", Encoder)
    topics = [make_topic("a", "Производная", ["Вычисление производной"]),
              make_topic("b", "Оптимизация", ["Обновление параметров"], ["Вычисление производной"])]
    materials = [{"id": t.id, "markdown": t.text, "topics": [t.model_dump()]} for t in topics]
    response = TestClient(api.app).post("/graph/build", json={"materials": materials})
    assert response.status_code == 200
    assert response.json()["prerequisites"][0]["source"] == "a"
    materials[0]["markdown"] = "Другой исходник"
    response = TestClient(api.app).post("/graph/build", json={"materials": materials})
    assert response.status_code == 503
    assert "topics" not in response.json()


def test_joint_endpoint_returns_cards_and_cleaning_audit(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class Client:
        async def complete(self, system, payload):
            return await extraction_stub(system, payload)
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", Client)
    response = TestClient(api.app).post("/topics/extract", json={"markdown": "Производная показывает скорость изменения функции.\n\nСпасибо за внимание!", "material_id": "m"})
    assert response.status_code == 200
    report = response.json()["report"]
    assert report["topics_version"] == "topics-v1"
    assert report["topics"][0]["sources"][0]["material_id"] == "m"
