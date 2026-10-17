"""Checkpointed validation of original PDFs through the actual upload pipeline.

No reviewed topic cards or artificial educational content are used. Duplicate
PDF bytes are skipped. Re-run with the same output directory to continue.
"""
import argparse
import hashlib
import http.cookiejar
import json
import re
import ssl
import subprocess
import time
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

from audit_pdf_graph import audit as audit_graph


def protected_spans(text):
    return re.findall(r"```.*?```|~~~.*?~~~|\$\$.*?\$\$|\\\[.*?\\\]|(?<!\$)\$[^$\n]+\$(?!\$)", text, re.S)


def language_ratio(text):
    text = re.sub(r"```.*?```|\$\$.*?\$\$|`[^`\n]*`|https?://\S+|<!--.*?-->", "", text, flags=re.S)
    latin = len(re.findall(r"[a-zA-Z]", text))
    cyrillic = len(re.findall(r"[а-яА-ЯёЁ]", text))
    return round(cyrillic/max(1, latin+cyrillic), 4)


def native_programs(text):
    # Literal expectations from the PDF text layer, independent of generative OCR.
    return list(dict.fromkeys(re.findall(r"\b[a-z][A-Za-z_]*\([^\n]+?\)\s*:-[^\n]*?\.",text)))


def missing_programs(programs, text):
    compact = lambda value: re.sub(r"\s+", "", value)
    target = compact(text)
    return [program for program in programs if compact(program) not in target]


