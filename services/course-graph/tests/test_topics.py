import asyncio
import json
import time

import numpy as np
import pytest

from course_graph.topics.extract import (
    PROMPT,
    ExtractionSettings,
    absorb_thin_topics,
    content_chars,
    administrative_question,
    clean_topic_title,
    extract_topics,
    procedural_title,
    topic_blocks,
)
from course_graph.topics.graph import GRAPH_CONCURRENCY, build_links, valid_evidence
from course_graph.topics.extract import join_topic
from course_graph.topics.structure import (
    ATTACH_PROMPT,
    QUESTION_PROMPT,
    SYNONYM_PROMPT,
    depth,
    merge_isolated,
    merge_synonyms,
    refresh_questions,
)
from course_graph.topics.models import Evidence, Source, Topic
from course_graph.topics.retrieval import CandidateIndex


def run(task):
    return asyncio.run(task)


def make_topic(id, title, provides, requires=(), text=None):
    text = text or title
    return Topic(id=id, title=title, provides=provides, requires=list(requires),
                 question="Объясните тему", text=text,
                 sources=[Source(material_id=id, block_id=0, start=0, end=len(text), text=text)])


def write_questions(payload):
    return json.dumps({"questions": [{"id": t["id"], "question": "Почему верно: " + t["title"] + "?"}
                                     for t in json.loads(payload)["topics"]]}, ensure_ascii=False)


def links_only(judge):
    """The structure passes get no decisions, merged cards get a question;
    judge answers the link prompt."""
    async def complete(system, payload):
        if system == SYNONYM_PROMPT:
            return json.dumps({"pairs": [{"a": p["a"], "b": p["b"], "same": False}
                                         for p in json.loads(payload)["pairs"]]})
        if system == ATTACH_PROMPT:
            return json.dumps({"topics": [{"id": t["id"], "prerequisites": [], "dependents": []}
                                         for t in json.loads(payload)["topics"]]})
        if system == QUESTION_PROMPT:
            return write_questions(payload)
        return await judge(system, payload)
    return complete


def quote(evidence):
    return {"material_id": evidence["material_id"], "block_id": evidence["block_id"], "quote": evidence["text"]}


def attach_answer(payload, choose):
    """choose maps a topic id to (prerequisite ids, dependent ids); every link
    quotes the first shown evidence of each side."""
    out = []
    for item in json.loads(payload)["topics"]:
        candidates = {c["id"]: c for c in item["candidates"]}

        def link(other):
            return {"id": other, "reason": "Используется это знание", "topic_evidence": quote(item["evidence"][0]),
                    "candidate_evidence": quote(candidates[other]["evidence"][0])}
        before, after = choose.get(item["id"], ([], []))
        out.append({"id": item["id"], "prerequisites": [link(c) for c in before if c in candidates],
                    "dependents": [link(c) for c in after if c in candidates]})
    return json.dumps({"topics": out}, ensure_ascii=False)


def with_attach(choose):
    async def complete(system, payload):
        if system == ATTACH_PROMPT:
            return attach_answer(payload, choose)
        return await links_only(grounded_links)(system, payload)
    return complete


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


@pytest.mark.parametrize("protected_text", [
    "```python\nx = 2\n```",
    "$$f'(x)=2x$$",
    "| Аргумент | Значение |\n| --- | --- |\n| x | 2 |",
    "![График функции](function.png)",
])
def test_ignored_protected_removal_cannot_reappear_in_markdown_audit(protected_text):
    source = "Производная показывает скорость изменения функции.\n\n" + protected_text + "\n\nСпасибо за внимание!"

    async def misguided_removal(system, payload):
        blocks = json.loads(payload)["blocks"]
        return json.dumps({
            "remove": [{"id": b["id"], "reason": "thanks"} for b in blocks
                       if b["protected"] or b["text"].strip() == "Спасибо за внимание!"],
            "topics": [{"title": "Производная", "requires": [],
                        "provides": ["Объяснять смысл производной"],
                        "question": "Что показывает производная?",
                        "block_ids": [b["id"] for b in blocks if not b["protected"]
                                      and b["text"].strip() != "Спасибо за внимание!"]}],
        }, ensure_ascii=False)

    result = run(extract_topics(source, misguided_removal, "lecture"))
    report = result["report"]
    assert protected_text in result["markdown"]
    assert "Спасибо за внимание!" not in result["markdown"]
    assert [r["text"].strip() for r in report["removed"]] == ["Спасибо за внимание!"]
    topic = report["topics"][0]
    assert protected_text in topic["text"]
    removed_ids = {r["id"] for r in report["removed"]}
    assert not removed_ids.intersection(s["block_id"] for s in topic["sources"])
    assert report["quality"]["assigned_blocks"] == report["quality"]["retained_blocks"]


@pytest.mark.parametrize("context", [
    "## Слайд 1\n\n# Введение в ИИ\n\nНеделя 3",
    "## Slide 1\n\n# Introduction to AI\n\nWeek 3",
    "## Слайд 1\n\n# План\n\n- Агенты\n- Окружающая среда",
    "## Слайд 1\n\n# Агент и среда\n\n<!-- image -->\nДатчики\nАгент\nСреда",
])
def test_context_is_retained_without_becoming_an_independent_claim(context):
    teaching = "## Слайд 2\n\nАгент воспринимает среду через датчики и действует с помощью исполнительных устройств."
    source = context + "\n\n" + teaching
    seen_blocks = []

    async def explain_agent(system, payload):
        blocks = json.loads(payload)["blocks"]
        seen_blocks.extend(b["text"] for b in blocks)
        assert all("воспринимает" in b["text"] for b in blocks)
        return json.dumps({"remove": [], "topics": [{
            "title": "Взаимодействие агента со средой", "requires": [],
            "provides": ["Объяснять назначение датчиков и исполнительных устройств"],
            "question": "Как агент воспринимает среду и действует на неё?",
            "block_ids": [b["id"] for b in blocks],
        }]}, ensure_ascii=False)

    result = run(extract_topics(source, explain_agent, "lecture", week=3))
    assert seen_blocks
    assert result["markdown"] == source
    report = result["report"]
    assert len(report["topics"]) == 1
    assert len(report["context_blocks"]) == 1
    topic = report["topics"][0]
    assert topic["sources"][0]["page"] == 1
    assert topic["sources"][1]["page"] == 2
    assert all(s["week"] == 3 and source[s["start"]:s["end"]] == s["text"] for s in topic["sources"])
    assert context in topic["text"]
    assert report["quality"]["assigned_blocks"] == report["quality"]["retained_blocks"]


def test_only_context_cannot_publish_topics_with_invented_content():
    async def never(*args):
        raise AssertionError("No model calls for cover-only material")

    with pytest.raises(ValueError, match="нет объяснений"):
        run(extract_topics("## Слайд 1\n\n# Введение в ИИ\n\nНеделя 3", never))


@pytest.mark.parametrize("administrative", [
    "Сколько человек должно быть в каждой сформированной команде?",
    "Каков размер команды для выполнения задания?",
    "Сколько времени даётся на подготовку задания?",
    "Когда нужно сдавать выполненную работу?",
])
def test_administrative_question_is_repaired_without_deleting_mixed_exercise(administrative):
    source = "Сформируйте команды по 4–5 человек. За 10 минут опишите датчики и исполнительные устройства агента для выбранного сценария."
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        return json.dumps({"remove": [], "topics": [{
            "title": "Датчики и исполнительные устройства агента", "requires": [],
            "provides": ["Подбирать датчики и исполнительные устройства для сценария"],
            "question": administrative if len(calls) == 1 else "Какие датчики и исполнительные устройства нужны агенту в выбранном сценарии?",
            "block_ids": [b["id"] for b in data["blocks"]],
        }]}, ensure_ascii=False)

    result = run(extract_topics(source, complete))
    assert len(calls) == 2
    assert "организац" in calls[1]["correction"]
    assert result["markdown"] == source
    assert result["report"]["removed"] == []
    assert result["report"]["topics"][0]["question"].startswith("Какие датчики")


@pytest.mark.parametrize("question", [
    "Сколько команд выполняет процессор за один такт?",
    "Сколько времени нужно для работы алгоритма?",
    "Сколько времени занимает подготовка обучающих данных?",
])
def test_administrative_filter_allows_technical_counts_and_runtime_questions(question):
    assert not administrative_question(question)


