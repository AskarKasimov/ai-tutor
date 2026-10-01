#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
openapi_tmp=$(mktemp -d)
trap 'rm -rf "$openapi_tmp"' EXIT

go tool swag init --v3.1 --parseInternal --requiredByDefault \
  --generalInfo cmd/api/main.go --outputTypes json --output "$openapi_tmp" --quiet

# Normalize Swag's legacy file/nullable schemas and JSON errors on WAV operations.
go tool yq -P -o=yaml '
  del(.externalDocs) |
  (.. | select(has("type") and .type == "file")) |= (.type = "string" | .format = "binary") |
  (.. | select(has("x-nullable") and .["x-nullable"] == true)) |= (.type = [.type, "null"] | del(.["x-nullable"])) |
  (.. | select(has("properties") and .type == "object")).additionalProperties = false |
  (.. | select(has("content") and .content.["audio/wav"].schema.["$ref"] == "#/components/schemas/fault.Error")).content |= {"application/json": .["audio/wav"]}
' "$openapi_tmp/swagger.json" > "$openapi_tmp/backend.api.yaml"
mv "$openapi_tmp/backend.api.yaml" backend.api.yaml
