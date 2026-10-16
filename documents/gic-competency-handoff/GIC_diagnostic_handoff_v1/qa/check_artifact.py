from __future__ import annotations
import json
from collections import Counter
from pathlib import Path
import networkx as nx
from jsonschema import Draft202012Validator

ROOT=Path(__file__).resolve().parents[1]
data=json.loads((ROOT/'data/course_graph.v1.json').read_text('utf-8'))
schema=json.loads((ROOT/'schema/course_graph.schema.json').read_text('utf-8'))
Draft202012Validator(schema).validate(data)
ids=[n['id'] for n in data['topics']];assert len(ids)==len(set(ids))==137
T={n['id']:n for n in data['topics']}
material={n['id'] for n in data['topics'] if n['material_status']=='available'}
assert len(material)==132
u=nx.DiGraph();u.add_nodes_from(material)
for e in data['prerequisites']:
 assert e['source'] in material and e['target'] in material
 u.add_edge(e['source'],e['target'])
assert nx.is_directed_acyclic_graph(u)
assert len(data['prerequisites'])==u.number_of_edges()==145
atom={n for n,d in u.in_degree() if d==0};term={n for n,d in u.out_degree() if d==0}
assert len(atom)==15 and len(term)==69
assert all(T[n]['is_atomic']==(n in atom) and T[n]['is_terminal']==(n in term) for n in material)
assert all(not T[n]['is_atomic'] and not T[n]['is_terminal'] for n in T if n not in material)
assert len(data['diagnostic_groups'])==20
assigned=[]
for g in data['diagnostic_groups']:
 assert not g['auto_close_allowed']
 assert all(x in term for x in g['terminal_topic_ids'])
 assigned.extend(g['terminal_topic_ids'])
 closure=set(g['terminal_topic_ids'])
 for x in g['terminal_topic_ids']:closure|=nx.ancestors(u,x)
 assert closure==set(g['closure_topic_ids']),(g['id'],closure^set(g['closure_topic_ids']))
assert Counter(assigned)==Counter(term)
assert len(data['unmapped_requirements'])==80
assert len(data['curriculum_gaps'])==5
for n in material:
 assert T[n]['full_text'].strip() and T[n]['question'].strip()
print('PASS: JSON Schema, IDs, DAG, 145 edges, 132 active / 5 missing,')
print('      15 atomic, 69 terminal, 20 gates, 69 unique assignments, all closures, no auto-close.')
