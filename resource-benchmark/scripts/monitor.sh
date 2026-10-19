#!/usr/bin/env bash
# Start or stop the two samplers of one measurement run.
#
#   scripts/monitor.sh start <run-dir>   containers + VM, Windows host with GPU
#   scripts/monitor.sh stop  <run-dir>
#
# Raw files in <run-dir>: containers.csv (sampler.py), host.csv
# (host-sampler.ps1, with GPU columns), events.csv (marks written by the
# scenario scripts).
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
cmd=$1
dir=$2
mkdir -p "$dir"
win() { cygpath -w "$1"; }

case $cmd in
start)
  MSYS_NO_PATHCONV=1 docker run -d --name aitbench-sampler --privileged --cgroupns=host --pid=host \
    -v /sys/fs/cgroup:/host/cgroup:ro -v /var/run/docker.sock:/var/run/docker.sock:ro \
    -v "$(win "$here")":/scripts:ro -v "$(win "$dir")":/out \
    python:3.12-alpine python -u /scripts/sampler.py /out/containers.csv >/dev/null
  powershell.exe -NoProfile -WindowStyle Hidden -Command \
    "Start-Process powershell -WindowStyle Hidden -ArgumentList '-NoProfile','-File','$(win "$here/host-sampler.ps1")','-Out','$(win "$dir/host.csv")' -PassThru | ForEach-Object { \$_.Id } | Out-File -Encoding ascii '$(win "$dir/host.pid")'"
  [ -f "$dir/events.csv" ] || echo 'ts,event,detail' >"$dir/events.csv"
  echo "monitoring -> $dir"
  ;;
stop)
  docker rm -f aitbench-sampler >/dev/null 2>&1 || true
  for f in host; do
    [ -f "$dir/$f.pid" ] && powershell.exe -NoProfile -Command "Stop-Process -Id $(tr -d '\r\n\357\273\277' <"$dir/$f.pid") -Force -ErrorAction SilentlyContinue"
    rm -f "$dir/$f.pid"
  done
  echo "stopped"
  ;;
esac