def main(args):
    if urllib.parse.urlsplit(args.url).hostname not in {"localhost", "127.0.0.1"}:
        raise ValueError("Validation is restricted to localhost")
    output = Path(args.output)
    output.mkdir(parents=True, exist_ok=True)
    manifest_path = output/"manifest.json"
    manifest = json.loads(manifest_path.read_text()) if manifest_path.exists() else {"files": {}, "started_at": time.time()}
    opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()),
                                         urllib.request.HTTPSHandler(context=ssl._create_unverified_context()))

    def request(method, path, data=None, content_type="application/json", base=None):
        if isinstance(data, dict):
            data = json.dumps(data, ensure_ascii=False).encode()
        req = urllib.request.Request((base or args.url)+path, data=data, method=method,
                                     headers={"Content-Type": content_type, "X-AI-Tutor-Portal": "admin"})
        with opener.open(req, timeout=7200) as response:
            return json.load(response)

    def save():
        manifest["updated_at"] = time.time()
        temporary = output/"manifest.tmp"
        temporary.write_text(json.dumps(manifest, ensure_ascii=False, indent=2))
        temporary.replace(manifest_path)

    def multipart(path):
        boundary = uuid.uuid4().hex
        header = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{path.name}"\r\n'
                  'Content-Type: application/pdf\r\n\r\n').encode()
        return header+path.read_bytes()+f"\r\n--{boundary}--\r\n".encode(), f"multipart/form-data; boundary={boundary}"

    request("POST", "/auth/teacher")
    if "subject_id" not in manifest:
        course = request("POST", "/admin/subjects", {"name": "PDF с нуля optimized_Michael " + time.strftime("%d.%m %H:%M")})
        manifest["subject_id"] = course["id"]
        save()
    subject_id = manifest["subject_id"]
    files = [p for p in Path(args.directory).glob("*.pdf") if p.is_file()]
    files.sort(key=lambda p: ("(Copy)" in p.name, p.name))
    seen, unique, duplicates = {}, [], []
    for path in files:
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        if digest in seen:
            duplicates.append({"file": path.name, "same_as": seen[digest]})
        else:
            seen[digest] = path.name
            unique.append(path)
    first = ["Week 3 Lecture.pdf", "Week 3 Lab.pptx.pdf", "Week 04 Lab.pdf"]
    unique.sort(key=lambda p: (first.index(p.name) if p.name in first else len(first), p.name))
    manifest["duplicates"] = duplicates
    selected = unique[:args.limit] if args.limit else unique
    manifest["selected_files"] = [p.name for p in selected]
    save()
    for number, path in enumerate(selected, 1):
        entry = manifest["files"].get(path.name, {})
        if entry.get("material_id") and entry.get("status") != "error":
            continue
        if entry.get("material_id"):
            current = request("GET", f"/admin/subjects/{subject_id}/materials")
            failed = next((m for m in current if m["id"] == entry["material_id"] and m["status"] == "failed"), None)
            if failed:
                request("DELETE", f"/admin/subjects/{subject_id}/materials/{failed['id']}")
            else:
                raise RuntimeError("Resume has a parsed material with an incomplete audit; inspect it before uploading a duplicate")
        digest = hashlib.sha256(path.read_bytes()).hexdigest()
        directory = output/digest[:12]
        directory.mkdir(exist_ok=True)
        entry = {"sha256": digest, "status": "uploading", "artifact_dir": directory.name, "started_at": time.time()}
        manifest["files"][path.name] = entry
        save()
        print(f"[{number}/{len(selected)}] PDF upload: {path.name}", flush=True)
        body, content_type = multipart(path)
        started = time.monotonic()
        try:
            uploaded = request("POST", f"/admin/subjects/{subject_id}/materials", body, content_type)
            (directory/"upload.json").write_text(json.dumps(uploaded, ensure_ascii=False, indent=2))
            if uploaded.get("failed") or not uploaded.get("uploaded"):
                raise RuntimeError(str(uploaded))
            material = uploaded["uploaded"][0]
            entry.update(material_id=material["id"], status=material["status"], upload_seconds=round(time.monotonic()-started, 2))
            if material["status"] != "parsed":
                raise RuntimeError("PDF did not parse: " + str(material.get("error_message")))
            (directory/"translated.md").write_text(material["parsed_text"])
            save()
            # Exact same PDF: cache returns the OCR artifact produced by upload.
            raw = request("POST", "/ocr/convert", body, content_type, args.ocr_url)
            (directory/"ocr.json").write_text(json.dumps(raw, ensure_ascii=False, indent=2))
            (directory/"ocr.md").write_text(raw["markdown"])
            native = subprocess.check_output(["pdftotext", "-layout", str(path), "-"], stderr=subprocess.DEVNULL).decode(errors="replace")
            (directory/"native.txt").write_text(native)
            native_words = set(re.findall(r"[a-zA-Z]{3,}", native.casefold()))
            ocr_words = set(re.findall(r"[a-zA-Z]{3,}", raw["markdown"].casefold()))
            missing = [span for span in protected_spans(raw["markdown"]) if span not in material["parsed_text"]]
            programs = native_programs(native)
            lost_programs = {"ocr": missing_programs(programs, raw["markdown"]),
                             "translation": missing_programs(programs, material["parsed_text"])}
            entry["audit"] = {"ocr": raw.get("report"), "native_word_recall": round(len(native_words & ocr_words)/max(1,len(native_words)), 4),
                              "ocr_chars": len(raw["markdown"]), "translated_chars": len(material["parsed_text"]),
                              "russian_prose_ratio": language_ratio(material["parsed_text"]),
                              "protected_fragments": len(protected_spans(raw["markdown"])), "changed_protected_fragments": len(missing),
                              "translated_s3": "/translated/" in material["s3_parsed_key"],
                              "native_programs": programs, "lost_programs": lost_programs}
            if missing:
                (directory/"changed_protected_fragments.json").write_text(json.dumps(missing, ensure_ascii=False, indent=2))
            entry["status"] = "uploaded"
            save()
            if missing or any(lost_programs.values()):
                raise RuntimeError("Code/formula preservation failed; inspect this material's audit")
            print(f"  OCR={entry['audit']['ocr']['engine']}; upload={entry['upload_seconds']}s; RU={entry['audit']['russian_prose_ratio']}; protected changed={len(missing)}", flush=True)
        except Exception as error:
            entry["status"] = "error"
            entry["error"] = str(error)
            save()
            raise
    deadline = time.monotonic()+args.wait
    while time.monotonic() < deadline:
        materials = request("GET", f"/admin/subjects/{subject_id}/materials")
        snapshot = request("GET", f"/admin/subjects/{subject_id}/graph")
        (output/"materials.json").write_text(json.dumps(materials, ensure_ascii=False, indent=2))
        (output/"snapshot.json").write_text(json.dumps(snapshot, ensure_ascii=False, indent=2))
        manifest["cleaning"] = {m["filename"]: m.get("cleaning") for m in materials}
        manifest["graph_status"] = snapshot["status"]
        manifest["stale"] = snapshot["stale"]
        save()
        if snapshot["status"] == "ready" and not snapshot["stale"]:
            graph = snapshot["graph"]
            checks = audit_graph(materials, graph)
            for material in materials:
                entry = manifest["files"].get(material["filename"])
                if entry:
                    (output/entry["artifact_dir"]/"cleaned.md").write_text(material["cleaning"]["cleaned_text"])
                    lost = missing_programs(entry.get("audit",{}).get("native_programs",[]), material["cleaning"]["cleaned_text"])
                    if lost:
                        checks["errors"].append("Cleaning lost literal PDF programs: " + material["filename"])
                        checks["structural_checks_passed"] = False
            (output/"independent_audit.json").write_text(json.dumps(checks, ensure_ascii=False, indent=2))
            manifest["independent_audit"] = checks
            if not checks["structural_checks_passed"]:
                save()
                raise RuntimeError("Independent graph audit failed: " + str(checks["errors"]))
            manifest["graph_quality"] = graph["quality"]
            manifest["finished_at"] = time.time()
            save()
            print(json.dumps({"subject_id": subject_id, "materials": len(materials), "quality": graph["quality"]}, ensure_ascii=False, indent=2), flush=True)
            return
        if any((m.get("cleaning") or {}).get("status") == "failed" for m in materials) or snapshot["status"] == "failed":
            raise RuntimeError("A stage failed; inspect manifest.json and service logs before retrying")
        time.sleep(5)
    raise TimeoutError("Graph has not finished; resume with the same output directory")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", default="notebooks")
    parser.add_argument("--output", required=True)
    parser.add_argument("--url", default="https://localhost:8443/api/v1")
    parser.add_argument("--ocr-url", default="http://127.0.0.1:8005")
    parser.add_argument("--limit", type=int, default=0, help="0 means all unique PDFs")
    parser.add_argument("--wait", type=int, default=2400)
    main(parser.parse_args())
