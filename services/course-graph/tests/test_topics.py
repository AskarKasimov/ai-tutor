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
from course_graph.topics.graph import build_links, valid_evidence
from course_graph.topics.extract import join_topic
from course_graph.topics.structure import (
    ATTACH_PROMPT,
    QUESTION_PROMPT,
    SYNONYM_PROMPT,
    depth,
    merge_isolated,
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
            return json.dumps({"topics": []})
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
        if item["id"] not in choose:
            continue
        candidates = {c["id"]: c for c in item["candidates"]}

        def link(other):
            return {"id": other, "reason": "Используется это знание", "topic_evidence": quote(item["evidence"][0]),
                    "candidate_evidence": quote(candidates[other]["evidence"][0])}
        before, after = choose[item["id"]]
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
        def completer(self, complexity):
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
        def completer(self, complexity): return self.complete
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
        def completer(self, complexity):
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
        for link in result["topics"][0]["prerequisites"]:
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
