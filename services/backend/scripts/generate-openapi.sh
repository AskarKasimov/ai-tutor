#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
openapi_tmp=$(mktemp -d)
trap 'rm -rf "$openapi_tmp"' EXIT

go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc6 init --v3.1 --parseInternal --requiredByDefault \
  --generalInfo cmd/api/main.go --outputTypes json --output "$openapi_tmp" --quiet

# Normalize Swag's legacy file/nullable schemas and JSON errors on WAV operations.
go run github.com/mikefarah/yq/v4@v4.54.1 -P -o=yaml '
  del(.externalDocs) |
  (.. | select(has("type") and .type == "file")) |= (.type = "string" | .format = "binary") |
  (.. | select(has("x-nullable") and .["x-nullable"] == true)) |= (.type = [.type, "null"] | del(.["x-nullable"])) |
  (.. | select(has("properties") and .type == "object")).additionalProperties = false |
  (.. | select(has("content") and .content.["audio/wav"].schema.["$ref"] == "#/components/schemas/fault.Error")).content |= {"application/json": .["audio/wav"]}
' "$openapi_tmp/swagger.json" > "$openapi_tmp/implemented.yaml"

# The single contract also contains planned operations. Refresh implemented
# operations already in that contract; retain the reviewed design and omit
# routes deliberately excluded from the target API.
go run ./scripts/merge-openapi.go ../../api/openapi.yaml "$openapi_tmp/implemented.yaml" "$openapi_tmp/openapi.yaml"
mv "$openapi_tmp/openapi.yaml" ../../api/openapi.yaml
