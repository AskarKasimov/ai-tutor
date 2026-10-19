"""Summarize one run directory written by monitor.sh.

  python analyze.py <run-dir> [--from TS] [--to TS] [--prefix aitbench-] [--label NAME]

Per container (name prefix filter), over the window:
  ws_median / ws_max   working set (memory.current - inactive_file), MiB; median is
                       the typical level, max the highest 1 s sample
  anon_max             anonymous memory, MiB (cannot be reclaimed)
  peak_cgroup          cgroup memory.peak at the window end, MiB: the true peak
                       since the container started (catches sub-second spikes)
  cpu_mean / cpu_p95 / cpu_max  logical cores (1.0 = one hardware thread fully
                       busy; the machine has 16, Docker VM sees 16)
  swap_max, oom_kills (delta), restarts (delta), throttled_s (delta)
Stack rows sum the containers of each 1 s tick (working set and CPU), then take
median/max of that sum. VM row: used = MemTotal - MemAvailable of the Docker VM
(includes page cache only partially; it is what the VM really needs). Host row:
Windows available memory and the vmmemWSL working set; GPU: whole-device numbers.
The three levels overlap (containers are inside the VM, the VM inside the host)
and are never added together.
"""
import argparse
import csv
import json
import statistics
from collections import defaultdict
from pathlib import Path

MIB = 2 ** 20


def pct(values, q):
    if not values:
        return None
    values = sorted(values)
    return values[min(len(values) - 1, int(round(q * (len(values) - 1))))]


def r(x, n=1):
    return None if x is None else round(x, n)


def main():
    p = argparse.ArgumentParser()
    p.add_argument("run")
    p.add_argument("--from", dest="start", type=float)
    p.add_argument("--to", dest="end", type=float)
    p.add_argument("--prefix", default="aitbench-")
    p.add_argument("--label", default="")
    a = p.parse_args()
    run = Path(a.run)
    inside = lambda ts: (a.start is None or ts >= a.start) and (a.end is None or ts <= a.end)
    rows = defaultdict(list)
    for row in csv.DictReader(open(run / "containers.csv")):
        ts = float(row["ts"])
        if inside(ts):
            rows[row["container"]].append(row)
    out = {"label": a.label, "from": a.start, "to": a.end, "containers": {}, "stack": {}, "vm": {}, "host": {}}
    ticks_ws, ticks_cpu = defaultdict(float), defaultdict(float)
    for name, items in sorted(rows.items()):
        if name == "_vm" or not name.startswith(a.prefix) or name == "aitbench-sampler":
            continue
        ws = [int(i["mem_ws"]) / MIB for i in items]
        cpu = [float(i["cpu_cores"]) for i in items if i["cpu_cores"] not in ("", None)]
        for i in items:
            key = round(float(i["ts"]))
            ticks_ws[key] += int(i["mem_ws"]) / MIB
            if i["cpu_cores"]:
                ticks_cpu[key] += float(i["cpu_cores"])
        first, last = items[0], items[-1]
        out["containers"][name] = {
            "samples": len(items),
            "ws_median": r(statistics.median(ws)), "ws_max": r(max(ws)),
            "anon_max": r(max(int(i["mem_anon"]) for i in items) / MIB),
            "peak_cgroup": r(int(last["mem_peak"]) / MIB),
            "cpu_mean": r(statistics.fmean(cpu), 3) if cpu else None,
            "cpu_p95": r(pct(cpu, 0.95), 3), "cpu_max": r(max(cpu), 3) if cpu else None,
            "swap_max": r(max(int(i["swap_current"]) for i in items) / MIB),
            "oom_kills": int(last["oom_kill"]) - int(first["oom_kill"]),
            "restarts": int(last["restarts"] or 0) - int(first["restarts"] or 0),
            "throttled_s": r((int(last["throttled_us"]) - int(first["throttled_us"])) / 1e6, 2),
        }
    if ticks_ws:
        ws, cpu = list(ticks_ws.values()), list(ticks_cpu.values())
        out["stack"] = {"ws_median": r(statistics.median(ws)), "ws_max": r(max(ws)),
                        "cpu_mean": r(statistics.fmean(cpu), 3) if cpu else None,
                        "cpu_p95": r(pct(cpu, 0.95), 3), "cpu_max": r(max(cpu), 3) if cpu else None,
                        "seconds": len(ws)}
    vm = rows.get("_vm", [])
    if vm:
        used = [int(i["mem_current"]) / MIB for i in vm]
        cpu = [float(i["cpu_cores"]) for i in vm if i["cpu_cores"]]
        swap = [int(i["swap_current"]) / MIB for i in vm]
        out["vm"] = {"used_median": r(statistics.median(used)), "used_max": r(max(used)),
                     "swap_max": r(max(swap)), "swap_min": r(min(swap)),
                     "cpu_mean": r(statistics.fmean(cpu), 3) if cpu else None, "cpu_max": r(max(cpu), 3) if cpu else None,
                     "oom_kills": int(vm[-1]["oom_kill"]) - int(vm[0]["oom_kill"])}
    host_file = run / "host.csv"
    if host_file.exists():
        host = [h for h in csv.DictReader(open(host_file)) if h["ts"] and inside(float(h["ts"]))]
        num = lambda key: [float(h[key]) for h in host if h.get(key) not in ("", None)]
        if host:
            avail, vmws, cpu = num("host_avail_mb"), num("vmmem_ws_mb"), num("host_cpu_pct")
            gu, gm = num("gpu_util_pct"), num("gpu_mem_used_mb")
            out["host"] = {"avail_min_mib": r(min(avail)) if avail else None,
                           "avail_median_mib": r(statistics.median(avail)) if avail else None,
                           "vmmem_ws_max_mib": r(max(vmws)) if vmws else None,
                           "vmmem_ws_median_mib": r(statistics.median(vmws)) if vmws else None,
                           "cpu_pct_mean": r(statistics.fmean(cpu)) if cpu else None,
                           "cpu_pct_max": r(max(cpu)) if cpu else None,
                           "gpu_util_mean": r(statistics.fmean(gu)) if gu else None, "gpu_util_max": r(max(gu)) if gu else None,
                           "gpu_mem_max_mib": r(max(gm)) if gm else None,
                           "gpu_mem_min_mib": r(min(gm)) if gm else None}
    print(json.dumps(out, ensure_ascii=False, indent=1))


if __name__ == "__main__":
    main()
