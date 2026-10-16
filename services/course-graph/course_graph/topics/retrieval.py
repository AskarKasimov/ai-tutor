"""Course-local hybrid retrieval: each requirement searches all provided skills."""
import os
import re
import threading
from functools import lru_cache

import numpy as np


class SkillEncoder:
    def __init__(self, model_name=None):
        from sentence_transformers import SentenceTransformer
        import torch
        torch.set_num_threads(max(1, int(os.environ.get("COURSE_GRAPH_EMBEDDING_THREADS", "4"))))
        self.model_name = model_name or os.environ.get("COURSE_GRAPH_EMBEDDING_MODEL", "intfloat/multilingual-e5-small")
        self.model = SentenceTransformer(self.model_name, device="cpu", trust_remote_code=False)
        self.lock = threading.Lock()
        self.cache = {}

    def encode(self, texts, query=False):
        prefix = "query: " if query else "passage: "
        with self.lock:
            keys = [prefix+t for t in texts]
            missing = list(dict.fromkeys(k for k in keys if k not in self.cache))
            if missing:
                vectors = self.model.encode(missing, batch_size=32, normalize_embeddings=True,
                                            show_progress_bar=False)
                self.cache.update(zip(missing, vectors))
            result = np.asarray([self.cache[k] for k in keys])
            if len(self.cache) > 20000:
                self.cache.clear()
            return result


@lru_cache(maxsize=1)
def get_encoder():
    return SkillEncoder()


def tokens(text):
    return set(re.findall(r"[\w]+", text.casefold().replace("ё", "е")))


class CandidateIndex:
    def __init__(self, topics, encoder):
        self.encoder = encoder
        self.entries = [(t.id, skill, t.title) for t in topics for skill in t.provides]
        self.vectors = encoder.encode([f"{title}. {skill}" for _, skill, title in self.entries]) if self.entries else None

    def search_all(self, topics, per_requirement=5):
        queries = [(t.id, r) for t in topics for r in t.requires]
        if not queries or not self.entries:
            return {}
        vectors = self.encoder.encode([r for _, r in queries], query=True)
        result = {}
        # Score one query at a time, not an N x N dense matrix in memory.
        for (target_id, requirement), vector in zip(queries, vectors):
            scores = np.asarray(self.vectors) @ vector
            valid = [i for i, (topic_id, _, _) in enumerate(self.entries) if topic_id != target_id]
            semantic = sorted(valid, key=lambda i: (-float(scores[i]), i))
            query_tokens = tokens(requirement)
            lexical = sorted(valid, key=lambda i: (-len(query_tokens & tokens(self.entries[i][1]+" "+self.entries[i][2])), i))
            # Reciprocal-rank fusion. Keep lexical entries only when terms match.
            fused = {}
            for ranking in (semantic, [i for i in lexical if query_tokens & tokens(self.entries[i][1]+" "+self.entries[i][2])]):
                for rank, i in enumerate(ranking[:max(30, per_requirement*4)], 1):
                    fused[i] = fused.get(i, 0) + 1/(60+rank)
            selected, seen = [], set()
            for i in sorted(fused, key=lambda i: (-fused[i], i)):
                topic_id, skill, _ = self.entries[i]
                if topic_id in seen:
                    continue
                selected.append({"topic_id": topic_id, "skill": skill,
                                 "similarity": round(float(scores[i]), 5), "rank_score": fused[i]})
                seen.add(topic_id)
                if len(selected) >= per_requirement:
                    break
            result[(target_id, requirement)] = selected
        return result
