#!/usr/bin/env python3
"""Measure a configured VoxCPM HTTP service; stdlib only, no weight downloads.

Client PCM arrival is not browser playback latency or model-only generation time.
Run each server configuration separately with the same pinned reference WAV.
"""

import argparse
import base64
import hashlib
import io
import json
import math
from pathlib import Path
import platform
import sys
import time
import urllib.request
import wave

MAX_PCM = 64 * 1024 * 1024
MAX_LINE = 256 * 1024


def load_cases(path):
    cases = json.loads(path.read_text())
    seen = set()
    ranges = {"short": (1, 100), "medium": (101, 500), "long": (501, 2000)}
    for case in cases:
        if case["id"] in seen:
            raise ValueError("duplicate case id")
        seen.add(case["id"])
        low, high = ranges[case["group"]]
        if not low <= len(case["text"]) <= high:
            raise ValueError(f"invalid length for {case['id']}")
        digest = hashlib.sha256(case["text"].encode()).hexdigest()
        if case["sha256"] != digest:
            raise ValueError(f"content hash mismatch for {case['id']}")
    return cases


def read_stream(response, output, started, clock=time.perf_counter):
    """Validate the entire stream before publishing a listening artifact."""
    rate = None
    pcm = bytearray()
    seq = 0
    first = None
    ended = False
    while True:
        line = response.readline(MAX_LINE + 1)
        if not line:
            break
        if len(line) > MAX_LINE or not line.endswith(b"\n"):
            raise ValueError("oversized or incomplete NDJSON line")
        event = json.loads(line)
        if ended:
            raise ValueError("data after terminal event")
        kind = event.get("type")
        if rate is None:
            if (kind != "start" or event.get("version") != 1
                    or event.get("channels") != 1 or event.get("encoding") != "pcm_s16le"
                    or type(event.get("sample_rate")) is not int or not 8000 <= event["sample_rate"] <= 192000):
                raise ValueError("invalid stream format")
            rate = event["sample_rate"]
        elif kind == "audio":
            if type(event.get("seq")) is not int or event["seq"] != seq:
                raise ValueError("discontinuous audio sequence")
            try:
                block = base64.b64decode(event["data"], validate=True)
            except (ValueError, TypeError, KeyError) as exc:
                raise ValueError("invalid PCM base64") from exc
            if not block or len(block) % 2 or len(pcm) + len(block) > MAX_PCM:
                raise ValueError("invalid PCM length")
            if first is None:
                first = clock() - started
            pcm.extend(block)
            seq += 1
        elif kind == "end":
            if not pcm or event.get("chunks") != seq or event.get("samples") != len(pcm) // 2:
                raise ValueError("incomplete PCM result")
            ended = True
        else:
            raise ValueError("failed or invalid stream event")
    if not ended:
        raise ValueError("stream ended without completion")
    with wave.open(str(output), "wb") as audio:
        audio.setparams((1, 2, rate, 0, "NONE", "not compressed"))
        audio.writeframes(pcm)
    return {"first_pcm_seconds": first, "audio_seconds": len(pcm) / 2 / rate, "pcm_bytes": len(pcm)}


def read_wav(response, output):
    data = response.read(MAX_PCM + 4097)
    if len(data) > MAX_PCM + 4096:
        raise ValueError("oversized WAV")
    with wave.open(io.BytesIO(data)) as audio:
        if audio.getnchannels() != 1 or audio.getsampwidth() != 2 or not audio.getnframes():
            raise ValueError("expected nonempty mono PCM16 WAV")
        size = audio.getnframes() * 2
        if len(audio.readframes(audio.getnframes())) != size or size > MAX_PCM:
            raise ValueError("truncated WAV")
        duration = audio.getnframes() / audio.getframerate()
    output.write_bytes(data)
    return {"first_pcm_seconds": None, "audio_seconds": duration, "pcm_bytes": size}


