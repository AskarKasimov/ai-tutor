import asyncio
import json

import pytest

from course_graph.topics.extract import merge_topics
from course_graph.topics.graph import build_hierarchy, build_links, validate_topology
from test_topics import Encoder, grounded_judge, make_topic


def test_merge_deduplicates_first_card_and_each_merged_source_list():
    first = make_topic("a", "Производная", ["Знание", "Знание"], ["Функция", "Функция"])
    first.sources *= 2
    second = make_topic("b", "ПРОИЗВОДНАЯ!", ["Знание", "Другое знание"])
    second.sources *= 2
    topics = merge_topics([first, second, second])
    assert len(topics) == 1
    assert topics[0].requires == ["Функция"]
    assert topics[0].provides == ["Знание", "Другое знание"]
    assert len(topics[0].sources) == 2
    assert topics[0].text == first.text + "\n\n" + second.text
    assert len(first.sources) == 2  # Caller-owned cards remain unchanged.


def test_duplicate_requirements_do_not_create_repeated_judgments_or_edges():
    prerequisite = make_topic("a", "Производная", ["Вычисление производной"])
    target = make_topic("b", "Оптимизация", ["Обновление"],
                        ["Вычисление производной", "Вычисление  производной"])
    graph = asyncio.run(build_links([prerequisite, target], grounded_judge, Encoder()))
    assert len(graph["requirements"]) == len(graph["prerequisites"]) == 1


@pytest.mark.parametrize("repeated", ["links", "requirements", "prerequisite_ids"])
def test_identical_model_repetitions_are_deduplicated_without_retry(repeated):
    topics = [make_topic("a", "Производная", ["Вычисление производной"]),
              make_topic("b", "Оптимизация", ["Обновление"], ["Вычисление производной"])]
    calls = []

    async def repeat(system, payload):
        calls.append(payload)
        decision = json.loads(await grounded_judge(system, payload))
        if repeated == "prerequisite_ids":
            decision["requirements"][0]["prerequisite_ids"] *= 2
        else:
            decision[repeated] *= 2
        return json.dumps(decision)

    graph = asyncio.run(build_links(topics, repeat, Encoder()))
    assert len(calls) == 1
    assert len(graph["requirements"]) == len(graph["prerequisites"]) == 1
    assert graph["requirements"][0]["prerequisite_ids"] == ["a"]


def test_conflicting_duplicate_model_decisions_are_not_silently_accepted():
    topics = [make_topic("a", "Производная", ["Вычисление производной"]),
              make_topic("b", "Оптимизация", ["Обновление"], ["Вычисление производной"])]

    async def contradictory(system, payload):
        decision = json.loads(await grounded_judge(system, payload))
        decision["requirements"].append({**decision["requirements"][0], "prerequisite_ids": []})
        return json.dumps(decision)

    graph = asyncio.run(build_links(topics, contradictory, Encoder()))
    assert graph["prerequisites"] == []
    assert len(graph["requirements"]) == 1
    assert graph["quality"]["unresolved_requirements"] == 1


@pytest.mark.parametrize("field,value", [("text", "Подменённый текст"), ("end", 100), ("block_id", 5)])
def test_conflicting_source_identity_is_rejected(field, value):
    first = make_topic("a", "Производная", ["Знание"])
    second = first.model_copy(deep=True)
    second.id = "b"
    second.title = "Другая тема"
    setattr(second.sources[0], field, value)
    with pytest.raises(ValueError, match="Противоречивые источники"):
        merge_topics([first, second])


def test_same_block_can_support_different_topics_without_being_erased():
    first = make_topic("a", "Производная", ["Знание"])
    second = first.model_copy(deep=True)
    second.id = "b"
    second.title = "Функция"
    assert all(len(t.sources) == 1 for t in merge_topics([first, second]))


def test_unicode_titles_merge_but_distinct_math_requirements_remain():
    first = make_topic("a", "Е\u0308мкость", ["f(x+y)", "f(x-y)"], ["Foo", "foo"])
    second = make_topic("b", "Ёмкость", ["Ещё знание"])
    merged = merge_topics([first, second])
    assert len(merged) == 1
    assert merged[0].requires == ["Foo", "foo"]
    assert merged[0].provides == ["f(x+y)", "f(x-y)", "Ещё знание"]