def test_procedural_title_cleaning_and_detection():
    assert clean_topic_title("## Упражнение 1: Семейное дерево") == "Семейное дерево"
    assert clean_topic_title("Задание 2 — Поиск в глубину") == "Поиск в глубину"
    assert clean_topic_title("Практика: Основы Prolog") == "Основы Prolog"
    assert clean_topic_title("Лабораторная работа 1: Синтаксис") == "Синтаксис"
    assert clean_topic_title("Домашнее задание: Рекурсивные правила") == "Рекурсивные правила"

    assert procedural_title("Домашнее задание")
    assert procedural_title("Упражнение 4: Домашнее задание (запрос без новых правил)")
    assert procedural_title("Ссылки")
    assert procedural_title("Литература")
    assert procedural_title("Вопросы для самопроверки")
    assert not procedural_title("Семейное дерево")
    assert not procedural_title("Производная функции")
    assert not procedural_title("Составление запросов к базе фактов в Prolog")


def test_procedural_title_is_repaired_to_substantive_concept():
    source = "Составьте запрос к базе фактов, определяющий всех людей с одинаковой фамилией без добавления новых правил."
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 1:
            title = "Упражнение 4: Домашнее задание"
        else:
            title = "Составление запросов к базе фактов в Prolog"
        return json.dumps({"remove": [], "topics": [{
            "title": title, "requires": ["факты в Prolog"],
            "provides": ["составление запросов к фактам без правил"],
            "question": "Какой запрос вернет людей с одинаковой фамилией?",
            "block_ids": [b["id"] for b in data["blocks"]],
        }]}, ensure_ascii=False)

    result = run(extract_topics(source, complete))
    assert len(calls) == 2
    assert "процедурное" in calls[1]["correction"]
    assert result["report"]["topics"][0]["title"] == "Составление запросов к базе фактов в Prolog"


def test_procedural_prefix_is_cleaned_without_retry():
    source = "Опишите факты parent(X, Y) для построения генеалогического древа."
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        return json.dumps({"remove": [], "topics": [{
            "title": "Упражнение 1: Семейное дерево", "requires": [],
            "provides": ["моделирование родственных отношений"],
            "question": "Как объявить предикат parent?",
            "block_ids": [b["id"] for b in data["blocks"]],
        }]}, ensure_ascii=False)

    result = run(extract_topics(source, complete))
    assert len(calls) == 1
    assert result["report"]["topics"][0]["title"] == "Семейное дерево"


def test_build_links_sanitizes_procedural_titles_and_filters_links_card():
    t_normal = make_topic("T1", "Синтаксис Prolog", ["базовый синтаксис"])
    t_hw = make_topic("T2", "Упражнение 4: Домашнее задание", ["формирование запросов к фактам"], ["базовый синтаксис"])
    t_links = make_topic("T3", "Ссылки", ["перечень видеоматериалов"])

    async def stub_judge(system, payload):
        tasks = json.loads(payload)["tasks"]
        decisions = [{"target_id": t["target_id"], "requirement": t["requirement"],
                      "prerequisite_ids": [], "reason": "Нет связей"} for t in tasks]
        return json.dumps({"links": [], "requirements": decisions}, ensure_ascii=False)

    graph = run(build_links([t_normal, t_hw, t_links], links_only(stub_judge), Encoder()))
    titles = [t["title"] for t in graph["topics"]]
    assert "Ссылки" not in titles
    assert "Упражнение 4: Домашнее задание" not in titles
    assert "Формирование запросов к фактам" in titles



def test_global_retrieval_finds_distant_topic_and_excludes_self():
    topics = [make_topic("T2", "Производная", ["Вычисление производной"])]
    topics += [make_topic(f"T{i}", "Другая тема " + str(i), ["Работа с матрицами"]) for i in range(3, 20)]
    target = make_topic("T20", "Оптимизация", ["Производная функции потерь"], ["Вычисление производной"])
    topics.append(target)
    result = CandidateIndex(topics, Encoder()).search_all(topics, 2)
    ids = [c["topic_id"] for c in result[("T20", "Вычисление производной")]]
    assert ids[0] == "T2" and "T20" not in ids


async def grounded_links(system, payload):
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


grounded_judge = links_only(grounded_links)


def test_links_have_direction_quotes_and_unresolved_requirements():
    topics = [make_topic("T2", "Производная", ["Вычисление производной"]),
              make_topic("T20", "Спуск", ["Обновление параметров"], ["Вычисление производной"])]
    graph = run(build_links(topics, grounded_judge, Encoder()))
    assert [(e["source"], e["target"]) for e in graph["prerequisites"]] == [("T2", "T20")]
    assert graph["quality"]["requests"] == 1


def test_fabricated_quote_fails_instead_of_publishing_edge():
    async def bad(system, payload):
        result = json.loads(await grounded_links(system, payload))
        result["links"][0]["target_evidence"]["quote"] = "Невстречающаяся цитата"
        return json.dumps(result)
    topics = [make_topic("a", "Производная", ["Производная"]),
              make_topic("b", "Спуск", ["Обновление"], ["Производная"])]
    graph = run(build_links(topics, links_only(bad), Encoder()))
    assert graph["prerequisites"] == []
    assert any(d["target_id"] == "b" and not d["prerequisite_ids"] for d in graph["requirements"])


def test_ocr_line_wrapping_is_canonicalized_but_words_cannot_change():
    excerpts = [{"material_id": "a", "block_id": 0, "text": "Some important\nconcept is used."}]
    evidence = Evidence(material_id="a", block_id=0, quote="important concept")
    assert valid_evidence(evidence, excerpts)
    assert evidence.quote == "important\nconcept"
    assert not valid_evidence(Evidence(material_id="a", block_id=0, quote="important different concept"), excerpts)


def test_evidence_cannot_salvage_partial_quote_or_reassign_source():
    excerpts = [{"material_id": "a", "block_id": 1, "text": "important concept appears here"}]
    assert not valid_evidence(Evidence(material_id="a", block_id=1,
                                       quote="FALSE important concept extra FALSE"), excerpts)
    assert not valid_evidence(Evidence(material_id="a", block_id=2, quote="important concept"), excerpts)
    assert not valid_evidence(Evidence(material_id="other", block_id=1, quote="important concept"), excerpts)


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
            assert "blocks" not in json.loads(payload), "Cached cards must not be extracted again"
            return await grounded_judge(system, payload)
        def completer(self, complexity, reasoning=False):
            return self.complete
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


