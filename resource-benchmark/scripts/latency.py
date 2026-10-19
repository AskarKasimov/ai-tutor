"""Latency of the HTTP requests in a requests.jsonl of diag.py / pipeline.py, by operation.

  python latency.py <requests.jsonl> [--json out.json]
seconds = client-side wall time of the whole request (through Caddy).
"""
import argparse
import json
import statistics
import sys
from collections import defaultdict

sys.stdout.reconfigure(encoding="utf-8")


def pct(values, q):
    values = sorted(values)
    return values[min(len(values) - 1, int(round(q * (len(values) - 1))))]


p = argparse.ArgumentParser()
p.add_argument("log")
p.add_argument("--json")
a = p.parse_args()
ops = defaultdict(list)
errors = defaultdict(list)
for line in open(a.log, encoding="utf-8"):
    r = json.loads(line)
    if "seconds" not in r or "status" not in r:
        continue
    ops[r["op"]].append(r["seconds"])
    if r.get("error"):
        errors[r["op"]].append(f"{r['status']} {r['error']}")
out = {}
print(f"{'op':26} {'n':>4} {'median':>8} {'p95':>8} {'max':>8} errors")
for op, values in sorted(ops.items()):
    out[op] = {"n": len(values), "median": round(statistics.median(values), 3), "p95": round(pct(values, 0.95), 3),
               "max": round(max(values), 3), "errors": errors.get(op, [])}
    print(f"{op:26} {len(values):4} {out[op]['median']:8} {out[op]['p95']:8} {out[op]['max']:8} {len(errors.get(op, []))} {errors.get(op, [''])[0][:60] if errors.get(op) else ''}")
if a.json:
    json.dump(out, open(a.json, "w", encoding="utf-8"), ensure_ascii=False, indent=1)
