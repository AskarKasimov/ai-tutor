"""Check every dependency with a real request, not the backend health.

  python deps.py --out run/deps.jsonl

  backend health      GET /api/v1/health through Caddy (backend + DB only)
  PostgreSQL          migrations applied (goose table) and a test write via registration
  S3                  backend writes and reads an object: TTS audio of a turn is cached in S3
  LLM gateway         POST /v1/chat/completions for complexity low, medium, high
  TTS                 POST /voice/syntheses (backend -> remote TTS), WAV bytes
  STT                 POST /voice/transcriptions with that WAV (backend -> remote STT)
  OCR models          GET course-graph /ready/models
Each check is one JSON line with seconds and the result.
"""
import sys as _sys
_sys.stdout.reconfigure(encoding="utf-8")
import argparse
import json
import subprocess
import time
import urllib.request

from diag import Client, Log, multipart


def timed(log, name, fn):
    started = time.time()
    try:
        detail = fn()
        log.write(ts=round(started, 3), session="deps", op=f"check_{name}", ok=True,
                  seconds=round(time.time() - started, 3), detail=detail)
    except Exception as error:
        log.write(ts=round(started, 3), session="deps", op=f"check_{name}", ok=False,
                  seconds=round(time.time() - started, 3), detail=str(error)[:300])


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--base", default="https://localhost:8443/api/v1")
    p.add_argument("--gateway", default="http://127.0.0.1:8010")
    p.add_argument("--course-graph", default="http://127.0.0.1:8005")
    p.add_argument("--db-container", default="aitbench-postgresql-db-1")
    p.add_argument("--out", required=True)
    a = p.parse_args()
    log = Log(a.out)
    client = Client(a.base, log, "deps")

    timed(log, "backend_health", lambda: client.call("health", "GET", "/health"))
    timed(log, "postgres_migrations", lambda: subprocess.run(
        ["docker", "exec", a.db_container, "psql", "-U", "ai_tutor", "-d", "ai_tutor", "-tA", "-c",
         "select count(*), max(version_id) from goose_db_version where is_applied"],
        capture_output=True, text=True, check=True).stdout.strip())
    run = time.strftime("%m%d%H%M%S")
    timed(log, "postgres_write_register", lambda: client.call(
        "register", "POST", "/auth/register",
        {"email": f"bench-{run}-deps@example.edu", "password": "Тестовая фраза стенда 42",
         "display_name": "Bench deps"})["user"]["id"])

    def llm(level):
        body = json.dumps({"messages": [{"role": "system", "content": "Ответь одним словом."},
                                        {"role": "user", "content": "Столица России?"}],
                           "complexity": level, "max_tokens": 256}).encode()
        request = urllib.request.Request(a.gateway + "/v1/chat/completions", data=body, method="POST",
                                         headers={"Content-Type": "application/json",
                                                  "Idempotency-Key": f"bench-{run}-{level}"})
        with urllib.request.urlopen(request, timeout=300) as response:
            data = json.loads(response.read())
        return {"answer": data["choices"][0]["message"]["content"][:40], "usage": data.get("usage")}

    for level in ("low", "medium", "high"):
        timed(log, f"llm_{level}", lambda level=level: llm(level))

    wav = {}

    def tts():
        wav["data"] = client.call("tts", "POST", "/voice/syntheses",
                                  {"text": "Эвристика оценивает стоимость пути до цели."}, raw=True)
        return {"wav_bytes": len(wav["data"])}

    def stt():
        form, ctype = multipart("audio", "check.wav", wav["data"], "audio/wav")
        return client.call("stt", "POST", "/voice/transcriptions", form, ctype=ctype)["text"]

    timed(log, "tts", tts)
    timed(log, "stt", stt)
    timed(log, "ocr_models_ready", lambda: json.loads(
        urllib.request.urlopen(a.course_graph + "/ready/models", timeout=10).read()))
    for line in open(a.out, encoding="utf-8"):
        record = json.loads(line)
        if record["op"].startswith("check_"):
            print(record["op"], "OK" if record["ok"] else "FAIL", record["seconds"], str(record["detail"])[:120])


if __name__ == "__main__":
    main()
