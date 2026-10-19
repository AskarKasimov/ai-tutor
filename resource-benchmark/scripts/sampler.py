"""Once a second, record cgroup v2 counters of every container and of the Docker VM.

Runs inside a privileged container (see monitor.sh) with the host cgroup tree at
/host/cgroup and the Docker socket for container names. Output, one CSV row per
container per tick:

  ts            Unix time of the tick (float seconds)
  container     container name ("_vm" is the whole Docker VM from /proc)
  mem_current   cgroup memory.current, bytes (anon + page cache + kernel)
  mem_ws        working set = memory.current - inactive_file (what docker stats shows)
  mem_anon      anonymous memory, bytes (process heaps, tensors, not reclaimable)
  mem_file      page cache, bytes (reclaimable)
  mem_peak      cgroup memory.peak, bytes, since the container started
  swap_current  memory.swap.current, bytes
  cpu_cores     CPU used since the previous tick in logical cores (1.0 = one thread busy)
  throttled_us  cumulative cpu.stat throttled_usec
  oom_kill      cumulative memory.events oom_kill
  pids          pids.current
  restarts      Docker RestartCount

For "_vm" the memory columns come from /proc/meminfo of the VM: mem_current is
MemTotal - MemAvailable, mem_file is Cached, swap_current is SwapTotal - SwapFree,
cpu_cores is busy time of all VM CPUs from /proc/stat.
"""
import csv
import http.client
import json
import os
import socket
import sys
import time

ROOT = "/host/cgroup/docker"
INTERVAL = float(os.environ.get("SAMPLE_INTERVAL", "1"))


class DockerSocket(http.client.HTTPConnection):
    def __init__(self):
        super().__init__("localhost")

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.connect("/var/run/docker.sock")


def containers():
    conn = DockerSocket()
    conn.request("GET", "/containers/json")
    listed = json.loads(conn.getresponse().read())
    result = {}
    for item in listed:
        conn.request("GET", f"/containers/{item['Id']}/json")
        detail = json.loads(conn.getresponse().read())
        result[item["Id"]] = (item["Names"][0].lstrip("/"), detail.get("RestartCount", 0))
    conn.close()
    return result


def read(path, default=""):
    try:
        with open(path) as f:
            return f.read()
    except OSError:
        return default


def keyed(text):
    out = {}
    for line in text.splitlines():
        parts = line.split()
        if len(parts) == 2:
            out[parts[0]] = int(parts[1])
    return out


def number(path):
    value = read(path).strip()
    return int(value) if value.isdigit() else 0


def vm_cpu():
    fields = [int(v) for v in read("/proc/stat").splitlines()[0].split()[1:]]
    idle = fields[3] + fields[4]
    return sum(fields), idle


def main():
    out = sys.argv[1]
    names, names_at = {}, 0.0
    previous = {}
    hz = os.sysconf("SC_CLK_TCK")
    ncpu = os.cpu_count()
    new = not os.path.exists(out)
    with open(out, "a", newline="") as f:
        writer = csv.writer(f)
        if new:
            writer.writerow(["ts", "container", "mem_current", "mem_ws", "mem_anon", "mem_file", "mem_peak",
                             "swap_current", "cpu_cores", "throttled_us", "oom_kill", "pids", "restarts"])
        while True:
            now = time.time()
            if now - names_at > 5:
                try:
                    names, names_at = containers(), now
                except Exception as error:  # Docker restarting: keep the last names.
                    print("names:", error, file=sys.stderr, flush=True)
            rows = []
            for cid, (name, restarts) in names.items():
                base = os.path.join(ROOT, cid)
                if not os.path.isdir(base):
                    continue
                stat = keyed(read(base + "/memory.stat"))
                cpu = keyed(read(base + "/cpu.stat"))
                current = number(base + "/memory.current")
                usage = cpu.get("usage_usec", 0)
                last = previous.get(cid)
                cores = "" if last is None else round((usage - last[1]) / 1e6 / (now - last[0]), 3)
                previous[cid] = (now, usage)
                rows.append([round(now, 3), name, current, current - stat.get("inactive_file", 0), stat.get("anon", 0),
                             stat.get("file", 0), number(base + "/memory.peak"), number(base + "/memory.swap.current"),
                             cores, cpu.get("throttled_usec", 0), keyed(read(base + "/memory.events")).get("oom_kill", 0),
                             number(base + "/pids.current"), restarts])
            info = {}
            for line in read("/proc/meminfo").splitlines():
                key, value = line.split(":", 1)
                info[key] = int(value.split()[0]) * 1024
            total, idle = vm_cpu()
            last = previous.get("_vm")
            cores = "" if last is None else round(((total - last[1]) - (idle - last[2])) / hz / (now - last[0]), 3)
            previous["_vm"] = (now, total, idle)
            vmstat = keyed(read("/proc/vmstat"))
            rows.append([round(now, 3), "_vm", info["MemTotal"] - info["MemAvailable"], "", "", info.get("Cached", 0), "",
                         info["SwapTotal"] - info["SwapFree"], cores, "", vmstat.get("oom_kill", 0), "", ""])
            writer.writerows(rows)
            f.flush()
            time.sleep(max(0.0, INTERVAL - (time.time() - now)))


if __name__ == "__main__":
    print(f"sampling every {INTERVAL}s, {os.cpu_count()} CPUs", flush=True)
    main()
