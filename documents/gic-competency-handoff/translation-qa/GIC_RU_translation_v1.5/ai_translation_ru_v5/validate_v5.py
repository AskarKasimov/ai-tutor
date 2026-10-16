"""Validate Russian localizations while preserving the canonical competency DAG."""
from __future__ import annotations

from collections import Counter
from copy import deepcopy
import json
from pathlib import Path
import re

import jsonschema
import networkx as nx

ROOT = Path(__file__).resolve().parent
BASE_PATH = ROOT / 'baseline_course_graph.v1.4.json'
CURRENT_PATH = ROOT / 'course_graph.ru-draft.v1.5.json'
SCHEMA_PATH = ROOT / 'course_graph.ru-draft.schema.json'
PATCH_PATH = ROOT / 'translation_patch_batch5.jsonl'
MARKER = re.compile(r'(?m)^\[D\d+ \| [^\n]+\]')


def validate() -> None:
    old = json.loads(BASE_PATH.read_text(encoding='utf-8'))
    new = json.loads(CURRENT_PATH.read_text(encoding='utf-8'))
    schema = json.loads(SCHEMA_PATH.read_text(encoding='utf-8'))
    jsonschema.validate(new, schema)

    old_by_id = {x['id']: x for x in old['topics']}
    new_by_id = {x['id']: x for x in new['topics']}
    assert len(old_by_id) == len(new_by_id) == 137
    assert list(old_by_id) == list(new_by_id), 'Topic order/IDs changed'
    assert old['prerequisites'] == new['prerequisites']
    assert old['diagnostic_groups'] == new['diagnostic_groups']
    assert all(old[k] == new[k] for k in old if k != 'topics')

    statuses = Counter(x['translation_status'] for x in new['topics'])
    assert statuses == {'translated_from_extracted_text': 132, 'no_source': 5}, statuses
    changed = []
    for tid, topic in new_by_id.items():
        previous = old_by_id[tid]
        en = topic['full_text'] or ''
        ru = topic['full_text_ru'] or ''
        if topic['material_status'] == 'available':
            assert en and ru, tid
            assert MARKER.findall(en) == MARKER.findall(ru), f'Source markers changed: {tid}'
            assert not re.search(r'@@[A-Z_]+@@', ru), f'Unresolved translation placeholder: {tid}'
            assert '\x00' not in ru
            assert ru != en, f'Untranslated node: {tid}'
        else:
            assert topic['translation_status'] == 'no_source', tid
        diff_keys = {k for k in topic if topic[k] != previous[k]}
        assert diff_keys <= {'full_text_ru', 'translation_status', 'translation_notes'}, (tid, diff_keys)
        if diff_keys:
            changed.append(tid)
    assert len(changed) == 22, changed

    patch = [json.loads(line) for line in PATCH_PATH.read_text(encoding='utf-8').splitlines()]
    assert len(patch) == 22
    assert {x['id'] for x in patch} == set(changed)
    reconstructed = deepcopy(old)
    index = {x['id']: x for x in reconstructed['topics']}
    for item in patch:
        tid = item.pop('id')
        assert set(item) == {'full_text_ru', 'translation_status', 'translation_notes'}
        index[tid].update(item)
    assert reconstructed == new, 'Patch does not reconstruct v1.5 exactly'

    graph = nx.DiGraph()
    graph.add_nodes_from(t['id'] for t in new['topics'] if t['material_status'] == 'available')
    for edge in new['prerequisites']:
        assert edge['source'] in graph and edge['target'] in graph
        assert edge['source'] != edge['target']
        graph.add_edge(edge['source'], edge['target'])
    assert nx.is_directed_acyclic_graph(graph)
    assert graph.number_of_nodes() == 132 and graph.number_of_edges() == 145
    assert len(new['diagnostic_groups']) == 20
    terminals = {n for n in graph if graph.out_degree(n) == 0}
    listed = [id for group in new['diagnostic_groups'] for id in group['terminal_topic_ids']]
    assert len(set(listed)) == len(listed) == 69
    assert set(listed) == terminals

    source_chars = sum(len(t['full_text']) for t in new['topics'])
    ru_chars = sum(len(t['full_text_ru'] or '') for t in new['topics'])
    print('PASS: Schema, translation markers, unchanged graph, JSON patch round-trip, 132 translated active topics')
    print(f'PASS: {len(new["topics"])} topics, {graph.number_of_edges()} prerequisite edges, '
          f'{len(new["diagnostic_groups"])} diagnostic groups, 0 cycles')
    print(f'English characters: {source_chars:,}; Russian characters: {ru_chars:,}; changed topics: {len(changed)}')


if __name__ == '__main__':
    validate()
