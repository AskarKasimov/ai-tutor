import asyncio
import json

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

    graph = run(build_links([t_normal, t_hw, t_links], stub_judge, Encoder()))
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
    graph = run(build_links(topics, bad, Encoder()))
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
            assert "tasks" in json.loads(payload), "Cached cards must not be extracted again"
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

    graph = run(build_links([t1, t2], stub_judge, Encoder()))
    assert {t["id"] for t in graph["topics"]} == {"t1", "t2"}
    assert graph["hierarchy"] == []


def test_independent_topic_remains_isolated_without_evidence():
    t1 = make_topic("t1", "Интеллектуальные агенты", ["понятие агента"])
    t2 = make_topic("t2", "Рациональность агента", ["критерий рациональности"], ["понятие агента"])
    t3 = make_topic("t3", "История искусственного интеллекта", ["исторические вехи"])

    graph = run(build_links([t1, t2, t3], grounded_judge, Encoder()))
    adj_nodes = set()
    for p in graph["prerequisites"]:
        adj_nodes.add(p["source"])
        adj_nodes.add(p["target"])
    for h in graph["hierarchy"]:
        adj_nodes.add(h["source"])
        adj_nodes.add(h["target"])

    assert "t3" not in adj_nodes
    assert {t["id"] for t in graph["topics"]} == {"t1", "t2", "t3"}


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


def test_every_stage_asks_for_the_low_complexity(monkeypatch):
    from fastapi.testclient import TestClient
    import course_graph.api as api

    asked = []

    class Client:
        async def complete(self, system, payload):
            return await extraction_stub(system, payload)
        def completer(self, complexity, reasoning=False):
            asked.append((complexity, reasoning))
            return self.complete
        async def close(self):
            pass

    monkeypatch.setattr(api, "ModelClient", Client)
    response = TestClient(api.app).post("/topics/extract", json={"markdown": "Производная показывает скорость изменения функции.", "material_id": "m"})
    assert response.status_code == 200
    # Topic extraction keeps the model's reasoning: without it Qwen3.6 broke
    # the strict removal format.
    assert asked and set(asked) == {("low", True)}


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


def test_an_answer_without_topics_is_still_rejected():
    async def empty(system, payload):
        return '{"remove":[],"topics":[]}'
    with pytest.raises(ValueError):
        run(extract_topics("Производная\n\nГрадиент", empty))
