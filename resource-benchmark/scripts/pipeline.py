"""Mikhail's pipeline end to end over the admin API, as the admin panel does it.

  test course -> upload PDFs (one multipart request with the items manifest)
  -> backend background jobs per material: OCR -> translation -> cleaning/topics
  -> POST graph/build -> course-graph: topics, synonyms, prerequisites, attach,
  questions -> graph stored in PostgreSQL

Every 2 s it reads GET .../materials and GET .../graph and logs each change of a
material stage or of the build stage, so the stages can be timed afterwards.

  python pipeline.py --out run/pipeline.jsonl "Week 2 Lecture.pdf" ...

Week and kind come from the file name (Week N, Lecture/Lab/Tutorial).
"""
import sys as _sys
_sys.stdout.reconfigure(encoding="utf-8")
import argparse
import json
import os
import re
import time
import uuid

from diag import Client, Log, multipart  # noqa: F401  (same HTTP client and log)


def meta(name):
    week = re.search(r"week\s*0*(\d+)|^(\d+)\s|lecture\s*(\d+)|tutorial\s*0*(\d+)|lab\s*(\d+)", name, re.I)
    number = next((int(g) for g in (week.groups() if week else []) if g), 1)
    lower = name.lower()
    kind = "lab" if "lab" in lower else "tutorial" if "tutorial" in lower else "lecture"
    return number, kind


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base", default="https://localhost:8443/api/v1")
    parser.add_argument("--out", required=True)
    parser.add_argument("--name", default="BENCH конвейер")
    parser.add_argument("--timeout", type=float, default=6 * 3600)
    parser.add_argument("pdfs", nargs="+")
    args = parser.parse_args()
    log = Log(args.out)
    admin = Client(args.base, log, "admin", admin=True)
    admin.call("teacher_login", "POST", "/auth/teacher")
    subject = admin.call("create_subject", "POST", "/admin/subjects",
                         {"name": f"{args.name} {time.strftime('%m%d%H%M%S')}"})["id"]
    items, parts = [], []
    boundary = uuid.uuid4().hex
    for path in args.pdfs:
        week, kind = meta(os.path.basename(path))
        items.append({"client_id": uuid.uuid4().hex, "week": week, "kind": kind})
        with open(path, "rb") as f:
            parts.append((os.path.basename(path), f.read()))
    body = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"items\"\r\n\r\n"
            + json.dumps(items) + "\r\n").encode()
    for name, content in parts:
        body += (f"--{boundary}\r\nContent-Disposition: form-data; name=\"file\"; filename=\"{name}\"\r\n"
                 "Content-Type: application/pdf\r\n\r\n").encode() + content + b"\r\n"
    body += f"--{boundary}--\r\n".encode()
    upload = admin.call("upload", "POST", f"/admin/subjects/{subject}/materials", body,
                        ctype=f"multipart/form-data; boundary={boundary}")
    log.write(ts=round(time.time(), 3), session="pipeline", op="uploaded", subject=subject,
              files=[{"name": n, "bytes": len(c), **i} for (n, c), i in zip(parts, items)],
              result={k: (len(v) if isinstance(v, list) else v) for k, v in (upload or {}).items()})

    started, seen, build_requested, build_seen = time.time(), {}, None, None
    while time.time() - started < args.timeout:
        materials = admin.call("poll_materials", "GET", f"/admin/subjects/{subject}/materials")
        for m in materials:
            processing = m.get("processing") or {}
            cleaning = m.get("cleaning") or {}
            state = (m.get("status"), processing.get("stage"), processing.get("status"), cleaning.get("status"))
            if seen.get(m["id"]) != state:
                seen[m["id"]] = state
                log.write(ts=round(time.time(), 3), session="pipeline", op="material_state", material=m["id"],
                          filename=m.get("filename"), status=state[0], stage=state[1], processing=state[2],
                          cleaning=state[3], attempts=processing.get("attempts"), chars=m.get("char_count"),
                          error=m.get("error_message"))
        graph = admin.call("poll_graph", "GET", f"/admin/subjects/{subject}/graph")
        progress = graph.get("progress") or {}
        state = (graph.get("status"), progress.get("stage"), progress.get("done"), progress.get("total"),
                 graph.get("inputs_ready"))
        if build_seen != state:
            build_seen = state
            log.write(ts=round(time.time(), 3), session="pipeline", op="graph_state", status=state[0],
                      stage=state[1], done=state[2], total=state[3], inputs_ready=state[4],
                      attempts=graph.get("attempts"), error=graph.get("error_message"))
        if build_requested is None and graph.get("inputs_ready"):
            admin.call("graph_build", "POST", f"/admin/subjects/{subject}/graph/build", {})
            build_requested = time.time()
        failed = all((m.get("processing") or {}).get("status") == "failed" for m in materials)
        if build_requested and graph.get("status") in ("ready", "failed") and time.time() - build_requested > 5:
            break
        if failed:
            break
        time.sleep(2)
    final = admin.call("graph_final", "GET", f"/admin/subjects/{subject}/graph")
    g = final.get("graph") or {}
    summary = {"subject": subject, "files": len(parts), "status": final.get("status"),
               "error": final.get("error_message"), "attempts": final.get("attempts"),
               "topics": len(g.get("nodes") or g.get("topics") or []),
               "edges": len(g.get("edges") or g.get("prerequisites") or []),
               "seconds": round(time.time() - started, 1)}
    log.write(ts=round(time.time(), 3), session="pipeline", op="summary", **summary)
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
