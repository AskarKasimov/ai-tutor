#!/usr/bin/env bash
# Write resource-benchmark/bench.env: the production template (.env.prod.example)
# with fresh test secrets, the local RustFS and HTTPS on 127.0.0.1:8443.
# The file holds secrets of the test stand only and is git-ignored.
#
#   COURSE_GRAPH_DEVICE=gpu|cpu scripts/make-env.sh
set -euo pipefail
root=$(cd "$(dirname "$0")/../.." && pwd)
out="$root/resource-benchmark/bench.env"
secret() { od -An -N24 -tx1 /dev/urandom | tr -d ' \n'; }
set_var() { # name value
  if grep -q "^$1=" "$out"; then sed -i "s|^$1=.*|$1=$2|" "$out"; else echo "$1=$2" >>"$out"; fi
}
cp "$root/.env.prod.example" "$out"
set_var DB_PASSWORD "$(secret)"
set_var S3_ACCESS_KEY "$(secret)"
set_var S3_SECRET_KEY "$(secret)"
set_var S3_LOCAL_ENABLED true
set_var BACKEND_S3_ENDPOINT http://s3:9000
set_var BACKEND_S3_CREATE_BUCKET true
set_var PROXY_HTTPS_BIND 127.0.0.1
set_var PROXY_HTTPS_PORT 8443
set_var COURSE_GRAPH_DEVICE "${COURSE_GRAPH_DEVICE:-gpu}"
echo "written $out (COURSE_GRAPH_DEVICE=${COURSE_GRAPH_DEVICE:-gpu})"
