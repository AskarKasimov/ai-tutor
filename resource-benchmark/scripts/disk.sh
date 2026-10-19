#!/usr/bin/env bash
# Disk usage of the bench stand, written to <out>.json lines and <out>.txt.
#
#   scripts/disk.sh <out-prefix>
#
# apparent = logical size of the files (du --apparent-size); allocated = blocks
# really taken on the VM disk (du). Images: docker system df -v gives each
# image's shared and unique size, so shared layers are counted once.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
out=$1
mkdir -p "$(dirname "$out")"
: >"$out.txt"
du_vm() { # label path-in-VM...
  local label=$1
  shift
  MSYS_NO_PATHCONV=1 docker run --rm -v /var/lib/docker:/d:ro alpine sh -c \
    "for p in $*; do printf '%s\t%s\t%s\n' \"\$p\" \"\$(du -sb \"/d/\$p\" 2>/dev/null | cut -f1)\" \"\$(du -sk \"/d/\$p\" 2>/dev/null | cut -f1)\"; done" |
    while IFS=$'\t' read -r path apparent kib; do
      printf '{"item":"%s","path":"%s","apparent_bytes":%s,"allocated_bytes":%s}\n' "$label" "$path" "${apparent:-0}" "$(( ${kib:-0} * 1024 ))"
    done
}
{
  # Sources: the working tree without .git and without git-ignored design refs.
  src=$(cd "$root" && git ls-files -z | xargs -0 du -cb 2>/dev/null | tail -1 | cut -f1)
  printf '{"item":"sources (git ls-files)","apparent_bytes":%s}\n' "$src"
  printf '{"item":".git","apparent_bytes":%s}\n' "$(du -sb "$root/.git" | cut -f1)"
  printf '{"item":"documents/materials (course PDFs)","apparent_bytes":%s}\n' "$(du -sb "$root/documents/materials" | cut -f1)"
  for v in aitbench_postgres_data aitbench_task_audio_data aitbench-course-graph_course_graph_models \
           aitbench-course-graph_course_graph_data aitbench_redpanda_data aitbench_caddy_data; do
    du_vm "volume $v" "volumes/$v/_data"
  done
  # Container logs (json-file driver) of the stand.
  for id in $(docker ps -aq --no-trunc --filter name=aitbench-); do
    name=$(docker inspect --format '{{.Name}}' "$id" | tr -d /)
    du_vm "log $name" "containers/$id/$id-json.log"
  done
} | tee -a "$out.jsonl"
{
  echo "== docker system df -v (images and build cache)"
  docker system df -v
  echo "== host disk"
  df -h /c
} >>"$out.txt" 2>&1
# PostgreSQL and S3 content split.
docker exec aitbench-postgresql-db-1 psql -U ai_tutor -d ai_tutor -tA -F, -c \
  "select 'db_total', pg_database_size('ai_tutor') union all
   select relname, pg_total_relation_size(c.oid) from pg_class c join pg_namespace n on n.oid=c.relnamespace
   where n.nspname='public' and c.relkind='r' order by 2 desc limit 12" >"$out.postgres.csv" 2>/dev/null || true
MSYS_NO_PATHCONV=1 docker run --rm -v aitbench_task_audio_data:/s:ro alpine sh -c \
  'for d in /s/*/*; do [ -d "$d" ] && printf "%s,%s,%s\n" "$d" "$(du -sb "$d" | cut -f1)" "$(find "$d" -type f | wc -l)"; done' \
  >"$out.s3.csv" 2>/dev/null || true
echo "disk -> $out.*"
