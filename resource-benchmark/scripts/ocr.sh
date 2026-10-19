#!/usr/bin/env bash
# Independent OCR runs, each in a fresh container of the bench course-graph image
# with the production limits (8 GiB, 4 OCR threads), the bench model volume and
# no service OCR cache. The backend and course-graph service are not involved.
#
#   scripts/ocr.sh <gpu|cpu> <runs> <out-dir> <pdf>...
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
device=$1 runs=$2 out=$3
shift 3
image=${OCR_IMAGE:-aitbench-course-graph-course-graph}
[ "$device" = cpu ] && image=${OCR_IMAGE:-aitbench-course-graph-cpu-course-graph}
models=${OCR_MODELS:-aitbench-course-graph_course_graph_models}
mkdir -p "$out"
docs=()
for pdf in "$@"; do docs+=("/docs/$(basename "$pdf")"); done
gpus=()
[ "$device" = gpu ] && gpus=(--gpus all)
for run in $(seq 1 "$runs"); do
  name="aitbench-ocr-$device-$run"
  echo "$(date +%s.%N),ocr_run_start,$device-$run" >>"$out/events.csv"
  MSYS_NO_PATHCONV=1 docker run --rm --name "$name" "${gpus[@]}" --memory 8g \
    -e COURSE_GRAPH_DEVICE="$device" -e COURSE_GRAPH_OCR_THREADS=4 -e OMP_NUM_THREADS=4 -e MKL_NUM_THREADS=4 \
    -e HF_HUB_OFFLINE=1 -e HF_HUB_DISABLE_XET=1 -e PYTHONPATH=/app \
    -v "$models":/root/.cache/huggingface \
    -v "$(cygpath -w "$root/documents/materials")":/docs:ro \
    -v "$(cygpath -w "$here")":/bench:ro -v "$(cygpath -w "$out")":/out \
    "$image" uv run --no-sync python /bench/ocr_bench.py "/out/$device-run$run.json" "${docs[@]}" \
    2>"$out/$device-run$run.stderr.log" | tee -a "$out/$device-run$run.stdout.log"
  echo "$(date +%s.%N),ocr_run_end,$device-$run" >>"$out/events.csv"
done
