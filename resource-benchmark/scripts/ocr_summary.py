"""Aggregate <device>-run*.json of ocr_bench.py: per document mean/min/max over runs."""
import glob
import json
import statistics
import sys

sys.stdout.reconfigure(encoding="utf-8")
runs = [json.load(open(p, encoding="utf-8")) for p in sorted(glob.glob(sys.argv[1]))]
out = {"runs": len(runs), "device": runs[0]["device"], "torch": runs[0]["torch"], "cuda_device": runs[0]["cuda_device"],
       "configuration": runs[0]["configuration"], "threads": runs[0]["threads"]}
stat = lambda xs: {"mean": round(statistics.fmean(xs), 2), "min": round(min(xs), 2), "max": round(max(xs), 2)}
out["init_seconds"] = stat([r["init_seconds"] for r in runs])
out["first_conversion_seconds"] = stat([r["first_conversion"]["cold_seconds"] for r in runs])
out["process_peak_rss_mib"] = stat([r["peak_rss_kib"] / 1024 for r in runs])
out["cgroup_peak_mib"] = stat([r.get("cgroup_memory_peak", 0) / 2**20 for r in runs])
docs = {}
for r in runs:
    for d in r["documents"]:
        docs.setdefault(d["file"], []).append(d)
out["documents"] = {}
for name, items in docs.items():
    entry = {"pages": items[0]["pages"], "sha256": items[0]["sha256"], "bytes": items[0]["bytes"],
             "cold_seconds": stat([i["cold_seconds"] for i in items]),
             "seconds_per_page": stat([i["seconds_per_page"] for i in items]),
             "cached_ms": stat([i["cached_seconds"] * 1000 for i in items]),
             "peak_rss_mib": stat([i["peak_rss_kib"] / 1024 for i in items]),
             "markdown_identical_across_runs": len({i["markdown_sha256"] for i in items}) == 1,
             "content_complete": [i["content_complete"] for i in items], "warnings": [i["warnings"] for i in items]}
    if "cuda_max_reserved_mib" in items[0]:
        entry["cuda_max_allocated_mib"] = stat([i["cuda_max_allocated_mib"] for i in items])
        entry["cuda_max_reserved_mib"] = stat([i["cuda_max_reserved_mib"] for i in items])
    out["documents"][name] = entry
total_pages = sum(d["pages"] for d in out["documents"].values())
per_run = [sum(d["cold_seconds"] for d in r["documents"]) for r in runs]
out["all_documents"] = {"pages": total_pages, "cold_seconds": stat(per_run),
                        "seconds_per_page": stat([s / total_pages for s in per_run])}
print(json.dumps(out, ensure_ascii=False, indent=1))
