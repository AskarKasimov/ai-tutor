"""Reproduce baseline limitations without changing original code or data."""
import sys
from pathlib import Path

PACKAGE = Path(__file__).resolve().parent / 'GIC_diagnostic_handoff_v1'
sys.path.insert(0, str(PACKAGE / 'reference'))
from diagnostic_engine import DiagnosticEngine, KnowledgeStatus, Outcome

engine = DiagnosticEngine.from_file(PACKAGE / 'data/course_graph.v1.json')
session = engine.start()
assert session.active is None
engine.summary(session)
assert session.active is not None
print('CONFIRMED: summary activates a question')

question = session.active
engine.submit(session, question.topic_id, 'invalid', 'example', 'expert', 'audit probe')
assert session.evidence[-1].outcome == 'invalid'
assert session.topic_status[question.topic_id] == KnowledgeStatus.INSUFFICIENT_EVIDENCE
print('CONFIRMED: invalid outcome stored as evidence and treated as uncertain')

session = engine.start()
a, b, c = 'AI-01-01', 'AI-01-07', 'AI-01-08'
assert a in engine.parents[b] and b in engine.parents[c]
session.topic_status[b] = KnowledgeStatus.MASTERED_OBSERVED
session.topic_status[c] = KnowledgeStatus.SUSPECTED_GAP
assert a not in {item['topic_id'] for item in engine.learning_plan(session)}
assert session.topic_status[a] == KnowledgeStatus.UNKNOWN
print('CONFIRMED: unknown ancestor behind a mastered topic excluded from gap plan')

# Exercise shared ancestors and deduplication beyond the original five tests.
session = engine.start()
while question := engine.next_question(session):
    engine.submit(session, question.topic_id, Outcome.GAP, 'example', 'expert', 'audit probe')
assert len(session.evidence) == len(session.asked) == 132
plan = engine.learning_plan(session)
position = {item['topic_id']: i for i, item in enumerate(plan)}
assert all(position[e['source']] < position[e['target']] for e in engine.graph['prerequisites'])
print('PASS: all-gap traversal asks all 132 topics once; plan respects all 145 edges')
