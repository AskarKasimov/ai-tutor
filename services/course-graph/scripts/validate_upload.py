"""Local live smoke test: HTTP upload -> S3 -> both workers -> graph snapshot.

Uploads two PDF files given on the command line. Creates a separate reviewable
course; it does not replace existing course data. TLS verification
is disabled only for the explicitly selected localhost development endpoint.
"""
import argparse
import http.cookiejar
import json
import ssl
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path


def upload_body(paths, boundary):
    """Multipart body of the materials upload: the manifest, then the files in its order."""
    items = [{"client_id": f"file-{i}", "week": i, "kind": "lecture"} for i in range(1, len(paths) + 1)]
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="items"\r\n\r\n'
            f'{json.dumps(items)}\r\n').encode()
    for path in paths:
        body += (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{Path(path).name}"\r\n'
                 'Content-Type: application/pdf\r\n\r\n').encode() + Path(path).read_bytes() + b"\r\n"
    return body + f"--{boundary}--\r\n".encode()


def needs_build(snapshot):
    """A build is requested when the materials are ready and no build is running."""
    return snapshot["inputs_ready"] and snapshot["status"] not in {"pending", "processing"}


def main(args):
    if urllib.parse.urlsplit(args.url).hostname not in {"localhost", "127.0.0.1"}:
        raise ValueError("This smoke runner is restricted to localhost")
    opener = urllib.request.build_opener(
        urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
        urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))

    def request(method, path, data=None, content_type="application/json"):
        if isinstance(data, dict):
            data = json.dumps(data, ensure_ascii=False).encode()
        req = urllib.request.Request(args.url+path, data=data, method=method,
                                     headers={"Content-Type": content_type, "X-AI-Tutor-Portal": "admin"})
        with opener.open(req, timeout=180) as response:
            return json.load(response)

    request("POST", "/auth/teacher")
    course = {"id": args.subject_id} if args.subject_id else request("POST", "/exam/admin/subjects", {"name": "Проверка optimized_Michael " + time.strftime("%H:%M:%S")})
    subject_id = course["id"]
    started = time.monotonic()
    if not args.subject_id:
        boundary = uuid.uuid4().hex
        uploaded = request("POST", f"/exam/admin/subjects/{subject_id}/materials", upload_body(args.pdf, boundary),
                           f"multipart/form-data; boundary={boundary}")
        if uploaded["failed"] or not uploaded["uploaded"]:
            raise RuntimeError(str(uploaded))
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    build_requested = False
    while time.monotonic()-started < args.timeout:
        materials = request("GET", f"/exam/admin/subjects/{subject_id}/materials")
        snapshot = request("GET", f"/exam/admin/subjects/{subject_id}/graph")
        progress = {"subject_id": subject_id, "seconds": round(time.monotonic()-started, 2),
                    "materials": [{"id": m["id"], "status": m.get("cleaning", {}).get("status")} for m in materials],
                    "graph_status": snapshot["status"], "stale": snapshot["stale"]}
        (output/"upload_progress.json").write_text(json.dumps(progress, ensure_ascii=False, indent=2))
        if snapshot["status"] == "ready" and not snapshot["stale"]:
            assert len(materials) == 2 and all(m["cleaning"]["status"] == "ready" for m in materials)
            graph = snapshot["graph"]
            assert graph["topics"] and graph["quality"]["acyclic"]
            assert graph["prerequisites"], "No prerequisite found"
            assert any(e["target_evidence"]["material_id"] != e["prerequisite_evidence"]["material_id"]
                       for e in graph["prerequisites"]), "No cross-material prerequisite found"
            material_ids = {m["id"] for m in materials}
            assert all(s["material_id"] in material_ids for t in graph["topics"] for s in t["sources"])
            (output/"upload_snapshot.json").write_text(json.dumps(snapshot, ensure_ascii=False, indent=2))
            (output/"upload_materials.json").write_text(json.dumps(materials, ensure_ascii=False, indent=2))
            print(json.dumps({**progress, "topics": len(graph["topics"]), "edges": len(graph["prerequisites"]), "quality": graph["quality"]}, ensure_ascii=False, indent=2))
            return
        if snapshot["status"] == "failed" or any(m.get("cleaning", {}).get("status") == "failed" for m in materials):
            raise RuntimeError(json.dumps(progress, ensure_ascii=False))
        if not build_requested and needs_build(snapshot):
            request("POST", f"/exam/admin/subjects/{subject_id}/graph/build")
            build_requested = True
        time.sleep(3)
    raise TimeoutError("Graph did not finish; see upload_progress.json")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--url", default="https://localhost:8443/api/v1")
    parser.add_argument("--pdf", action="append", default=[], help="PDF to upload; pass exactly two related files")
    parser.add_argument("--output", required=True)
    parser.add_argument("--timeout", type=int, default=600)
    parser.add_argument("--subject-id", default="", help="Validate an existing smoke course without uploading again")
    arguments = parser.parse_args()
    if not arguments.subject_id and len(arguments.pdf) != 2:
        parser.error("pass --pdf twice, or --subject-id to check an existing course")
    main(arguments)
