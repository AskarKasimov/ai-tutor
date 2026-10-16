"""Project v1.5 localized competencies and diagnostic groups into Fedor's Graph DTO.

Keeps its rich source dataset in this handoff branch. Writes only the strict Graph
payload to the target repository; no localization metadata is added to DTO nodes.
"""
from __future__ import annotations

import argparse
import json
from pathlib import Path

import networkx as nx
import yaml
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parent
SOURCE = ROOT / 'GIC_RU_translation_v1.5/ai_translation_ru_v5/course_graph.ru-draft.v1.5.json'


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repo', type=Path, required=True, help='Fedor contract repository checkout')
    parser.add_argument('--output', type=Path, help='defaults to api/fixtures/intro-to-ai.ru.graph.json')
    args = parser.parse_args()
    repo = args.repo.resolve()
    data = json.loads(SOURCE.read_text(encoding='utf-8'))
    available = {t['id']: t for t in data['topics'] if t['material_status'] == 'available'}
    groups = {g['id']: g for g in data['diagnostic_groups']}
    assert len(available) == 132
    assert len(groups) == 20
    assert all(t.get('full_text_ru') for t in available.values())

    dag = nx.DiGraph()
    dag.add_nodes_from(available)
    original_edges = set()
    for e in data['prerequisites']:
        edge = (e['source'], e['target'])
        assert edge[0] in available and edge[1] in available
        original_edges.add(edge)
        dag.add_edge(*edge)
    assert dag.number_of_edges() == len(original_edges) == 145
    assert nx.is_directed_acyclic_graph(dag)
    terminal = {n for n in dag if dag.out_degree(n) == 0}
    assignments = [t for g in groups.values() for t in g['terminal_topic_ids']]
    assert len(assignments) == len(set(assignments)) == len(terminal) == 69
    assert set(assignments) == terminal

    nodes: dict[str, dict] = {}
    for tid, topic in available.items():
        nodes[tid] = {
            'id': tid,
            'title': topic['title'],
            'content': topic['full_text_ru'],
            'prerequisites': sorted(dag.predecessors(tid)),
            'outcomes': topic['provides'],
            'questions': [topic['question']],
        }

    group_edges = set()
    for gid, group in groups.items():
        targets = list(group['terminal_topic_ids'])
        assert targets
        group_edges.update((target, gid) for target in targets)
        criteria = [item['criterion'] for item in group['rubric_draft'] if item.get('criterion')]
        nodes[gid] = {
            'id': gid,
            'title': group['title'],
            'content': group['composite_prompt_draft'],
            'prerequisites': sorted(targets),
            'outcomes': criteria,
            'questions': [group['composite_prompt_draft']],
        }

    graph = nx.DiGraph()
    graph.add_nodes_from(nodes)
    graph.add_edges_from(original_edges | group_edges)
    assert len(nodes) == 152
    assert len(group_edges) == 69
    assert graph.number_of_edges() == 214
    assert nx.is_directed_acyclic_graph(graph)
    assert {n for n in graph if graph.out_degree(n) == 0} == set(groups)
    ordered = list(nx.lexicographical_topological_sort(graph))
    positions = {node_id: i for i, node_id in enumerate(ordered)}
    assert all(positions[a] < positions[b] for a, b in graph.edges)
    payload = {'nodes': [nodes[node_id] for node_id in ordered]}

    contract = yaml.safe_load((repo / 'api/course-graph.yaml').read_text(encoding='utf-8'))
    internal_node = contract['components']['schemas']['Node']
    internal_graph = {'type': 'object', 'additionalProperties': False,
                      'properties': {'nodes': {'type': 'array', 'items': internal_node}},
                      'required': ['nodes']}
    Draft202012Validator(internal_graph).validate(payload)
    for name in ['openapi.yaml']:
        public = yaml.safe_load((repo / 'api' / name).read_text(encoding='utf-8'))
        schema = public['components']['schemas']['geek.GraphNode']
        for node in payload['nodes']:
            Draft202012Validator(schema).validate(node)
    known = set(nodes)
    for node in payload['nodes']:
        assert len(node) == 6
        assert all(p in known for p in node['prerequisites'])
        assert all(isinstance(x, str) for field in ['outcomes', 'questions'] for x in node[field])
    assert sum(1 for n in payload['nodes'] if n['id'].startswith('AI-')) == 132
    assert sum(1 for n in payload['nodes'] if n['id'].startswith('DG-')) == 20

    output = args.output.resolve() if args.output else repo / 'api/fixtures/intro-to-ai.ru.graph.json'
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(payload, ensure_ascii=False, indent=2) + '\n', encoding='utf-8')
    print(f'Wrote {output}: {len(nodes)} nodes, {len(original_edges)} topic edges, {len(group_edges)} group links.')
    print('PASS: internal Graph and public geek.GraphNode schemas; 0 cycles; topological order.')
    print('NOTE: topic edges are preserved; group prerequisite links point from each terminal topic to its DG node.')


if __name__ == '__main__':
    main()
