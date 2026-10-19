#!/usr/bin/env bash
# The production stack of prod.sh, isolated for measurements: the same service
# Compose files, order and --wait, but projects aitbench-<service>, network
# aitbench_default and volumes aitbench_* (see compose/). The dev stack shares
# host ports, so it is stopped (never removed) before the stand starts.
#
#   scripts/stack.sh build  <run-dir>   build images, time each service
#   scripts/stack.sh up     <run-dir>   start in prod.sh order, time each service
#   scripts/stack.sh stop               stop the stand, keep its containers and volumes
#   scripts/stack.sh ps
#   scripts/stack.sh compose <service> <args...>
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
bench="$root/resource-benchmark"
env="$bench/bench.env"
services='postgresql s3 course-graph llm-gateway backend frontend caddy'

compose() {
  local service=$1
  shift
  docker compose -p "aitbench-$service" --env-file "$env" \
    -f "$root/services/$service/docker-compose.yaml" \
    -f "$bench/compose/network.yaml" -f "$bench/compose/volumes.yaml" "$@"
}
mark() { [ -n "${run:-}" ] && echo "$(date +%s.%N),$1,$2" >>"$run/events.csv" || true; }
timed() { # label command...
  local started ended
  started=$(date +%s.%N)
  "${@:2}"
  ended=$(date +%s.%N)
  echo "$1,$(awk "BEGIN { printf \"%.1f\", $ended - $started }")" | tee -a "$run/timings.csv"
}

cmd=${1:-ps}
run=${2:-}
case $cmd in
build)
  mkdir -p "$run"
  for service in $services; do compose "$service" config --quiet; done
  for service in $services; do
    mark build_start "$service"
    timed "build:$service" compose "$service" build
    mark build_end "$service"
  done
  ;;
up)
  mkdir -p "$run"
  docker network inspect aitbench_default >/dev/null 2>&1 || docker network create aitbench_default >/dev/null
  for service in $services; do
    mark up_start "$service"
    timed "up:$service" compose "$service" up -d --wait --wait-timeout 900
    mark up_end "$service"
  done
  ;;
stop)
  for service in $(echo $services | tr ' ' '\n' | tac); do compose "$service" stop; done
  ;;
ps)
  docker ps -a --filter name=aitbench- --format '{{.Names}}\t{{.Status}}\t{{.Image}}'
  ;;
compose)
  compose "${@:2}"
  ;;
esac
