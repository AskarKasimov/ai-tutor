"""Offline compatibility proof, not a production importer or grading adapter.

Python 3.11+. Run from any directory; --check verifies committed projection.
Original handoff files remain byte-for-byte unchanged.
"""
import argparse
import hashlib
import json
from pathlib import Path

import networkx as nx
import yaml
from jsonschema import Draft202012Validator

ROOT = Path(__file__).resolve().parent
REPO = ROOT.parents[1]
PACKAGE = ROOT / 'GIC_diagnostic_handoff_v1'


def read_json(path):
    return json.loads(path.read_text(encoding='utf-8'))


def validate_component(value, contract, name):
    schema = {'$ref': f'#/components/schemas/{name}', 'components': contract['components']}
    Draft202012Validator(schema).validate(value)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--check', action='store_true')
    args = parser.parse_args()
    source = PACKAGE / 'data/course_graph.v1.json'
    data = read_json(source)
    digest = hashlib.sha256(source.read_bytes()).hexdigest()
    assert digest == read_json(PACKAGE / 'data/manifest.json')['full_graph_sha256']
    Draft202012Validator(read_json(PACKAGE / 'schema/course_graph.schema.json')).validate(data)
    topics = {t['id']: t for t in data['topics'] if t['material_status'] == 'available'}
    dag = nx.DiGraph()
    dag.add_nodes_from(topics)
    for edge in data['prerequisites']:
        assert edge['source'] in topics and edge['target'] in topics
        dag.add_edge(edge['source'], edge['target'])
    assert dag.number_of_edges() == len(data['prerequisites']) == 145
    order = list(nx.lexicographical_topological_sort(dag))
    projection = {'nodes': [dict(
        id=tid, title=topics[tid]['title'], content=topics[tid]['full_text'],
        prerequisites=sorted(dag.predecessors(tid)),
        outcomes=topics[tid]['provides'], questions=[topics[tid]['question']],
    ) for tid in order]}
    contract = yaml.safe_load((REPO / 'api/course-graph.yaml').read_text(encoding='utf-8'))
    public = yaml.safe_load((REPO / 'api/openapi.yaml').read_text(encoding='utf-8'))
    validate_component(projection, contract, 'Graph')
    # subject_id and built_at belong to runtime; never fabricate them in the fixture.
    for node in projection['nodes']:
        validate_component(node, public, 'geek.GraphNode')
    fixture = read_json(REPO / 'api/fixtures/course-graph.json')
    validate_component(fixture, contract, 'Graph')
    restored_edges = {(p, n['id']) for n in projection['nodes'] for p in n['prerequisites']}
    assert restored_edges == set(dag.edges)
    positions = {tid: i for i, tid in enumerate(order)}
    assert all(positions[a] < positions[b] for a, b in restored_edges)
    assert len(topics) == 132
    assert {t['id'] for t in data['topics']} == {t['id'] for t in read_json(PACKAGE / 'data/topic_provenance.v1.json')['topics']}
    groups = data['diagnostic_groups']
    assert len({g['id'] for g in groups}) == len(groups) == 20
    assert all(not g['auto_close_allowed'] for g in groups)
    path = ROOT / 'course-graph.fedia.v1.json'
    encoded = json.dumps(projection, ensure_ascii=False, indent=2) + '\n'
    if args.check:
        assert path.read_text(encoding='utf-8') == encoded, 'Projection is stale'
    else:
        path.write_text(encoded, encoding='utf-8', newline='\n')
    print(f'PASS: SHA256 {digest}; original schema; provenance IDs; unique DG IDs')
    print('PASS: 132 nodes / 145 edges preserved, prerequisite direction and topological order')
    print('PASS: course-graph Graph and geek.GraphNode schemas; existing 10-node fixture schema')
    print('WARNING: schema compatibility is not Russian-language or pedagogical validation.')
    print('WARNING: metadata stays in the canonical package; projection alone is lossy.')


if __name__ == '__main__':
    main()
