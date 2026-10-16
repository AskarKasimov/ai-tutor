"""Reference GIC baseline: evidence-driven adaptive diagnosis, no hidden LLM grading.

It is a portable algorithm sketch, NOT a production backend or a validated assessment.
Requires only Python 3.11+ standard library.
"""
from __future__ import annotations

from collections import defaultdict, deque
from dataclasses import dataclass, field
from datetime import datetime, timezone
from enum import StrEnum
from pathlib import Path
from typing import Any, Literal
import json
import uuid


class Outcome(StrEnum):
    PASS = "pass"
    GAP = "gap"
    UNCERTAIN = "uncertain"


class KnowledgeStatus(StrEnum):
    UNKNOWN = "unknown"
    MASTERED_OBSERVED = "mastered_observed"
    SUSPECTED_GAP = "suspected_gap"
    INSUFFICIENT_EVIDENCE = "insufficient_evidence"


@dataclass(frozen=True)
class Question:
    topic_id: str
    group_id: str
    question: str
    rubric: list[str]
    reason: Literal["terminal_screening", "prerequisite_drilldown"]


@dataclass(frozen=True)
class Evidence:
    evidence_id: str
    topic_id: str
    group_id: str
    outcome: Outcome
    answer: str
    grader: str
    rationale: str
    created_at: str


@dataclass
class DiagnosticSession:
    session_id: str
    dataset_id: str
    worklist: deque[tuple[str, str, str]]
    topic_status: dict[str, KnowledgeStatus]
    evidence: list[Evidence] = field(default_factory=list)
    asked: set[str] = field(default_factory=set)
    active: Question | None = None


class DiagnosticEngine:
    """Deterministic reference of the safe baseline, no inference of ancestor mastery."""

    def __init__(self, graph: dict[str, Any]):
        self.graph = graph
        self.topics = {t["id"]: t for t in graph["topics"] if t["material_status"] == "available"}
        self.gates = {g["id"]: g for g in graph["diagnostic_groups"]}
        self.parents: dict[str, list[str]] = defaultdict(list)
        self.children: dict[str, list[str]] = defaultdict(list)
        for edge in graph["prerequisites"]:
            source, target = edge["source"], edge["target"]
            if source not in self.topics or target not in self.topics:
                raise ValueError("Active prerequisite references missing teaching material")
            self.parents[target].append(source)
            self.children[source].append(target)
        self.level = self._topological_levels()
        self.sessions: dict[str, DiagnosticSession] = {}

    @classmethod
    def from_file(cls, path: Path) -> DiagnosticEngine:
        return cls(json.loads(path.read_text(encoding="utf-8")))

    def _topological_levels(self) -> dict[str, int]:
        indegree = {x: len(self.parents[x]) for x in self.topics}
        queue = deque(sorted(x for x, v in indegree.items() if v == 0))
        levels = {x: 0 for x in queue}
        count = 0
        while queue:
            node = queue.popleft()
            count += 1
            for child in sorted(self.children[node]):
                levels[child] = max(levels.get(child, 0), levels[node] + 1)
                indegree[child] -= 1
                if indegree[child] == 0:
                    queue.append(child)
        if count != len(self.topics):
            raise ValueError("Prerequisite graph contains a cycle")
        return levels

    def start(self, group_ids: list[str] | None = None) -> DiagnosticSession:
        target_groups = group_ids if group_ids is not None else list(self.gates)
        if len(set(target_groups)) != len(target_groups) or any(x not in self.gates for x in target_groups):
            raise ValueError("Group IDs must be unique and known")
        worklist: deque[tuple[str, str, str]] = deque()
        for gid in target_groups:
            # Baseline asks individual terminal topic questions; composite draft is NOT graded.
            for tid in self.gates[gid]["terminal_topic_ids"]:
                worklist.append((tid, gid, "terminal_screening"))
        session = DiagnosticSession(
            session_id=str(uuid.uuid4()), dataset_id=self.graph["dataset_id"],
            worklist=worklist,
            topic_status={k: KnowledgeStatus.UNKNOWN for k in self.topics},
        )
        self.sessions[session.session_id] = session
        return session

    def next_question(self, session: DiagnosticSession) -> Question | None:
        if session.active is not None:
            return session.active
        while session.worklist:
            tid, gid, reason = session.worklist.popleft()
            if tid in session.asked:
                continue
            t = self.topics[tid]
            session.active = Question(
                topic_id=tid, group_id=gid, question=t["question"], rubric=t["rubric"],
                reason=reason,  # type: ignore[arg-type]
            )
            return session.active
        return None

    def submit(
        self, session: DiagnosticSession, topic_id: str, outcome: Outcome,
        answer: str, grader: str, rationale: str,
    ) -> Evidence:
        q = session.active
        if q is None or q.topic_id != topic_id:
            raise ValueError("Answer does not match the active question")
        if not grader.strip() or not rationale.strip():
            raise ValueError("A grader and grading rationale are required")
        evidence = Evidence(
            evidence_id=str(uuid.uuid4()), topic_id=topic_id, group_id=q.group_id,
            outcome=outcome, answer=answer, grader=grader, rationale=rationale,
            created_at=datetime.now(timezone.utc).isoformat(),
        )
        session.evidence.append(evidence)
        session.asked.add(topic_id)
        session.active = None
        if outcome == Outcome.PASS:
            session.topic_status[topic_id] = KnowledgeStatus.MASTERED_OBSERVED
            # Deliberately do not infer any ancestors.
        elif outcome == Outcome.GAP:
            session.topic_status[topic_id] = KnowledgeStatus.SUSPECTED_GAP
            new = [p for p in self.parents[topic_id] if p not in session.asked]
            new.sort(key=lambda x: (-self.level[x], x))
            for p in reversed(new):
                session.worklist.appendleft((p, q.group_id, "prerequisite_drilldown"))
        else:
            session.topic_status[topic_id] = KnowledgeStatus.INSUFFICIENT_EVIDENCE
        return evidence

    def learning_plan(self, session: DiagnosticSession) -> list[dict[str, str | int]]:
        """Targets = suspected gaps and their unmastered ancestors; unknown != failed."""
        relevant = set()
        stack = [k for k, status in session.topic_status.items()
                 if status == KnowledgeStatus.SUSPECTED_GAP]
        while stack:
            tid = stack.pop()
            if tid in relevant or session.topic_status[tid] == KnowledgeStatus.MASTERED_OBSERVED:
                continue
            relevant.add(tid)
            stack.extend(self.parents[tid])
        return [
            {'topic_id':tid, 'title':self.topics[tid]['title'],
             'status':session.topic_status[tid].value, 'level':self.level[tid]}
            for tid in sorted(relevant, key=lambda x: (self.level[x],x))
        ]

    def summary(self, session: DiagnosticSession) -> dict[str, Any]:
        from collections import Counter
        counts = Counter(x.value for x in session.topic_status.values())
        return {
            'session_id':session.session_id, 'dataset_id':session.dataset_id,
            'screening_finished':self.next_question(session) is None,
            'asked_topics':len(session.asked),
            'counts':dict(counts), 'learning_plan':self.learning_plan(session),
            'note':'Unknown ancestors remain unknown even when a descendant passed.'
        }
