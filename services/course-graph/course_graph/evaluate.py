"""Reproducible live evaluation; all generative calls use the backend gRPC pool.

python -m course_graph.evaluate --dataset topics_reviewed_v2.json --output /tmp/eval
Only source-backed cards are imported; RPD-only topics are excluded.
"""
import argparse
import asyncio
import json
import time
from collections import defaultdict
from pathlib import Path

from course_graph.llm.client import ModelClient
from course_graph.topics.extract import ExtractionSettings, extract_topics
from course_graph.topics.graph import build_links
from course_graph.topics.models import Source, Topic
from course_graph.topics.retrieval import CandidateIndex, get_encoder


def import_dataset(path):
    data = json.loads(Path(path).read_text())
    topics, materials = [], defaultdict(dict)
    for card in data["topics"]:
        fragments = card.get("clean_core_fragments", [])
        sources = []
        for block_id, f in enumerate(fragments):
            text = f.get("text", "")
            if not text.strip():
                continue
            material_id = f["document_id"]
            # Dataset lacks character offsets. Each topic is an evaluation document;
            # the original document/page references remain visible in material_id/page.
            sources.append(Source(material_id=f"{material_id}:{card['id']}", block_id=block_id,
                                  start=0, end=len(text), text=text, page=f.get("page")))
            materials[material_id][f.get("page", block_id)] = text
        if not sources:
            continue
        topics.append(Topic(id=card["id"], title=card["title"], requires=card["requires"],
                            provides=card["provides"], question=card["question"],
                            text="\n\n".join(s.text for s in sources), sources=sources))
    return topics, {id: "\n\n".join(f"## Слайд {page}\n\n{text}" for page, text in sorted(pages.items()))
                    for id, pages in materials.items()}


async def evaluate(args):
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    topics, materials = import_dataset(args.dataset)
    summary = {"dataset_topics_with_sources": len(topics), "materials": len(materials),
               "extraction": [], "semantic_quality": "requires_independent_human_labels"}
    client = ModelClient()
    trace_count = 0
    async def complete(system, payload):
        nonlocal trace_count
        trace_count += 1
        number = trace_count
        raw = await client.complete(system, payload)
        (output/f"trace_{number:03d}.json").write_text(json.dumps(
            {"system": system, "input": json.loads(payload), "output": raw}, ensure_ascii=False, indent=2))
        return raw
    try:
        for material_id, text in list(materials.items())[:args.materials]:
            text = text[:args.material_chars]
            for size in args.sweep:
                started = time.monotonic()
                try:
                    result = await extract_topics(text, complete, material_id,
                                                  settings=ExtractionSettings(max_chars=size))
                    (output/f"extract_{material_id}_{size}.json").write_text(json.dumps(result, ensure_ascii=False, indent=2))
                    summary["extraction"].append({"material_id": material_id, "max_chars": size,
                        "seconds": round(time.monotonic()-started, 2), **result["report"]["quality"],
                        "requests": result["report"]["requests"], "adaptive_splits": result["report"]["adaptive_splits"],
                        "requires": sum(len(t["requires"]) for t in result["report"]["topics"])})
                except Exception as exc:
                    summary["extraction"].append({"material_id": material_id, "max_chars": size, "error": str(exc)})
                (output/"summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))
        started = time.monotonic()
        encoder = await asyncio.to_thread(get_encoder)
        index = await asyncio.to_thread(CandidateIndex, topics, encoder)
        candidates = await asyncio.to_thread(index.search_all, topics, args.k)
        summary["retrieval"] = {"seconds": round(time.monotonic()-started, 2),
            "requirements": len(candidates), "candidate_pairs": sum(len(c) for c in candidates.values()),
            "full_directed_pairs": len(topics)*(len(topics)-1), "k": args.k}
        (output/"candidates.json").write_text(json.dumps([
            {"target_id": key[0], "requirement": key[1], "candidates": value} for key, value in candidates.items()], ensure_ascii=False, indent=2))
        # All source-backed topics remain candidate providers. Only the selected
        # targets keep requirements, so distant-topic retrieval is exercised.
        targets = args.target_ids.split(",") if args.target_ids else [t.id for t in sorted(topics, key=lambda t: -len(t.requires))[:args.targets]]
        known_ids = {t.id for t in topics}
        if set(targets)-known_ids:
            raise ValueError("Целевые темы не имеют основного текста: " + ", ".join(sorted(set(targets)-known_ids)))
        subset = [t.model_copy(update={"requires": t.requires if t.id in targets else []}) for t in topics]
        started = time.monotonic()
        graph = await build_links(subset, complete, encoder, per_requirement=args.k)
        graph["evaluation"] = {"partial_graph": True, "target_ids": targets,
                               "candidate_catalog_topics": len(topics)}
        (output/"graph.json").write_text(json.dumps(graph, ensure_ascii=False, indent=2))
        summary["links"] = {"seconds": round(time.monotonic()-started, 2), "target_ids": targets, "partial_graph": True,
                            "accepted_edges": len(graph["prerequisites"]), **graph["quality"]}
        (output/"summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2))
        print(json.dumps(summary, ensure_ascii=False, indent=2), flush=True)
    finally:
        await client.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--dataset", required=True)
    parser.add_argument("--output", required=True)
    parser.add_argument("--materials", type=int, default=2)
    parser.add_argument("--material-chars", type=int, default=18000)
    parser.add_argument("--sweep", type=lambda s: [int(x) for x in s.split(",")], default=[4000, 6000])
    parser.add_argument("--targets", type=int, default=8)
    parser.add_argument("--target-ids", default="")
    parser.add_argument("--k", type=int, default=5)
    asyncio.run(evaluate(parser.parse_args()))
