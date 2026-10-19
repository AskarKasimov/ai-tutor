"""Run real Geek diagnostics over the public API, as the browser does.

Every session belongs to a new test student (bench-<run>-<n>@example.edu) on a
test course created through the admin panel account; real users are not touched.
A session repeats the browser loop until Geek completes it:

  read the current Geek turn -> (voice) GET its audio = TTS
  -> answer: text, or (voice) WAV -> POST /voice/transcriptions = STT -> answer
     with the transcription -> backend grades it with the LLM -> next turn

Answers cycle strong / weak / "не знаю" like services/backend/scripts/geek-smoke.sh.
The voice answer is a WAV synthesized once per answer text through
POST /voice/syntheses (backend TTS), so STT gets real Russian speech.

  python diag.py --sessions 3 --mode text --out run/requests.jsonl
  python diag.py --sessions 1 --mode voice --out run/requests.jsonl
  python diag.py --open-app --out run/requests.jsonl      # only the frontend load

Each HTTP request is one JSON line: ts, session, op, seconds, status, bytes, error.
"""
import sys as _sys
_sys.stdout.reconfigure(encoding="utf-8")
import argparse
import http.cookiejar
import json
import re
import ssl
import sys
import threading
import time
import urllib.error
import urllib.request
import uuid

CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE

STRONG = ("Эвристика оценивает стоимость пути до цели, и если она не переоценивает расстояние, "
          "алгоритм находит оптимальное решение, а иначе может ошибиться.")
WEAK = "Ну это поиск"
lock = threading.Lock()


class Log:
    def __init__(self, path):
        self.file = open(path, "a", encoding="utf-8")

    def write(self, **record):
        with lock:
            self.file.write(json.dumps(record, ensure_ascii=False) + "\n")
            self.file.flush()


class Client:
    def __init__(self, base, log, session, admin=False):
        self.base, self.log, self.session = base, log, session
        self.headers = {"X-AI-Tutor-Portal": "admin"} if admin else {}
        jar = http.cookiejar.CookieJar()
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(jar),
                                                  urllib.request.HTTPSHandler(context=CTX))

    def call(self, op, method, path, body=None, headers=None, raw=False, ctype="application/json"):
        data = body if isinstance(body, (bytes, type(None))) else json.dumps(body).encode()
        request = urllib.request.Request(self.base + path, data=data, method=method,
                                         headers={**self.headers, **({"Content-Type": ctype} if data else {}),
                                                  **(headers or {})})
        started = time.time()
        status, payload, error = None, b"", None
        try:
            with self.opener.open(request, timeout=600) as response:
                status, payload = response.status, response.read()
        except urllib.error.HTTPError as failure:
            status, payload, error = failure.code, failure.read(), failure.reason
        except Exception as failure:  # timeouts, resets: logged, the session stops
            error = repr(failure)
        self.log.write(ts=round(started, 3), session=self.session, op=op, seconds=round(time.time() - started, 3),
                       status=status, bytes=len(payload), error=error and str(error)[:300],
                       body=(payload[:300].decode("utf-8", "replace") if error else None))
        if error:
            raise RuntimeError(f"{op}: {status} {error}")
        return payload if raw else (json.loads(payload) if payload else None)


def multipart(field, filename, content, ctype):
    boundary = uuid.uuid4().hex
    head = (f"--{boundary}\r\nContent-Disposition: form-data; name=\"{field}\"; filename=\"{filename}\"\r\n"
            f"Content-Type: {ctype}\r\n\r\n").encode()
    return head + content + f"\r\n--{boundary}--\r\n".encode(), f"multipart/form-data; boundary={boundary}"


def open_app(base, log):
    site = base.rsplit("/api/", 1)[0]
    client = Client(site, log, "app")
    html = client.call("app_index", "GET", "/", raw=True).decode("utf-8", "replace")
    for asset in sorted(set(re.findall(r'(?:src|href)="(/assets/[^"]+)"', html))):
        client.call("app_asset", "GET", asset, raw=True)


def student(base, log, run, n, subject, mode, voices, results):
    name = f"s{n}"
    client = Client(base, log, name)
    email = f"bench-{run}-{n}@example.edu"
    try:
        client.call("register", "POST", "/auth/register",
                    {"email": email, "password": "Тестовая фраза стенда 42", "display_name": f"Bench {n}"})
        client.call("subjects", "GET", "/subjects")
        session = client.call("start", "POST", "/diagnostic-sessions", {"subject_id": subject},
                              headers={"Idempotency-Key": f"start-{email}"})
        sid, step = session["id"], 0
        while session["status"] == "active":
            turn = session["current_turn_id"]
            if mode == "voice":
                client.call("turn_audio_tts", "GET", f"/diagnostic-sessions/{sid}/turns/{turn}/audio", raw=True)
            kind = ("strong", "weak", "dont_know")[step % 3]
            if kind == "dont_know":
                body = {"turn_id": turn, "dont_know": True}
            elif mode == "voice":
                wav = voices[kind]
                form, ctype = multipart("audio", "answer.wav", wav, "audio/wav")
                transcription = client.call("transcribe_stt", "POST", "/voice/transcriptions", form, ctype=ctype)
                body = {"turn_id": turn, "transcription_id": transcription["id"]}
            else:
                body = {"turn_id": turn, "text": STRONG if kind == "strong" else WEAK}
            session = client.call(f"answer_{kind}", "POST", f"/diagnostic-sessions/{sid}/answers", body,
                                  headers={"Idempotency-Key": f"answer-{turn}"})
            step += 1
        result = client.call("result", "GET", f"/diagnostic-sessions/{sid}/result")
        results[n] = {"session": sid, "answers": step, "grade": result.get("grade"), "ok": True}
    except Exception as error:
        results[n] = {"ok": False, "error": str(error)[:300]}


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--base", default="https://localhost:8443/api/v1")
    parser.add_argument("--sessions", type=int, default=1)
    parser.add_argument("--mode", choices=["text", "voice"], default="text")
    parser.add_argument("--open-app", action="store_true")
    parser.add_argument("--subject", help="existing test course; default: create one")
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    log = Log(args.out)
    run = time.strftime("%m%d%H%M%S")
    if args.open_app:
        open_app(args.base, log)
        return
    admin = Client(args.base, log, "admin", admin=True)
    admin.call("teacher_login", "POST", "/auth/teacher")
    subject = args.subject or admin.call("create_subject", "POST", "/admin/subjects",
                                         {"name": f"BENCH ресурсы {run}"})["id"]
    voices = {}
    if args.mode == "voice":
        speaker = Client(args.base, log, "voice-setup")
        speaker.call("register", "POST", "/auth/register",
                     {"email": f"bench-{run}-voice@example.edu", "password": "Тестовая фраза стенда 42",
                      "display_name": "Bench voice"})
        for kind, text in (("strong", STRONG), ("weak", WEAK)):
            voices[kind] = speaker.call(f"synthesize_tts_{kind}", "POST", "/voice/syntheses", {"text": text}, raw=True)
    results = {}
    threads = [threading.Thread(target=student, args=(args.base, log, run, n, subject, args.mode, voices, results))
               for n in range(args.sessions)]
    started = time.time()
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()
    summary = {"run": run, "subject": subject, "mode": args.mode, "sessions": args.sessions,
               "seconds": round(time.time() - started, 1), "results": results}
    print(json.dumps(summary, ensure_ascii=False))
    sys.exit(0 if all(r.get("ok") for r in results.values()) else 1)


if __name__ == "__main__":
    main()