def percentile(values, fraction):
    values = sorted(values)
    return values[max(0, math.ceil(len(values) * fraction) - 1)] if values else None


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True, help="Full /synthesize URL; /stream is appended in stream mode")
    parser.add_argument("--mode", choices=("wav", "stream"), default="wav")
    parser.add_argument("--label", required=True, help="Server variant, e.g. baseline or compile-cache-10")
    parser.add_argument("--steps", type=int, choices=range(1, 51), default=20, metavar="1..50")
    parser.add_argument("--runs", type=int, default=3)
    parser.add_argument("--warmup", type=int, default=1)
    parser.add_argument("--timeout", type=float, default=300)
    parser.add_argument("--cases", type=Path, default=Path(__file__).parents[1] / "documents/benchmarks/voxcpm-cases.json")
    parser.add_argument("--output", type=Path, required=True, help="New output directory; never overwrites a previous run")
    parser.add_argument("--environment", type=Path, required=True, help="JSON with device/runtime/revision/reference SHA/settings")
    args = parser.parse_args(argv)
    if args.runs < 1 or args.warmup < 0 or args.timeout <= 0:
        parser.error("runs >= 1, warmup >= 0 and timeout > 0 required")
    cases = load_cases(args.cases)
    environment = json.loads(args.environment.read_text())
    required = ("device", "model_revision", "reference_sha256", "torch_version", "optimize", "reference_cache_size")
    if any(not str(environment.get(key, "")).strip() for key in required):
        parser.error("environment must identify " + ", ".join(required))
    args.output.mkdir(parents=True, exist_ok=False)
    (args.output / "environment.json").write_text(json.dumps(environment, ensure_ascii=False, indent=2) + "\n")
    url = args.url.rstrip("/") + ("/stream" if args.mode == "stream" else "")
    rows = []
    failures = 0
    with (args.output / "measurements.jsonl").open("w") as log:
        schedule = [("warmup", i, cases[0]) for i in range(args.warmup)]
        schedule += [("measured", run, case) for run in range(args.runs) for case in cases]
        for phase, run, case in schedule:
            name = f"{phase}-{run}-{case['id']}.wav"
            row = {"phase": phase, "run": run, "case": case["id"], "group": case["group"], "sha256": case["sha256"],
                   "steps": args.steps, "label": args.label, "mode": args.mode, "artifact": name}
            request = urllib.request.Request(url, json.dumps({"text": case["text"], "seed": 510, "cfg_value": 2,
                "inference_timesteps": args.steps, "use_reference": True}).encode(), {"Content-Type": "application/json"})
            started = time.perf_counter()
            try:
                with urllib.request.urlopen(request, timeout=args.timeout) as response:
                    content_type = response.headers.get_content_type()
                    expected = "application/x-ndjson" if args.mode == "stream" else "audio/wav"
                    if content_type != expected:
                        raise ValueError("unexpected content type")
                    if args.mode == "stream":
                        row.update(read_stream(response, args.output / name, started))
                    else:
                        row.update(read_wav(response, args.output / name))
                row["total_seconds"] = time.perf_counter() - started
                row["client_rtf"] = row["total_seconds"] / row["audio_seconds"]
            except Exception as exc:
                # Do not print provider bodies, potentially signed URLs or user content.
                row["error"] = type(exc).__name__
                failures += 1
            rows.append(row)
            log.write(json.dumps(row, ensure_ascii=False) + "\n")
            log.flush()
            print(f"{phase} {run} {case['id']}: {row.get('error', 'ok')}", flush=True)
    measured = [row for row in rows if row["phase"] == "measured" and "error" not in row]
    summary = {"label": args.label, "mode": args.mode, "steps": args.steps, "successful": len(measured),
               "failures_including_warmup": failures, "python": platform.python_version(),
               "note": "HTTP client timings, not browser playback TTFA or model-only RTF"}
    for metric in ("total_seconds", "first_pcm_seconds", "client_rtf"):
        values = [row[metric] for row in measured if row.get(metric) is not None]
        summary[metric] = {"p50": percentile(values, .5), "p95": percentile(values, .95)}
    (args.output / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
