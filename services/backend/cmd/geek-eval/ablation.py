"""Replay frozen Geek grading requests, varying only reasoning and topic text.

Evaluation-only transport; requires the operator's explicit model endpoint.
Run from services/backend after producing the stress-test reports.
"""

import argparse
import copy
import datetime
import json
from pathlib import Path
import random
import time
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--endpoint", required=True)
    parser.add_argument("--out", default="bin/geek-reasoning-ablation.json")
    parser.add_argument("--repeats", type=int, default=2)
    args = parser.parse_args()
    if args.repeats < 1:
        parser.error("repeats must be positive")
    graph = json.loads(Path("../../api/fixtures/intro-to-ai.weeks-1-2.ru.graph.json").read_text(encoding="utf-8"))
    nodes = {n["id"]: n for n in graph["nodes"]}
    selections = {
        "bin/geek-compact-stress.json": [
            "evaluate:08-parrot:1", "evaluate:04-wrong-confident:1",
            "evaluate:13-example-question:1", "evaluate:16-parrot-plus-example:1",
            "evaluate:14-good-oral:1", "evaluate:18-not-entailed-good:1",
            "evaluate:02-good-short:1", "evaluate:02-wrong-plus-injection:1",
        ],
        "bin/geek-compact-followup.json": [
            "evaluate:18-contradictory-followup-dontknow:2",
        ],
    }
    frozen = []
    for source, labels in selections.items():
        report = json.loads(Path(source).read_text(encoding="utf-8"))
        for label in labels:
            call = next(c for c in report["calls"] if c["label"] == label)
            case_id = label.split(":")[1]
            result = next(r for r in report["results"] if r["id"] == case_id)
            frozen.append((source, label, call["request"], nodes[result["node_id"]]["content"]))
    jobs = []
    for repeat in range(1, args.repeats + 1):
        for source, label, request, content in frozen:
            conditions = [(effort, text) for effort in ("low", "medium") for text in (True, False)]
            random.Random(label + str(repeat)).shuffle(conditions)
            for effort, text in conditions:
                body = copy.deepcopy(request)
                body["reasoning_effort"] = effort
                if not text:
                    message = body["messages"][1]
                    if content not in message["content"]:
                        raise ValueError("topic text not found in " + label)
                    message["content"] = message["content"].replace(content, "[Topic text omitted for this experiment.]", 1)
                jobs.append(dict(source=source, label=label, repeat=repeat, reasoning=effort, topic_text=text, request=body))
    report = dict(created_at=datetime.datetime.now(datetime.timezone.utc).isoformat(),
                  method="Frozen requests; teacher guidance, dialogue, system and question retained; only topic content and effort vary.",
                  results=[])
    out = Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    for i, job in enumerate(jobs, 1):
        start = time.monotonic()
        try:
            req = urllib.request.Request(args.endpoint.rstrip("/") + "/chat/completions",
                                         json.dumps(job["request"], ensure_ascii=False).encode("utf-8"),
                                         {"Content-Type": "application/json"})
            with urllib.request.urlopen(req, timeout=90) as response:
                job["response"] = json.load(response)
            choice = job["response"]["choices"][0]
            if choice.get("finish_reason") == "length":
                raise ValueError("completion truncated at token limit")
            job["assessment"] = json.loads(choice["message"]["content"])
        except Exception as exc:
            job["error"] = str(exc)
        job["seconds"] = time.monotonic() - start
        report["results"].append(job)
        out.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
        a = job.get("assessment", {})
        print(i, "/", len(jobs), job["label"], job["reasoning"], "topic=" + str(job["topic_text"]),
              round(job["seconds"], 2), a.get("verdict"), a.get("criteria"), job.get("error", ""), flush=True)


if __name__ == "__main__":
    main()
