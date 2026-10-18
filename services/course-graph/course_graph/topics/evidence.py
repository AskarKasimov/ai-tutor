"""Evidence excerpts shown to the model and the check of its quotes."""
import re

from course_graph.topics.retrieval import tokens


def evidence_for(topic, query, limit, encoder=None):
    terms = tokens(query)
    windows = [(s, s.text[start:start+800]) for s in topic.sources for start in range(0, len(s.text), 650)]
    if encoder and windows:
        vectors = encoder.encode([text for _, text in windows])
        query_vector = encoder.encode([query], query=True)[0]
        scores = vectors @ query_vector
        ranked = [windows[i] for i in sorted(range(len(windows)), key=lambda i: (-float(scores[i]), i))]
    else:
        ranked = sorted(windows, key=lambda item: (-len(terms & tokens(item[1])), item[0].material_id, item[0].start))
    evidence, remaining = [], limit
    for source, window in ranked:
        if remaining <= 0:
            break
        # Visible excerpt is explicit; evidence is checked against this excerpt.
        text = window[:remaining]
        evidence.append({"material_id": source.material_id, "block_id": source.block_id, "text": text})
        remaining -= len(text)
    return evidence


def valid_evidence(evidence, excerpts):
    if not excerpts or not evidence or not evidence.quote or not evidence.quote.strip():
        return False
    raw_quote = evidence.quote.strip()
    words = raw_quote.split()
    if not words:
        return False

    pattern = r"\s+".join(re.escape(word) for word in words)
    for source in excerpts:
        if evidence.material_id == source["material_id"] and evidence.block_id == source["block_id"]:
            match = re.search(pattern, source["text"])
            if match:
                evidence.quote = match.group()
                return True
    return False
