#!/usr/bin/env bash
set -euo pipefail

# scripts/recover-failed-jobs.sh
# Admin recovery script for failed/stalled course-graph, OCR, and cleaning jobs.
# Delegates to services/backend/scripts/recover-failed-jobs.sh.

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
exec "$DIR/services/backend/scripts/recover-failed-jobs.sh" "$@"
