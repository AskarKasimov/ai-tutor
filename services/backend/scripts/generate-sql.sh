#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate -f sqlc.yaml
