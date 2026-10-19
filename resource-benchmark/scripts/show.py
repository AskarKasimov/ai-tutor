"""Print a summary.json of analyze.py as a compact table (MiB, logical cores)."""
import json
import sys

sys.stdout.reconfigure(encoding="utf-8")
d = json.load(open(sys.argv[1], encoding="utf-8"))
print(f"{'container':40} {'ws_med':>8} {'ws_max':>8} {'anon_max':>8} {'peak':>8} {'cpu_mean':>8} {'cpu_p95':>8} {'cpu_max':>8} oom rst swap")
for name, c in d["containers"].items():
    print(f"{name.removeprefix('aitbench-'):40} {c['ws_median']:8} {c['ws_max']:8} {c['anon_max']:8} {c['peak_cgroup']:8} "
          f"{c['cpu_mean']!s:>8} {c['cpu_p95']!s:>8} {c['cpu_max']!s:>8} {c['oom_kills']:3} {c['restarts']:3} {c['swap_max']}")
print("stack", d["stack"])
print("vm   ", d["vm"])
print("host ", d["host"])
