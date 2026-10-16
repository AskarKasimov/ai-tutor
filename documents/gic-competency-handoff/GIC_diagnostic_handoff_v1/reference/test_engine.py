"""Run with: python -m unittest discover -s reference -p 'test_*.py' -v"""
from __future__ import annotations
import json
import unittest
from pathlib import Path
from diagnostic_engine import DiagnosticEngine, KnowledgeStatus, Outcome

DATA = Path(__file__).resolve().parents[1] / "data/course_graph.v1.json"


class GicEngineTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.engine = DiagnosticEngine.from_file(DATA)

    def test_structure(self):
        e=self.engine
        self.assertEqual(len(e.topics),132)
        self.assertEqual(sum(len(v) for v in e.parents.values()),145)
        self.assertEqual(sum(len(g['terminal_topic_ids']) for g in e.gates.values()),69)
        self.assertEqual(max(e.level.values()),7)
        self.assertEqual(sum(not e.parents[n] for n in e.topics),15)

    def test_every_terminal_screened_once(self):
        e=self.engine;s=e.start()
        while q:=e.next_question(s):
            e.submit(s,q.topic_id,Outcome.PASS,'example','expert','checked against criteria')
        self.assertEqual(len(s.asked),69)
        self.assertEqual(sum(v==KnowledgeStatus.MASTERED_OBSERVED for v in s.topic_status.values()),69)
        self.assertEqual(sum(v==KnowledgeStatus.UNKNOWN for v in s.topic_status.values()),63)
        self.assertEqual(e.learning_plan(s),[])

    def test_gap_drills_into_direct_prerequisites(self):
        e=self.engine;s=e.start(['DG-08'])
        q=e.next_question(s)
        while q and not e.parents[q.topic_id]:
            e.submit(s,q.topic_id,Outcome.PASS,'ok','expert','checked')
            q=e.next_question(s)
        self.assertIsNotNone(q)
        self.assertTrue(e.parents[q.topic_id])
        e.submit(s,q.topic_id,Outcome.GAP,'incorrect','expert','wrong transition')
        nxt=e.next_question(s)
        self.assertIn(nxt.topic_id,e.parents[q.topic_id])
        self.assertEqual(s.topic_status[q.topic_id],KnowledgeStatus.SUSPECTED_GAP)
        self.assertEqual(s.topic_status[nxt.topic_id],KnowledgeStatus.UNKNOWN)
        plan=e.learning_plan(s)
        self.assertTrue(any(x['topic_id']==q.topic_id for x in plan))
        levels=[x['level'] for x in plan]
        self.assertEqual(levels,sorted(levels))

    def test_uncertain_is_not_failure(self):
        e=self.engine;s=e.start(['DG-01']);q=e.next_question(s)
        e.submit(s,q.topic_id,Outcome.UNCERTAIN,'?','expert','insufficient signal')
        self.assertEqual(s.topic_status[q.topic_id],KnowledgeStatus.INSUFFICIENT_EVIDENCE)
        self.assertEqual(e.learning_plan(s),[])

    def test_cannot_grade_wrong_item(self):
        e=self.engine;s=e.start(['DG-01']);e.next_question(s)
        with self.assertRaises(ValueError):
            e.submit(s,'AI-04-17',Outcome.PASS,'?','expert','checked')


if __name__ == '__main__':
    unittest.main()
