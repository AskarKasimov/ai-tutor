import email
import importlib.util
import json
from pathlib import Path


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