def test_build_updates_cached_source_metadata_and_merges_across_weeks(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class Client:
        async def complete(self, system, payload):
            return await grounded_judge(system, payload)
        def completer(self, complexity, reasoning=False): return self.complete
        async def close(self): pass

    monkeypatch.setattr(api, "ModelClient", Client)
    monkeypatch.setattr(api, "get_encoder", Encoder)
    text = "Матрица описывает преобразование векторов."
    materials = []
    for mid, week, kind, revision in [("z", 2, "lab", "r2"), ("a", 1, "lecture", "r1")]:
        card = make_topic(mid, "Матрица", ["Преобразование векторов"])
        card.text = text
        card.sources[0].start = 0
        card.sources[0].end = len(text)
        card.sources[0].text = text
        materials.append({"id": mid, "markdown": text, "week": week, "kind": kind,
                          "filename": f"{mid}.pdf", "kind_label": kind, "material_revision": revision,
                          "topics": [card.model_dump()]})
    response = TestClient(api.app).post("/graph/build", json={"materials": materials})
    assert response.status_code == 200
    topic = response.json()["topics"][0]
    assert [s["week"] for s in topic["sources"]] == [1, 2]
    assert {s["kind"] for s in topic["sources"]} == {"lecture", "lab"}
    assert {s["material_revision"] for s in topic["sources"]} == {"r1", "r2"}


def test_joint_endpoint_returns_cards_and_cleaning_audit(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    class Client:
        async def complete(self, system, payload):
            return await extraction_stub(system, payload)
        def completer(self, complexity, reasoning=False):
            return self.complete
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", Client)
    response = TestClient(api.app).post("/topics/extract", json={"markdown": "Производная показывает скорость изменения функции.\n\nСпасибо за внимание!", "material_id": "m"})
    assert response.status_code == 200
    report = response.json()["report"]
    assert report["topics_version"] == "topics-v2"
    assert report["topics"][0]["sources"][0]["material_id"] == "m"
@pytest.mark.parametrize("text",["", "   ", "\n\r\n\t"])
def test_blank_material_cannot_publish_empty_graph(text):
    async def complete(*args):
        raise AssertionError("Blank material must not call LLM")
    with pytest.raises(ValueError,match="не содержит"):
        run(extract_topics(text,complete))


def test_clean_topic_title_strips_multipart_suffixes_and_balances_delimiters():
    assert clean_topic_title("Свойства среды – часть 1") == "Свойства среды"
    assert clean_topic_title("Свойства среды – часть 2") == "Свойства среды"
    assert clean_topic_title("Свойства среды (часть 3)") == "Свойства среды"
    assert clean_topic_title("Мир Вампуса (продолжение)") == "Мир Вампуса"
    assert clean_topic_title("Алгоритм A* (1)") == "Алгоритм A*"
    assert clean_topic_title("Мир Вампуса (описание среды") == "Мир Вампуса (описание среды)"
    assert clean_topic_title("Аргумент «Китайская комната") == "Аргумент «Китайская комната»"


def test_build_hierarchy_does_not_invent_parent_topic():
    t1 = make_topic("t1", "Доступность среды", ["свойства среды"])
    t1.parent_title = "Свойства среды"
    t2 = make_topic("t2", "Детерминированность среды", ["свойства среды"])
    t2.parent_title = "Свойства среды"

    async def stub_judge(system, payload):
        return json.dumps({"links": [], "requirements": []}, ensure_ascii=False)

    graph = run(build_links([t1, t2], links_only(stub_judge), Encoder()))
    assert {t["id"] for t in graph["topics"]} == {"t1", "t2"}
    assert graph["hierarchy"] == []


def placed(topic, week=None, kind=None):
    for source in topic.sources:
        source.week, source.kind = week, kind
    return topic


def pairs(graph):
    return {(e["source"], e["target"]) for e in graph["prerequisites"]}


def test_isolated_topic_is_merged_into_the_nearest_linked_topic():
    t1 = make_topic("t1", "Интеллектуальные агенты", ["понятие агента"])
    t2 = make_topic("t2", "Рациональность агента", ["критерий рациональности"], ["понятие агента"])
    t3 = make_topic("t3", "История искусственного интеллекта", ["исторические вехи"])

    graph = run(build_links([t1, t2, t3], grounded_judge, Encoder()))
    assert pairs(graph) == {("t1", "t2")}
    assert {t["id"] for t in graph["topics"]} == {"t1", "t2"}
    host = graph["aliases"]["t3"]
    assert "История искусственного интеллекта" in next(t["text"] for t in graph["topics"] if t["id"] == host)
    assert graph["merged_isolated"] == [{"id": "t3", "title": "История искусственного интеллекта", "into": host}]
    assert graph["quality"]["isolated"] == 0
    merged = next(t for t in graph["topics"] if t["id"] == host)
    other = next(t for t in graph["topics"] if t["id"] != host)
    # The merged card is asked about as a whole; the untouched one keeps its question.
    assert merged["question"] == "Почему верно: " + merged["title"] + "?"
    assert other["question"] == "Объясните тему"


def test_course_without_links_keeps_its_topics():
    topics = [make_topic("a", "Агенты", ["понятие агента"]), make_topic("b", "Среды", ["свойства среды"])]
    graph = run(build_links(topics, links_only(grounded_links), Encoder()))
    assert {t["id"] for t in graph["topics"]} == {"a", "b"}
    assert graph["merged_isolated"] == []


@pytest.mark.parametrize("same", [True, False])
def test_synonyms_merge_only_when_the_model_confirms_them(same):
    topics = [placed(make_topic("a", "Производная функции", ["Вычисление производной"]), week=1),
              placed(make_topic("b", "Дифференцирование", ["Производная сложной функции"]), week=2)]
    asked = []

    async def complete(system, payload):
        if system == SYNONYM_PROMPT:
            asked.extend(json.loads(payload)["pairs"])
            return json.dumps({"pairs": [{"a": "a", "b": "b", "same": same}]})
        return await links_only(grounded_links)(system, payload)

    graph = run(build_links(topics, complete, Encoder()))
    assert [(p["a"], p["b"]) for p in asked] == [("a", "b")]
    if same:
        assert [t["id"] for t in graph["topics"]] == ["a"]
        assert graph["aliases"] == {"b": "a"}
        assert [s["week"] for s in graph["topics"][0]["sources"]] == [1, 2]
    else:
        assert len(graph["topics"]) == 2


def test_later_week_topic_is_never_a_prerequisite():
    early = placed(make_topic("early", "Спуск", ["Обновление параметров"], ["Вычисление производной"]), week=1)
    late = placed(make_topic("late", "Производная", ["Вычисление производной"]), week=2)

    async def links(system, payload):
        raise AssertionError("No link check without candidates")

    graph = run(build_links([early, late], links_only(links), Encoder()))
    assert graph["prerequisites"] == []
    assert graph["requirements"][0]["reason"] == "Нет кандидатов в материалах курса"


def test_attach_links_practice_to_theory_and_keeps_week_order():
    lecture = placed(make_topic("L", "Матрицы", ["Определение матрицы"]), week=1, kind="lecture")
    lab = placed(make_topic("P", "Умножение матриц на практике", ["Перемножать матрицы"]), week=1, kind="lab")
    later = placed(make_topic("L2", "Ранг матрицы", ["Вычисление ранга"]), week=2, kind="lecture")
    seen = {}

    async def complete(system, payload):
        if system != ATTACH_PROMPT:
            return await links_only(grounded_links)(system, payload)
        for item in json.loads(payload)["topics"]:
            seen[item["id"]] = [c["id"] for c in item["candidates"]]
        # A later week cannot come first: L2 -> L is dropped.
        return attach_answer(payload, {"P": (["L"], ["L2"]), "L": (["L2"], [])})

    graph = run(build_links([lecture, lab, later], complete, Encoder()))
    assert seen["P"][0] == "L"
    assert pairs(graph) == {("L", "P"), ("P", "L2")}
    assert all(e["via"] == "attach" for e in graph["prerequisites"])
    assert graph["quality"]["attached_links"] == 2
    assert {"source": "L2", "target": "L"}.items() <= next(
        r for r in graph["rejected_links"] if r["validation_error"] == "week_order").items()


def test_redundant_edges_are_reduced_to_a_chain():
    topics = [make_topic(i, title, [f"Знание {i}"]) for i, title in
              [("A", "Множества"), ("B", "Отношения"), ("C", "Функции")]]

    graph = run(build_links(topics, with_attach({"C": (["B", "A"], []), "B": (["A"], [])}), Encoder()))
    assert pairs(graph) == {("A", "B"), ("B", "C")}
    assert [(e["source"], e["target"]) for e in graph["transitive_links"]] == [("A", "C")]
    assert graph["quality"]["depth"] == 3


def test_one_bad_quote_does_not_drop_the_other_links_of_the_batch():
    topics = [make_topic("D", "Производная", ["Вычисление производной"]),
              make_topic("M", "Матрицы", ["Операции с матрицами"]),
              make_topic("T", "Обучение модели", ["Обновление весов"],
                         ["Вычисление производной", "Операции с матрицами"])]

    async def links(system, payload):
        result = json.loads(await grounded_links(system, payload))
        for link in result["links"]:
            if link["requirement"] == "Операции с матрицами":
                link["prerequisite_evidence"]["quote"] = "Невстречающаяся цитата"
        return json.dumps(result)

    graph = run(build_links(topics, links_only(links), Encoder()))
    assert ("D", "T") in pairs(graph)
    assert any(r["validation_error"] == "Недостоверная цитата связи" for r in graph["rejected_links"])


def test_batch_sequential_reuse_passes_existing_topics():
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 1:
            return json.dumps({"remove": [], "topics": [{
                "title": "Свойства среды", "requires": [],
                "provides": ["Определение свойств среды"],
                "question": "Какие бывают свойства среды?",
                "block_ids": [b["id"] for b in data["blocks"]],
            }]}, ensure_ascii=False)
        else:
            assert "existing_topics" in data, "Second batch must receive existing_topics from first batch"
            assert any(e["title"] == "Свойства среды" for e in data["existing_topics"])
            return json.dumps({"remove": [], "topics": [{
                "title": "Свойства среды", "requires": [],
                "provides": ["Детерминированность среды"],
                "question": "Что такое детерминированность среды?",
                "block_ids": [b["id"] for b in data["blocks"]],
            }]}, ensure_ascii=False)

    source = "Блок 1 о свойствах среды.\n\nБлок 2 о свойствах среды."
    settings = ExtractionSettings(max_chars=1500, max_blocks=1)
    result = run(extract_topics(source, complete, settings=settings))
    assert len(calls) >= 2
    assert len(result["report"]["topics"]) == 1
    assert result["report"]["topics"][0]["title"] == "Свойства среды"


def test_resolve_parent_topic_fuzzy_and_containment():
    from course_graph.topics.extract import normalized_title
    from course_graph.topics.graph import build_hierarchy, resolve_parent_topic
    from course_graph.topics.models import Source, Topic

    src = [Source(material_id="m", block_id=0, start=0, end=10, text="sample text")]
    t_parent = Topic(
        id="t_parent",
        title="Линейные модели в машинном обучении",
        requires=[],
        provides=["Основы линейных моделей"],
        question="Что такое линейные модели?",
        text="text",
        sources=src,
        parent_title=None,
    )
    t_child = Topic(
        id="t_child",
        title="Линейная регрессия",
        requires=["Линейные модели"],
        provides=["Метод МНК"],
        question="Как работает линейная регрессия?",
        text="text",
        sources=src,
        parent_title="Линейные модели",
    )
    t_unrelated = Topic(
        id="t_unrelated",
        title="Деревья решений",
        requires=[],
        provides=["Жадное построение деревьев"],
        question="Что такое деревья решений?",
        text="text",
        sources=src,
        parent_title=None,
    )

    topics = [t_parent, t_child, t_unrelated]
    by_norm = {normalized_title(t.title): t for t in topics}

    # 1. Fuzzy match where parent_title is a subset of parent topic title
    matched = resolve_parent_topic("Линейные модели", topics, by_norm)
    assert matched is not None
    assert matched.id == t_parent.id

    # 2. Match where parent topic title is a subset of parent_title
    matched2 = resolve_parent_topic("Линейные модели в машинном обучении и анализе данных", topics, by_norm)
    assert matched2 is not None
    assert matched2.id == t_parent.id

    # 3. Unrelated parent title returns None
    matched_none = resolve_parent_topic("Нейронные сети и глубокое обучение", topics, by_norm)
    assert matched_none is None

    # 4. build_hierarchy builds the link from t_parent to t_child
    _, hierarchy, hierarchy_adj = build_hierarchy(topics, [], {})
    assert len(hierarchy) == 1
    assert hierarchy[0]["source"] == "t_parent"
    assert hierarchy[0]["target"] == "t_child"
    assert "t_child" in hierarchy_adj.get("t_parent", set())


def test_thin_topics_are_merged_into_the_nearest_topic_without_losing_text():
    body = "Муравьи откладывают феромон на пройденном пути, и следующие муравьи чаще выбирают путь с большим следом. " * 2
    demo = "## Демонстрация\n\nДемонстрация алгоритма на Python."
    near = make_topic("near", "Феромоновые следы", ["Объяснять роль феромона"], text=body)
    thin = make_topic("thin", "Демонстрация ACO", ["Видеть работу ACO"], requires=["Python"], text=demo)
    thin.sources[0].start, thin.sources[0].end = 10, 10 + len(demo)
    far = make_topic("far", "Задача коммивояжёра", ["Формулировать TSP"], text=body)
    far.sources[0].material_id = "other"
    kept, merged = absorb_thin_topics([near, thin, far], 120)
    assert [t.id for t in kept] == ["near", "far"]
    assert merged == [{"title": "Демонстрация ACO", "into": "Феромоновые следы"}]
    assert demo in kept[0].text and "Видеть работу ACO" in kept[0].provides and "Python" in kept[0].requires
    assert content_chars(demo) < 120 <= content_chars(body)


def test_a_material_of_only_thin_topics_keeps_them():
    a = make_topic("a", "Заголовок", ["Знание"], text="# Заголовок")
    kept, merged = absorb_thin_topics([a], 120)
    assert kept == [a] and merged == []


def test_extraction_asks_reasoning_questions_and_no_empty_topics():
    assert "на рассуждение" in PROMPT and "Кто и когда" in PROMPT
    assert "ТЕМА БЕЗ СОДЕРЖАНИЯ" in PROMPT


def test_merging_thin_topics_keeps_cards_within_the_contract():
    body = "Муравьи откладывают феромон на пройденном пути, и следующие муравьи чаще выбирают путь с большим следом. " * 2
    def knowledge(prefix):
        return [f"{prefix} {i}" for i in range(12)]
    main = make_topic("main", "Феромоновые следы", knowledge("знание main"), requires=knowledge("основа main"), text=body)
    thin = []
    for n in range(10):
        topic = make_topic(f"thin{n}", f"Пример {n}", knowledge(f"знание {n}"), requires=knowledge(f"основа {n}"), text="Пример.")
        topic.sources[0].start = topic.sources[0].end = 10 + n
        thin.append(topic)
    kept, merged = absorb_thin_topics([main, *thin], 120)
    # Nine thin topics fit into the 128 items of the card; the tenth stays on
    # its own instead of losing knowledge.
    assert len(merged) == 9 and len(kept) == 2
    for topic in kept:
        Topic.model_validate(topic.model_dump())
    assert len(kept[0].provides) == 120 and len(kept[0].requires) == 120


class TitleEncoder:
    """Vectors chosen per topic title; any text starting with the title gets them."""
    def __init__(self, vectors):
        self.vectors = vectors

    def encode(self, texts, query=False):
        return np.array([next((v for k, v in self.vectors.items() if t.startswith(k)), [0.0, 0.0]) for t in texts])


def test_isolated_topic_never_moves_its_host_before_a_prerequisite():
    # #113: merging the week 1 topic into B would put B before its prerequisite A.
    a = placed(make_topic("A", "Базовая теория", ["Основа"]), week=3)
    b = placed(make_topic("B", "Целевая тема", ["Применение"]), week=4)
    i = placed(make_topic("I", "Ранняя тема", ["Введение"]), week=1)
    encoder = TitleEncoder({"Базовая теория": [1.0, 0.0], "Целевая тема": [0.0, 1.0], "Ранняя тема": [0.0, 1.0]})

    graph = run(build_links([a, b, i], with_attach({"B": (["A"], [])}), encoder))
    assert pairs(graph) == {("A", "B")}
    assert graph["aliases"]["I"] == "A"
    weeks = {t["id"]: min(s["week"] for s in t["sources"]) for t in graph["topics"]}
    assert all(weeks[e["source"]] <= weeks[e["target"]] for e in graph["prerequisites"])
    assert "Ранняя тема" in next(t["text"] for t in graph["topics"] if t["id"] == "A")


@pytest.mark.parametrize("weeks, host", [([1], "P"), ([5, 1], "P"), ([None], "H")])
def test_isolated_merge_respects_the_earliest_week_of_its_sources(weeks, host):
    prerequisite = placed(make_topic("P", "Другое понятие", ["Другое"]), week=2)
    linked = make_topic("H", "Тема хоста", ["Хост"])
    orphan = make_topic("O", "Тема хоста подробнее", ["Подробности"])
    orphan.sources = [Source(material_id=f"o{n}", block_id=0, start=0, end=len(orphan.text), text=orphan.text, week=w)
                      for n, w in enumerate(weeks)]
    edges = [{"source": "P", "target": "H"}]

    topics, aliases, merged, originals = merge_isolated([prerequisite, linked, orphan], edges, None)
    assert aliases == {"O": host}
    assert set(originals) == {host}
    assert [t.id for t in topics] == ["P", "H"]


@pytest.mark.parametrize("field", ["provides", "requires"])
@pytest.mark.parametrize("size, fits", [(127, True), (128, False)])
def test_join_topic_never_cuts_knowledge(field, size, fits):
    # #114: one new item on top of 127 makes 128 and fits; on top of 128 it does not.
    host = make_topic("host", "Основная тема", ["Основа"])
    setattr(host, field, [f"Знание {n}" for n in range(size)])
    extra = make_topic("extra", "Дополнительная тема", ["Дополнение"])
    setattr(extra, field, ["Новое знание"])
    before = host.model_copy(deep=True)

    assert join_topic(host, extra) is fits
    if fits:
        assert "Новое знание" in getattr(host, field) and len(getattr(host, field)) == 128
    else:
        assert host == before


def test_join_topic_counts_duplicates_and_explained_requirements_before_the_limit():
    # 127 + a duplicate + one new item is 128; 128 + 1 requirements minus the
    # one the extra card explains is 128.
    host = make_topic("host", "Основная тема", [f"Знание {n}" for n in range(127)])
    host.requires = [f"Основа {n}" for n in range(128)]
    extra = make_topic("extra", "Дополнительная тема", ["Знание 0", "Основа 5"], ["Основа 200"])

    assert join_topic(host, extra)
    assert len(host.provides) == 128 and "Основа 5" in host.provides
    assert "Основа 5" not in host.requires and "Основа 200" in host.requires
    assert len(host.requires) == 128


def test_merge_is_undone_when_no_question_can_be_written():
    # #115: the old question must not stand for the merged card.
    t1 = make_topic("t1", "Интеллектуальные агенты", ["понятие агента"])
    t2 = make_topic("t2", "Рациональность агента", ["критерий рациональности"], ["понятие агента"])
    t3 = make_topic("t3", "История искусственного интеллекта", ["исторические вехи"])

    async def complete(system, payload):
        if system == QUESTION_PROMPT:
            return json.dumps({"questions": []})
        return await grounded_judge(system, payload)

    graph = run(build_links([t1, t2, t3], complete, Encoder()))
    assert {t["id"] for t in graph["topics"]} == {"t1", "t2", "t3"}
    assert "t3" not in graph["aliases"] and graph["merged_isolated"] == []
    assert all(t["question"] == "Объясните тему" for t in graph["topics"])
    assert "История" not in next(t["text"] for t in graph["topics"] if t["id"] == "t1")
    assert graph["quality"]["isolated"] == 1


def test_untouched_cards_ask_for_no_question():
    asked = []

    async def complete(system, payload):
        if system == QUESTION_PROMPT:
            asked.append(payload)
        return await grounded_judge(system, payload)

    topics = [make_topic("T2", "Производная", ["Вычисление производной"]),
              make_topic("T20", "Спуск", ["Обновление параметров"], ["Вычисление производной"])]
    run(build_links(topics, complete, Encoder()))
    assert asked == []


@pytest.mark.parametrize("second", ["empty", "fixed"])
def test_a_confirmed_link_survives_the_retry_of_another_one(second):
    # #116: D -> T is grounded, M -> T is not; the retry must not drop D -> T.
    topics = [make_topic("D", "Производная", ["Вычисление производной"]),
              make_topic("M", "Матрицы", ["Операции с матрицами"]),
              make_topic("T", "Оптимизация", ["Обновление весов"], ["Производная матрицы"])]

    async def judge(system, payload):
        data = json.loads(payload)
        task = data["tasks"][0]
        links = []
        for candidate in task["candidates"]:
            evidence = quote(candidate["evidence"][0])
            if candidate["id"] == "M" and ("correction" not in data or second == "empty"):
                evidence["quote"] = "Невстречающаяся цитата"
            links.append({"target_id": "T", "prerequisite_id": candidate["id"], "requirement": task["requirement"],
                          "reason": "Использует это знание", "target_evidence": quote(task["evidence"][0]),
                          "prerequisite_evidence": evidence})
        if "correction" in data and second == "empty":
            links = []
        return json.dumps({"links": links, "requirements": []}, ensure_ascii=False)

    graph = run(build_links(topics, links_only(judge), Encoder()))
    edges = [(e["source"], e["target"]) for e in graph["prerequisites"]]
    decision = next(d for d in graph["requirements"] if d["requirement"] == "Производная матрицы")
    quote_errors = [r for r in graph["rejected_links"] if r.get("validation_error") == "Недостоверная цитата связи"]
    if second == "empty":
        assert edges == [("D", "T")]
        assert decision["prerequisite_ids"] == ["D"]
        assert [r["prerequisite_id"] for r in quote_errors] == ["M"]
    else:
        assert sorted(edges) == [("D", "T"), ("M", "T")]
        assert sorted(decision["prerequisite_ids"]) == ["D", "M"]
        assert quote_errors == []


@pytest.mark.parametrize("answer, linked", [("missing", False), ("fabricated", False),
                                            ("swapped", False), ("grounded", True)])
def test_attach_links_need_quotes_from_both_sides(answer, linked):
    # #117: a reason alone does not publish a link.
    topics = [make_topic("A", "Множества", ["Понятие множества"]),
              make_topic("B", "Отношения", ["Понятие отношения"])]

    async def complete(system, payload):
        if system != ATTACH_PROMPT:
            return await links_only(grounded_links)(system, payload)
        if answer == "missing":
            return json.dumps({"topics": [{"id": "B", "prerequisites": [{"id": "A", "reason": "Модель так решила"}]}]})
        result = json.loads(attach_answer(payload, {"B": (["A"], [])}))
        decision = next(item for item in result["topics"] if item["id"] == "B")
        for link in decision["prerequisites"]:
            if answer == "fabricated":
                link["candidate_evidence"]["quote"] = "Невстречающаяся цитата"
            if answer == "swapped":
                link["topic_evidence"], link["candidate_evidence"] = link["candidate_evidence"], link["topic_evidence"]
        return json.dumps(result, ensure_ascii=False)

    graph = run(build_links(topics, complete, Encoder()))
    assert (pairs(graph) == {("A", "B")}) is linked
    if linked:
        edge = graph["prerequisites"][0]
        assert edge["target_evidence"]["material_id"] == "B" and edge["prerequisite_evidence"]["material_id"] == "A"


@pytest.mark.parametrize("explained", [False, True])
def test_requirements_follow_the_merged_topics(explained):
    # #118: decisions of merged topics move to their host, once per requirement.
    a = make_topic("A", "Производная", ["Вычисление производной"])
    b = make_topic("B", "Спуск", ["Обновление параметров"], ["Вычисление производной"])
    i = make_topic("I", "История", ["Исторические вехи"], ["Недоступная основа"])
    j = make_topic("J", "Хронология", ["Недоступная основа"] if explained else ["Хронология событий"],
                   [] if explained else ["Недоступная основа"])

    async def judge(system, payload):
        result = json.loads(await grounded_links(system, payload))
        result["links"] = [link for link in result["links"] if link["target_id"] == "B"]
        result["requirements"] = []
        return json.dumps(result, ensure_ascii=False)

    graph = run(build_links([a, b, i, j], links_only(judge), Encoder()))
    ids = {t["id"] for t in graph["topics"]}
    assert ids == {"A", "B"} and graph["aliases"] == {"I": "A", "J": "A"}
    for decision in graph["requirements"]:
        assert decision["target_id"] in ids and set(decision["prerequisite_ids"]) <= ids
    open_base = [d for d in graph["requirements"] if d["requirement"] == "Недоступная основа"]
    assert open_base == ([] if explained else [{"target_id": "A", "requirement": "Недоступная основа",
                                                 "prerequisite_ids": [], "reason": open_base[0]["reason"]}])
    assert graph["quality"]["requirements"] == len(graph["requirements"])
    assert graph["quality"]["unresolved_requirements"] == sum(not d["prerequisite_ids"] for d in graph["requirements"])


def test_depth_is_counted_without_recursion():
    # #119: long chains, branches, shared descendants, repeated edges and cycles.
    def chain(n):
        return [{"source": str(k), "target": str(k + 1)} for k in range(n - 1)]
    assert depth(chain(1501)) == 1501
    assert depth(chain(2000)) == 2000
    assert depth([]) == 0
    diamond = [{"source": s, "target": t} for s, t in
               [("a", "b"), ("a", "c"), ("b", "d"), ("c", "d"), ("b", "d"), ("d", "e")]]
    assert depth(diamond) == 4
    with pytest.raises(ValueError):
        depth([{"source": "a", "target": "b"}, {"source": "b", "target": "a"}])


def test_question_of_a_merged_card_sees_a_source_added_after_the_budget():
    # Re-review 1: the absorbed fragment comes after 6000 characters of the host.
    old = "Старое объяснение темы. " * 334
    added = "Коэффициент новой теоремы равен 137."
    host = make_topic("H", "Основная тема", ["Основа"], text=old)
    host.sources.append(Source(material_id="I", block_id=0, start=0, end=len(added), text=added))
    host.text = old + "\n\n" + added
    sent = []

    async def complete(system, payload):
        sent.extend(json.loads(payload)["topics"])
        return write_questions(payload)

    failed, _ = run(refresh_questions([host], complete))
    assert failed == set()
    assert added in sent[0]["text"] and len(sent[0]["text"]) <= 6000
    assert host.question == "Почему верно: Основная тема?"


def test_question_is_not_confirmed_when_the_sources_do_not_fit_the_budget():
    host = make_topic("H", "Основная тема", ["Основа"], text="x" * 300)
    host.sources = [Source(material_id=f"m{n}", block_id=0, start=0, end=300, text="x" * 300) for n in range(40)]
    host.text = "\n\n".join(s.text for s in host.sources)

    async def never(system, payload):
        raise AssertionError("No question without enough of every source")

    failed, requests = run(refresh_questions([host], never))
    assert failed == {"H"} and requests == 0
    assert host.question == "Объясните тему"


@pytest.mark.parametrize("damage", ["empty_quote", "missing_field", "wrong_type"])
def test_a_malformed_link_does_not_take_the_valid_ones_with_it(damage):
    # Re-review 2: a schema error in M -> T must not drop the grounded D -> T.
    topics = [make_topic("D", "Производная", ["Вычисление производной"]),
              make_topic("M", "Матрицы", ["Операции с матрицами"]),
              make_topic("T", "Оптимизация", ["Обновление весов"], ["Производная матрицы"])]

    async def judge(system, payload):
        data = json.loads(payload)
        if "correction" in data:
            return json.dumps({"links": [], "requirements": []})
        task = data["tasks"][0]
        links = []
        for candidate in task["candidates"]:
            link = {"target_id": "T", "prerequisite_id": candidate["id"], "requirement": task["requirement"],
                    "reason": "Использует это знание", "target_evidence": quote(task["evidence"][0]),
                    "prerequisite_evidence": quote(candidate["evidence"][0])}
            if candidate["id"] == "M":
                if damage == "empty_quote":
                    link["prerequisite_evidence"]["quote"] = ""
                elif damage == "missing_field":
                    del link["prerequisite_evidence"]
                else:
                    link["prerequisite_evidence"]["block_id"] = "0"
            links.append(link)
        return json.dumps({"links": links, "requirements": []}, ensure_ascii=False)

    graph = run(build_links(topics, links_only(judge), Encoder()))
    assert [(e["source"], e["target"]) for e in graph["prerequisites"]] == [("D", "T")]
    assert any(r["validation_error"].startswith("Неверная запись связи") and r["prerequisite_id"] == "M"
               for r in graph["rejected_links"])


@pytest.mark.parametrize("pairs_answer, merged", [
    ([{"a": "a", "b": "b", "same": True}, {"a": "a", "b": "b", "same": False}], False),
    ([{"a": "a", "b": "b", "same": True}, {"a": "b", "b": "a", "same": True}], False),
    ([{"a": "a", "b": "b", "same": True}, {"a": "a", "b": "x", "same": True}], False),
    ([{"a": "b", "b": "a", "same": True}], True),
])
def test_synonym_answers_must_decide_each_pair_once(pairs_answer, merged):
    # Re-review 3: duplicate, contradictory or unknown decisions never merge.
    topics = [make_topic("a", "Производная функции", ["Вычисление производной"]),
              make_topic("b", "Дифференцирование", ["Производная сложной функции"])]

    async def complete(system, payload):
        if system == SYNONYM_PROMPT:
            return json.dumps({"pairs": pairs_answer})
        return await links_only(grounded_links)(system, payload)

    graph = run(build_links(topics, complete, Encoder()))
    assert (len(graph["topics"]) == 1) is merged


class SlowEncoder(Encoder):
    def encode(self, texts, query=False):
        time.sleep(0.2)
        return super().encode(texts, query)


def test_embeddings_do_not_block_the_event_loop():
    # Re-review 4: a slow encoder must not stall other requests of the service.
    topics = [make_topic("D", "Производная", ["Вычисление производной"]),
              make_topic("T", "Спуск", ["Обновление параметров"], ["Вычисление производной"]),
              make_topic("I", "История", ["Исторические вехи"])]

    async def scenario():
        delays, done = [], False

        async def ticker():
            while not done:
                started = time.perf_counter()
                await asyncio.sleep(0.01)
                delays.append(time.perf_counter() - started)
        task = asyncio.create_task(ticker())
        await build_links(topics, grounded_judge, SlowEncoder())
        done = True
        await task
        return max(delays)
    assert run(scenario()) < 0.15


@pytest.mark.parametrize("field", ["target_id", "requirement", "prerequisite_id"])
@pytest.mark.parametrize("value", [[], ["bad"], {"bad": 1}])
def test_a_link_with_non_string_ids_is_rejected_alone(field, value):
    # Third review: a list or object in an id field is not a key and must not fail the build.
    topics = [make_topic("D", "Производная", ["Вычисление производной"]),
              make_topic("M", "Матрицы", ["Операции с матрицами"]),
              make_topic("T", "Оптимизация", ["Обновление весов"], ["Производная матрицы"])]

    async def judge(system, payload):
        data = json.loads(payload)
        if "correction" in data:
            return json.dumps({"links": [], "requirements": []})
        task = data["tasks"][0]
        links = []
        for candidate in task["candidates"]:
            link = {"target_id": "T", "prerequisite_id": candidate["id"], "requirement": task["requirement"],
                    "reason": "Использует это знание", "target_evidence": quote(task["evidence"][0]),
                    "prerequisite_evidence": quote(candidate["evidence"][0])}
            if candidate["id"] == "M":
                link[field] = value
            links.append(link)
        return json.dumps({"links": links, "requirements": []}, ensure_ascii=False)

    graph = run(build_links(topics, links_only(judge), Encoder()))
    assert [(e["source"], e["target"]) for e in graph["prerequisites"]] == [("D", "T")]
    broken = [r for r in graph["rejected_links"] if r["validation_error"].startswith("Неверная запись связи")]
    assert len(broken) == 1 and broken[0][field] is None and broken[0]["raw"][field] == value


def synonym_topics():
    return [make_topic("A", "Альфа", ["Знание альфа"]), make_topic("B", "Бета", ["Знание бета"]),
            make_topic("C", "Гамма", ["Знание гамма"])]


def synonym_answers(decisions, batches):
    """Answers each asked pair from decisions; records the pairs of every call."""
    async def complete(system, payload):
        asked = json.loads(payload)["pairs"]
        batches.append({frozenset((p["a"], p["b"])) for p in asked})
        return json.dumps({"pairs": [{"a": p["a"], "b": p["b"], "same": decisions[frozenset((p["a"], p["b"]))]}
                                     for p in asked]})
    return complete


@pytest.mark.parametrize("max_chars", [12000, 300])
def test_contradictory_synonym_decisions_never_merge_their_group(max_chars):
    # A=B, B=C, A≠C — in one batch, or one pair per batch with max_chars=300.
    encoder = TitleEncoder({"Альфа": [1.0, 0.0], "Бета": [1.0, 0.0], "Гамма": [1.0, 0.0]})
    decisions = {frozenset("AB"): True, frozenset("BC"): True, frozenset("AC"): False}
    batches = []

    kept, aliases, requests = run(merge_synonyms(synonym_topics(), synonym_answers(decisions, batches),
                                                 encoder, lambda t: t.id, max_chars=max_chars))
    assert [t.id for t in kept] == ["A", "B", "C"] and aliases == {}
    assert len(batches) == (1 if max_chars == 12000 else 3) and set().union(*batches) == set(decisions)


def test_a_consistent_synonym_chain_still_merges():
    # A~B and B~C are close, A and C are not asked: the chain is one concept.
    angle = {"Альфа": 0.0, "Бета": 0.38, "Гамма": 0.76}
    encoder = TitleEncoder({k: [np.cos(a), np.sin(a)] for k, a in angle.items()})
    decisions = {frozenset("AB"): True, frozenset("BC"): True}
    batches = []

    kept, aliases, _ = run(merge_synonyms(synonym_topics(), synonym_answers(decisions, batches),
                                          encoder, lambda t: t.id))
    assert set().union(*batches) == set(decisions)
    assert [t.id for t in kept] == ["A"] and aliases == {"B": "A", "C": "A"}
    assert {s.text for s in kept[0].sources} == {"Альфа", "Бета", "Гамма"}
    assert set(kept[0].provides) == {"Знание альфа", "Знание бета", "Знание гамма"}


@pytest.mark.parametrize("endpoint", ["/topics/extract", "/graph/links", "/graph/build"])
def test_every_stage_asks_for_the_low_complexity(monkeypatch, endpoint):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    asked = []

    class Client:
        async def complete(self, system, payload):
            if "blocks" in json.loads(payload):
                return await extraction_stub(system, payload)
            return await grounded_judge(system, payload)
        def completer(self, complexity, reasoning=False):
            asked.append((complexity, reasoning))
            return self.complete
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", Client)
    monkeypatch.setattr(api, "get_encoder", Encoder)
    topics = [make_topic("a", "Производная", ["Вычисление производной"]),
              make_topic("b", "Оптимизация", ["Обновление параметров"], ["Вычисление производной"])]
    if endpoint == "/topics/extract":
        body = {"markdown": "Производная показывает скорость изменения функции.", "material_id": "m"}
    elif endpoint == "/graph/links":
        body = {"topics": [t.model_dump() for t in topics]}
    else:
        body = {"materials": [{"id": t.id, "markdown": t.text, "topics": [t.model_dump()]} for t in topics]}
    response = TestClient(api.app).post(endpoint, json=body)
    assert response.status_code == 200
    assert asked and {complexity for complexity, _ in asked} == {"low"}
    assert ("low", True) in asked
    if endpoint != "/topics/extract":
        assert ("low", False) in asked


def test_requirements_use_only_edges_left_after_transitive_reduction():
    topics = [make_topic("A", "Множества", ["Понятие множества"]),
              make_topic("B", "Отношения", ["Понятие отношения"], ["Понятие множества"]),
              make_topic("C", "Композиция", ["Композиция отношений"],
                         ["Понятие множества", "Понятие отношения"])]

    async def complete(system, payload):
        data = json.loads(payload)
        for task in data["tasks"]:
            task["candidates"].sort(key=lambda c: task["requirement"] not in c["provides"])
        return await grounded_links(system, json.dumps(data))

    graph = run(build_links(topics, links_only(complete), Encoder()))
    assert {(e["source"], e["target"]) for e in graph["prerequisites"]} == {("A", "B"), ("B", "C")}
    assert [(e["source"], e["target"]) for e in graph["transitive_links"]] == [("A", "C")]
    decisions = {(d["target_id"], d["requirement"]): d for d in graph["requirements"]}
    assert decisions[("C", "Понятие множества")]["prerequisite_ids"] == []
    assert "transitive_links" in decisions[("C", "Понятие множества")]["reason"]
    assert decisions[("C", "Понятие отношения")]["prerequisite_ids"] == ["B"]
    assert decisions[("B", "Понятие множества")]["prerequisite_ids"] == ["A"]
    assert graph["quality"]["requirements"] == 3
    assert graph["quality"]["unresolved_requirements"] == 1


@pytest.mark.parametrize("damage", ["missing_quote", "empty_quote", "list_id", "dict_id", "missing_reason", "not_object", "not_list"])
@pytest.mark.parametrize("second", ["empty", "invalid", "repeated", "fixed"])
def test_attach_preserves_good_neighbors_when_one_record_is_malformed(damage, second):
    from course_graph.topics.structure import attach_links
    topics = [make_topic("A", "Множества", ["Понятие множества"]),
              make_topic("B", "Отношения", ["Понятие отношения"]),
              make_topic("C", "Композиция", ["Композиция отношений"])]
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 2:
            assert [t["id"] for t in data["topics"]] == ["C"]
            assert "correction" in data
            if second == "empty":
                return '{"topics":[]}'
            if second == "invalid":
                return '{"topics":'
            if second == "fixed":
                return attach_answer(payload, {"C": (["B"], [])})
        result = json.loads(attach_answer(payload, {"B": (["A"], []), "C": (["B"], [])}))
        target = next(t for t in result["topics"] if t["id"] == "C")
        broken = target["prerequisites"][0]
        if damage == "missing_quote":
            del broken["candidate_evidence"]
        elif damage == "empty_quote":
            broken["candidate_evidence"]["quote"] = ""
        elif damage in ("list_id", "dict_id"):
            broken["id"] = ["B"] if damage == "list_id" else {"id": "B"}
        elif damage == "missing_reason":
            del broken["reason"]
        elif damage == "not_object":
            target["prerequisites"][0] = "broken"
        else:
            target["prerequisites"] = "broken"
        return json.dumps(result)

    links, rejected, requests = run(attach_links(topics, [], {}, complete, Encoder()))
    pairs = {(e["source"], e["target"]) for e in links}
    assert ("A", "B") in pairs
    assert (("B", "C") in pairs) == (second == "fixed")
    assert requests == len(calls) == 2
    if second != "fixed":
        assert any("raw" in r and "запись" in r["validation_error"].lower() for r in rejected)


@pytest.mark.parametrize("missing", ["absent", "omitted", "invalid", "null", "object"])
def test_missing_requirement_decisions_are_retried_without_losing_neighbors(missing):
    topics = [make_topic("A", "Основа", ["Первое знание", "Второе знание"]),
              make_topic("B", "Применение", ["Практическое применение"], ["Первое знание", "Второе знание"])]
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 2:
            expected = {"Первое знание", "Второе знание"} if missing in ("absent", "null", "object") else {"Второе знание"}
            assert {t["requirement"] for t in data["tasks"]} == expected
            assert "correction" in data
            return await grounded_links(system, payload)
        if missing == "absent":
            return '{"links":[]}'
        result = json.loads(await grounded_links(system, payload))
        result["links"] = [l for l in result["links"] if l["requirement"] == "Первое знание"]
        if missing in ("null", "object"):
            result["requirements"] = None if missing == "null" else {}
        elif missing == "omitted":
            result["requirements"] = [d for d in result["requirements"] if d["requirement"] == "Первое знание"]
        else:
            next(d for d in result["requirements"] if d["requirement"] == "Второе знание")["prerequisite_ids"] = "bad"
        return json.dumps(result)

    graph = run(build_links(topics, links_only(complete), Encoder()))
    assert len(calls) == 2
    assert {d["requirement"] for d in graph["requirements"]} == {"Первое знание", "Второе знание"}
    assert all(d["prerequisite_ids"] == ["A"] for d in graph["requirements"])
    assert graph["quality"]["unresolved_requirements"] == 0


@pytest.mark.parametrize("answered", [False, True])
def test_explicit_refusal_is_distinct_from_missing_requirement_decisions(answered):
    topics = [make_topic("A", "Основа", ["Первое знание"]),
              make_topic("B", "Применение", ["Практическое применение"], ["Первое знание"])]
    calls = []

    async def complete(system, payload):
        tasks = json.loads(payload)["tasks"]
        calls.append(tasks)
        decisions = [{"target_id": t["target_id"], "requirement": t["requirement"],
                      "prerequisite_ids": [], "reason": "В источнике знание только упомянуто"} for t in tasks]
        return json.dumps({"links": [], "requirements": decisions} if answered else {"links": []})

    graph = run(build_links(topics, links_only(complete), Encoder()))
    assert len(calls) == (1 if answered else 2)
    reason = graph["requirements"][0]["reason"]
    assert reason == "В источнике знание только упомянуто" if answered else "после исправления" in reason
    assert graph["quality"]["unresolved_requirements"] == 1


@pytest.mark.parametrize("second", ["empty", "invalid"])
def test_grounded_links_survive_a_missing_decision_and_failed_correction(second):
    topics = [make_topic("A", "Основа", ["Первое знание"]),
              make_topic("B", "Применение", ["Практическое применение"], ["Первое знание"])]
    calls = []

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 1:
            answer = json.loads(await grounded_links(system, payload))
            answer["requirements"] = None
            return json.dumps(answer)
        return '{"links":[]}' if second == "empty" else '{"links":'

    graph = run(build_links(topics, links_only(complete), Encoder()))
    assert len(calls) == 2
    assert [(e["source"], e["target"]) for e in graph["prerequisites"]] == [("A", "B")]
    assert graph["requirements"][0]["prerequisite_ids"] == ["A"]
    assert graph["quality"]["unresolved_requirements"] == 0


def test_mechanical_slips_are_fixed_without_losing_text():
    # Three blocks; the model slips the way weak models do: a foreign id in a
    # removal and in a topic, an invented reason, a block both removed and
    # used, a repeated block and a topic of foreign blocks only.
    source = "Производная показывает скорость изменения функции.\n\nГрадиент обобщает производную.\n\nСлайд 3"
    async def sloppy(system, payload):
        ids = [b["id"] for b in json.loads(payload)["blocks"]]
        first, second, third = ids
        return json.dumps({"remove": [
            {"id": 99, "reason": "thanks"},
            {"id": second, "reason": "thanks"},
            {"id": third, "reason": "slide_number"},
        ], "topics": [
            {"title": "Производная", "requires": [], "provides": ["Вычислять производную"],
             "question": "Почему производная описывает скорость изменения?", "block_ids": [first, second, second, 42]},
            {"title": "Чужая тема", "requires": [], "provides": ["Ничего"],
             "question": "Почему?", "block_ids": [77]},
        ]})
    result = run(extract_topics(source, sloppy, "m"))
    report = result["report"]
    assert [t["title"] for t in report["topics"]] == ["Производная"]
    text = report["topics"][0]["text"]
    for part in ("Производная показывает", "Градиент обобщает", "Слайд 3"):
        assert part in text
    assert result["markdown"] == source and report["removed"] == []
    assert report["tolerated_foreign_ids"] == 3
    assert report["tolerated_invalid_reasons"] == 1
    assert report["tolerated_used_removals"] == 1


@pytest.mark.parametrize("reasons", [("thanks", "unknown"), ("unknown", "thanks")])
def test_an_unknown_reason_among_repeats_keeps_the_block_in_any_order(reasons):
    # Review of PR #129: a valid reason listed first used to win and drop the
    # block silently, while the reverse order kept it.
    source = "Производная показывает скорость изменения функции.\n\nГрадиент задаёт направление наибольшего роста функции."
    async def contradictory(system, payload):
        first, second = [b["id"] for b in json.loads(payload)["blocks"]]
        return json.dumps({"remove": [{"id": second, "reason": r} for r in reasons], "topics": [
            {"title": "Производная", "requires": [], "provides": ["Смысл производной"],
             "question": "Почему производная показывает локальную скорость изменения?", "block_ids": [first]},
        ]})
    result = run(extract_topics(source, contradictory, "m"))
    report = result["report"]
    assert "Градиент задаёт направление" in report["topics"][0]["text"]
    assert result["markdown"] == source and report["removed"] == []
    assert report["tolerated_invalid_reasons"] == 1


def test_an_answer_without_topics_is_still_rejected():
    async def empty(system, payload):
        return '{"remove":[],"topics":[]}'
    with pytest.raises(ValueError):
        run(extract_topics("Производная\n\nГрадиент", empty))


@pytest.mark.parametrize("missing", ["partial", "empty", "unknown", "list_id", "dict_id", "duplicate", "missing_field"])
def test_attach_retries_only_topics_without_one_complete_decision(missing):
    from course_graph.topics.structure import attach_links
    topics = [make_topic("A", "Множества", ["Понятие множества"]),
              make_topic("B", "Отношения", ["Понятие отношения"]),
              make_topic("C", "Композиция", ["Композиция отношений"])]
    calls = []
    choose = {"A": ([], []), "B": (["A"], []), "C": (["B"], [])}

    async def complete(system, payload):
        data = json.loads(payload)
        calls.append(data)
        if len(calls) == 2:
            expected = {"A", "C"} if missing == "partial" else {"A", "B", "C"} if missing == "empty" else {"C"}
            assert {t["id"] for t in data["topics"]} == expected
            assert "correction" in data
            return attach_answer(payload, choose)
        result = json.loads(attach_answer(payload, choose))
        if missing == "partial":
            result["topics"] = [t for t in result["topics"] if t["id"] == "B"]
        elif missing == "empty":
            result["topics"] = []
        else:
            target = next(t for t in result["topics"] if t["id"] == "C")
            if missing in ("unknown", "list_id", "dict_id"):
                target["id"] = "X" if missing == "unknown" else ["C"] if missing == "list_id" else {"id": "C"}
            elif missing == "duplicate":
                result["topics"].append(dict(target))
            else:
                del target["dependents"]
        return json.dumps(result)

    links, rejected, requests = run(attach_links(topics, [], {}, complete, Encoder()))
    assert requests == len(calls) == 2
    assert {(e["source"], e["target"]) for e in links} == {("A", "B"), ("B", "C")}
    assert not any(r["validation_error"] == "incomplete_topic_decision" for r in rejected)


@pytest.mark.parametrize("answered", [False, True])
def test_attach_distinguishes_explicit_empty_decisions_from_missing_ones(answered):
    from course_graph.topics.structure import attach_links
    topics = [make_topic("A", "Множества", ["Понятие множества"]),
              make_topic("B", "Отношения", ["Понятие отношения"])]
    calls = []

    async def complete(system, payload):
        calls.append(payload)
        return attach_answer(payload, {t.id: ([], []) for t in topics}) if answered else '{"topics":[]}'

    links, rejected, requests = run(attach_links(topics, [], {}, complete, Encoder()))
    assert links == []
    assert requests == len(calls) == (1 if answered else 2)
    incomplete = [r for r in rejected if r["validation_error"] == "incomplete_topic_decision"]
    assert {r["topic_id"] for r in incomplete} == (set() if answered else {"A", "B"})


def long_knowledge(count):
    return [f"Знание {i:03d}: " + "я" * (600 - len(f"Знание {i:03d}: ")) for i in range(count)]


def test_question_refresh_skips_oversized_host_but_processes_small_neighbor():
    large = make_topic("L", "Большая тема", long_knowledge(128))
    small = make_topic("S", "Малая тема", ["Малое знание"])
    before = large.model_dump()
    calls = []

    async def complete(system, payload):
        calls.append(payload)
        assert len(payload) <= 20000
        assert [t["id"] for t in json.loads(payload)["topics"]] == ["S"]
        return write_questions(payload)

    failed, requests = run(refresh_questions([large, small], complete))
    assert failed == {"L"} and requests == len(calls) == 1
    assert large.model_dump() == before
    assert small.question.startswith("Почему верно:")


def test_attach_never_sends_the_oversized_valid_cards_from_the_review():
    from course_graph.topics.structure import attach_links
    topics = [make_topic(f"T{i}", f"Тема {i}", long_knowledge(8), long_knowledge(8)) for i in range(9)]
    before = [t.model_dump() for t in topics]

    async def never(*args):
        raise AssertionError("An oversized card must not reach the LLM")

    links, rejected, requests = run(attach_links(topics, [], {}, never, Encoder()))
    assert links == [] and requests == 0
    assert {r["topic_id"] for r in rejected if r["validation_error"] == "budget_exceeded"} == {t.id for t in topics}
    assert [t.model_dump() for t in topics] == before


def test_attach_processes_a_small_neighbor_when_a_large_item_cannot_fit():
    from course_graph.topics.structure import attach_links
    topics = [make_topic("L", "Большая тема", long_knowledge(128), long_knowledge(8)),
              make_topic("S", "Малая тема", ["Малое знание"])]
    calls = []

    async def complete(system, payload):
        calls.append(payload)
        assert len(payload) <= 6000
        assert [t["id"] for t in json.loads(payload)["topics"]] == ["S"]
        return attach_answer(payload, {"S": (["L"], [])})

    links, rejected, requests = run(attach_links(topics, [], {}, complete, Encoder(), max_chars=6000))
    assert [(e["source"], e["target"]) for e in links] == [("L", "S")]
    assert requests == len(calls) == 1
    assert any(r.get("topic_id") == "L" and r["validation_error"] == "budget_exceeded" for r in rejected)
    assert len(topics[0].provides) == 128


def test_question_correction_respects_the_budget_including_json_wrapper():
    from course_graph.topics.structure import question_text
    host = make_topic("H", "Тема", ["Знание"])
    item = {"id": host.id, "title": host.title, "provides": host.provides, "text": question_text(host)}
    budget = len(json.dumps({"topics": [item]}, ensure_ascii=False))
    calls = []

    async def complete(system, payload):
        calls.append(payload)
        assert len(payload) <= budget
        return '{"questions":[]}'

    failed, requests = run(refresh_questions([host], complete, max_chars=budget))
    assert failed == {"H"}
    assert requests == len(calls) == 1


def test_synonym_pass_does_not_send_a_pair_over_its_budget():
    topics = synonym_topics()
    before = [t.model_dump() for t in topics]

    async def never(*args):
        raise AssertionError("An oversized synonym pair must not reach the LLM")

    encoder = TitleEncoder({t.title: [1, 0] for t in topics})
    kept, aliases, requests = run(merge_synonyms(topics, never, encoder, lambda t: t.id, max_chars=1))
    assert aliases == {} and requests == 0
    assert [t.model_dump() for t in kept] == before


def test_graph_passes_keep_four_model_calls_in_flight():
    # One link batch per requirement: with two calls in flight half of the
    # gateway's four instances stayed idle during a build.
    # Long texts make each requirement fill most of a batch on its own.
    topics = [make_topic(f"t{i}", f"Тема {i}", [f"Знание {i}"], [f"Знание {i - 1}"] if i else [],
                         text=f"Тема {i}. " + "слово " * 160)
              for i in range(9)]
    flight = {"now": 0, "max": 0}

    async def slow(system, payload):
        flight["now"] += 1
        flight["max"] = max(flight["max"], flight["now"])
        await asyncio.sleep(0.01)
        flight["now"] -= 1
        return await grounded_links(system, payload)
    graph = run(build_links(topics, links_only(slow), Encoder(), per_requirement=1, max_chars=3500))
    assert graph["quality"]["requests"] >= 8
    assert flight["max"] == GRAPH_CONCURRENCY == 4
