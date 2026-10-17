#!/usr/bin/env bash
set -euo pipefail

# scripts/recover-failed-jobs.sh
# Admin recovery script for failed/stalled course-graph, OCR, and cleaning jobs.
# Conforms to spec line 97:
# - token/status CAS
# - audit logging
# - --dry-run support
# - preserves existing raw/parsed/cleaned artifacts
# - does not create duplicate material rows

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$DIR"

exec go run ./cmd/recover-failed-jobs "$@"
