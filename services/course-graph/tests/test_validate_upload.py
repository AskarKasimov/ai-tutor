import argparse
import email
import importlib.util
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

import pytest


def load_script():
    script = Path(__file__).parents[1] / "scripts" / "validate_upload.py"
    spec = importlib.util.spec_from_file_location("validate_upload", script)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def test_upload_body_sends_the_manifest_and_the_pdfs_in_its_order(tmp_path):
    first, second = tmp_path / "first.pdf", tmp_path / "second.pdf"
    first.write_bytes(b"%PDF-1.7 first")
    second.write_bytes(b"%PDF-1.7 second")
    body = load_script().upload_body([first, second], "boundary")
    message = email.message_from_bytes(b"Content-Type: multipart/form-data; boundary=boundary\r\n\r\n" + body)
    items, *files = message.get_payload()
    assert items.get_param("name", header="content-disposition") == "items"
    assert json.loads(items.get_payload()) == [
        {"client_id": "file-1", "week": 1, "kind": "lecture"},
        {"client_id": "file-2", "week": 2, "kind": "lecture"},
    ]
    assert [part.get_filename() for part in files] == ["first.pdf", "second.pdf"]
    assert [part.get_content_type() for part in files] == ["application/pdf"] * 2
    assert files[0].get_payload().startswith("%PDF-")


GRAPH = {
    "topics": [{"sources": [{"material_id": "m1"}, {"material_id": "m2"}]}],
    "quality": {"acyclic": True},
    "prerequisites": [{"target_evidence": {"material_id": "m1"}, "prerequisite_evidence": {"material_id": "m2"}}],
}


def snapshot(status, inputs_ready=True, stale=False):
    return {"status": status, "inputs_ready": inputs_ready, "stale": stale, "graph": GRAPH if status == "ready" else None}


def run_against_api(tmp_path, monkeypatch, graphs, after_build, **arguments):
    """Run the script against a fake API. Each read takes the next graph state; a build switches to after_build."""
    module = load_script()
    monkeypatch.setattr(module.time, "sleep", lambda _: None)
    state = {"graphs": list(graphs), "builds": 0}
    materials = [{"id": id, "cleaning": {"status": "ready"}} for id in ("m1", "m2")]

    class Api(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def answer(self, value, status=200):
            self.send_response(status)
            self.end_headers()
            self.wfile.write(json.dumps(value).encode())

        def do_POST(self):
            self.rfile.read(int(self.headers.get("Content-Length") or 0))
            if self.path.endswith("/graph/build"):
                state["builds"] += 1
                state["graphs"] = list(after_build)
                self.answer(snapshot("pending"), 202)
            elif self.path.endswith("/materials"):
                self.answer({"uploaded": materials, "failed": []})
            elif self.path.endswith("/admin/subjects"):
                self.answer({"id": "new"}, 201)
            else:
                self.answer({})

        def do_GET(self):
            if self.path.endswith("/materials"):
                self.answer(materials)
            else:
                graphs = state["graphs"]
                self.answer(graphs.pop(0) if len(graphs) > 1 else graphs[0])

    server = ThreadingHTTPServer(("127.0.0.1", 0), Api)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        module.main(argparse.Namespace(**{
            "url": f"http://127.0.0.1:{server.server_port}/api/v1", "output": str(tmp_path / "out"), "timeout": 5,
            "pdf": [], "subject_id": "existing", **arguments}))
    finally:
        server.shutdown()
        server.server_close()
    return state["builds"]


def test_new_course_requests_one_build_after_the_materials_are_ready(tmp_path, monkeypatch):
    pdf = tmp_path / "first.pdf"
    pdf.write_bytes(b"%PDF-1.7")
    builds = run_against_api(tmp_path, monkeypatch,
                             [snapshot("not_requested", inputs_ready=False), snapshot("not_requested")],
                             [snapshot("pending"), snapshot("processing"), snapshot("ready")],
                             subject_id="", pdf=[pdf, pdf])
    assert builds == 1


@pytest.mark.parametrize("graphs, after_build, expected", [
    ([snapshot("ready")], [snapshot("ready")], 0),
    ([snapshot("processing"), snapshot("ready")], [snapshot("ready")], 0),
    ([snapshot("ready", stale=True)], [snapshot("processing"), snapshot("ready")], 1),
])
def test_existing_course_builds_only_when_needed(tmp_path, monkeypatch, graphs, after_build, expected):
    assert run_against_api(tmp_path, monkeypatch, graphs, after_build) == expected
