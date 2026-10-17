"""Audit saved end-to-end artifacts independently of the service's quality flag."""
import argparse
import json
import re
from pathlib import Path


def audit(materials, graph):
    errors, warnings = [], []
    by_material = {m["id"]: m for m in materials}
    topics = {t["id"]: t for t in graph["topics"]}
    if len(topics) != len(graph["topics"]):
        errors.append("Duplicate topic IDs")
    covered = {mid: bytearray(len(m["parsed_text"])) for mid, m in by_material.items()}
    source_lookup = {}
    for topic in topics.values():
        if topic["text"] != "\n\n".join(s["text"] for s in topic["sources"]):
            errors.append(f"Topic text differs from sources: {topic['id']}")
        if not topic["question"].strip() or not topic["provides"]:
            errors.append(f"Missing question/provides: {topic['id']}")
        for source in topic["sources"]:
            mid, start, end = source["material_id"], source["start"], source["end"]
            original = by_material.get(mid, {}).get("parsed_text", "")
            if not 0 <= start < end <= len(original) or original[start:end] != source["text"]:
                errors.append(f"Source text/coordinates mismatch: {topic['id']}")
                continue
            covered[mid][start:end] = b"\1"*(end-start)
            source_lookup[(topic["id"], mid, source["block_id"])] = source["text"]
    per_material = []
    compact = lambda value: re.sub(r"\s+", "", value)
    for mid, material in by_material.items():
        cleaning = material.get("cleaning") or {}
        if material["status"] != "parsed" or cleaning.get("status") != "ready":
            errors.append(f"Material not ready: {material['filename']}")
        source_text = material["parsed_text"]
        assigned = "".join(c for i,c in enumerate(source_text) if covered[mid][i])
        cleaned = cleaning.get("cleaned_text", "")
        if compact(assigned) != compact(cleaned):
            errors.append(f"Cleaned text and assigned source coverage differ: {material['filename']}")
        spans = re.findall(r"```.*?```|~~~.*?~~~|\$\$.*?\$\$|\\\[.*?\\\]|(?<!\$)\$[^$\n]+\$(?!\$)", source_text, re.S)
        if any(span not in cleaned for span in spans):
            errors.append(f"Cleaning lost code/formula: {material['filename']}")
        per_material.append({"filename": material["filename"], "topics": sum(any(s["material_id"]==mid for s in t["sources"]) for t in topics.values()),
                             "parsed_chars":len(source_text), "cleaned_chars":len(cleaned),
                             "retained_fraction":round(len(cleaned)/max(1,len(source_text)),4)})
    cross_material = 0
    for edge in graph["prerequisites"]:
        if edge["source"] not in topics or edge["target"] not in topics or edge["source"] == edge["target"]:
            errors.append("Invalid prerequisite endpoint")
            continue
        for topic_id, field in ((edge["source"],"prerequisite_evidence"),(edge["target"],"target_evidence")):
            evidence = edge[field]
            source = source_lookup.get((topic_id,evidence["material_id"],evidence["block_id"]), "")
            if not evidence["quote"].strip() or evidence["quote"] not in source:
                errors.append(f"Unsupported quotation: {edge['source']} -> {edge['target']}")
        if edge["target_evidence"]["material_id"] != edge["prerequisite_evidence"]["material_id"]:
            cross_material += 1
    for field in ("prerequisites", "hierarchy"):
        adjacency = {tid: [] for tid in topics}
        for edge in graph[field]:
            if edge["source"] not in topics or edge["target"] not in topics:
                errors.append("Unknown topic in " + field)
                continue
            adjacency[edge["source"]].append(edge["target"])
        visiting, done = set(), set()
        def visit(node):
            if node in visiting:
                raise ValueError("Cycle in " + field)
            if node in done:
                return
            visiting.add(node)
            for child in adjacency[node]:
                visit(child)
            visiting.remove(node)
            done.add(node)
        try:
            for node in topics:
                visit(node)
        except ValueError as error:
            errors.append(str(error))
    expected = {(t["id"],r) for t in topics.values() for r in t["requires"]}
    decisions = {(d["target_id"],d["requirement"]) for d in graph["requirements"]}
    if decisions != expected or len(decisions) != len(graph["requirements"]):
        errors.append("Incomplete or duplicate requirement decisions")
    unresolved = sum(not d["prerequisite_ids"] for d in graph["requirements"])
    if unresolved:
        warnings.append(f"{unresolved} requirements have no confirmed provider; review recall and material coverage")
    return {"structural_checks_passed":not errors, "errors":errors, "warnings":warnings,
            "materials":per_material, "topics":len(topics), "edges":len(graph["prerequisites"]),
            "cross_material_edges":cross_material, "unresolved_requirements":unresolved,
            "semantic_quality":"requires_manual_review"}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("directory")
    args = parser.parse_args()
    directory = Path(args.directory)
    result = audit(json.loads((directory/"materials.json").read_text()), json.loads((directory/"snapshot.json").read_text())["graph"])
    (directory/"independent_audit.json").write_text(json.dumps(result,ensure_ascii=False,indent=2))
    print(json.dumps(result,ensure_ascii=False,indent=2))
    raise SystemExit(0 if result["structural_checks_passed"] else 1)