@pytest.mark.parametrize("titles", [("C", "C++", "C#"), ("x+y", "x-y"), ("ℝ", "R")])
def test_distinct_programming_and_mathematical_titles_are_not_merged(titles):
    topics = [make_topic(str(i), title, ["Знание"]) for i, title in enumerate(titles)]
    assert len(merge_topics(topics)) == len(titles)


@pytest.mark.parametrize("field,value", [("id", "a"), ("id", " "), ("title", "A!"), ("title", "!!!")])
def test_final_guard_rejects_duplicate_or_empty_topic_identity(field, value):
    first, second = make_topic("a", "A", ["Знание"]), make_topic("b", "B", ["Знание"])
    setattr(second, field, value)
    with pytest.raises(ValueError):
        validate_topology([first, second], [], [])


def test_repeated_id_is_rejected_before_model_calls_even_with_equal_titles():
    async def never(*args):
        raise AssertionError("Duplicate IDs must be rejected before inference")
    topic = make_topic("a", "Производная", ["Знание"])
    with pytest.raises(ValueError, match="Повторный ID"):
        asyncio.run(build_links([topic, topic], never, Encoder()))


@pytest.mark.parametrize("relation", ["prerequisites", "hierarchy"])
@pytest.mark.parametrize("pairs", [
    [("a", "a")], [("a", "missing")], [("a", "b"), ("a", "b")],
    [("a", "b"), ("b", "a")], [("a", "b"), ("b", "c"), ("c", "a")],
])
def test_final_guard_rejects_invalid_relations(relation, pairs):
    topics = [make_topic(id, id, [id]) for id in ("a", "b", "c")]
    edges = [{"source": source, "target": target, "requirement": "Знание"} for source, target in pairs]
    with pytest.raises(ValueError):
        validate_topology(topics, edges if relation == "prerequisites" else [],
                          edges if relation == "hierarchy" else [])


def test_final_guard_handles_long_dag_without_recursion_and_allows_distinct_evidence():
    topics = [make_topic(str(i), "Тема " + str(i), ["Знание"]) for i in range(1500)]
    edges = [{"source": str(i), "target": str(i+1), "requirement": "Первое знание"} for i in range(1499)]
    edges.append({"source": "0", "target": "1", "requirement": "Другое знание"})
    validate_topology(topics, edges, [{"source": "1", "target": "0"}])


def test_hierarchy_filters_long_cycle_self_reference_and_repeated_children():
    a, b, c = [make_topic(id, id, [id]) for id in ("a", "b", "c")]
    a.parent_title, b.parent_title, c.parent_title = "c", "a", "b"
    _, edges, _ = build_hierarchy([a, b, c, b], [], {})
    assert len(edges) == 2
    validate_topology([a, b, c], [], edges)
    a.parent_title = "a"
    _, edges, _ = build_hierarchy([a], [], {})
    assert edges == []


def test_three_node_model_cycle_is_rejected_across_separate_batches():
    topics = [make_topic(id, id, ["Знание " + id], ["Знание " + parent])
              for id, parent in [("a", "c"), ("b", "a"), ("c", "b")]]

    async def judge(system, payload):
        data = json.loads(payload)
        for task in data["tasks"]:
            required_id = {"a": "c", "b": "a", "c": "b"}[task["target_id"]]
            task["candidates"] = [c for c in task["candidates"] if c["id"] == required_id]
        return await grounded_judge(system, json.dumps(data))

    graph = asyncio.run(build_links(topics, judge, Encoder(), max_chars=1000))
    assert graph["quality"]["requests"] >= 2
    assert len(graph["prerequisites"]) == 2
    assert graph["quality"]["cycles_rejected"] == 1
    assert graph["quality"]["unresolved_requirements"] == 1
    validate_topology(topics, graph["prerequisites"], [])
