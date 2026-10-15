#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc6 init --v3.1 --parseInternal \
  --generalInfo cmd/llm-gateway/main.go --outputTypes json --output "$out" --quiet
go run github.com/mikefarah/yq/v4@v4.54.1 -P -o=yaml '
  del(.externalDocs) |
  .servers = [{"url": "http://localhost:8010"}] |
  .components.schemas["gateway.OpenAPIRequest"].additionalProperties = false |
  .components.schemas["gateway.OpenAPIRequest"].properties.messages.minItems = 1 |
  .components.schemas["gateway.Message"].additionalProperties = false |
  .components.schemas["gateway.Message"].properties.role.enum = ["system", "user", "assistant"] |
  .components.schemas["gateway.OpenAPIRequest"].properties.stop = {"oneOf": [{"type": "string"}, {"type": "array", "minItems": 1, "maxItems": 4, "items": {"type": "string"}}]} |
  .components.schemas["gateway.OpenAPIRequest"].properties.response_format.required = ["type"] |
  .components.schemas["gateway.OpenAPIRequest"].properties.response_format.properties.type.type = "string" |
  .components.schemas["gateway.OpenAPIRequest"].properties.response_format.properties.type.enum = ["text", "json_object"] |
  .components.schemas["gateway.OpenAPIRequest"].properties.response_format.additionalProperties = false |
  .components.schemas["gateway.OpenAPIRequest"].properties.temperature.minimum = 0 |
  .components.schemas["gateway.OpenAPIRequest"].properties.temperature.maximum = 2 |
  .components.schemas["gateway.OpenAPIRequest"].properties.top_p.minimum = 0 |
  .components.schemas["gateway.OpenAPIRequest"].properties.top_p.maximum = 1 |
  .components.schemas["gateway.OpenAPIRequest"].properties.presence_penalty.minimum = -2 |
  .components.schemas["gateway.OpenAPIRequest"].properties.presence_penalty.maximum = 2 |
  .components.schemas["gateway.OpenAPIRequest"].properties.frequency_penalty.minimum = -2 |
  .components.schemas["gateway.OpenAPIRequest"].properties.frequency_penalty.maximum = 2 |
  .components.schemas["gateway.OpenAPIRequest"].properties.n.enum = [1] |
  .components.schemas["gateway.OpenAPIRequest"].properties.stream.enum = [false] |
  .components.schemas["gateway.OpenAPIRequest"].properties.max_tokens.minimum = 1 |
  .components.schemas["gateway.OpenAPIRequest"].properties.max_tokens."x-mutually-exclusive" = ["max_completion_tokens"] |
  .components.schemas["gateway.OpenAPIRequest"].properties.max_completion_tokens.minimum = 1 |
  .components.schemas["gateway.OpenAPIRequest"].properties.max_completion_tokens."x-mutually-exclusive" = ["max_tokens"]
' "$out/swagger.json" > openapi.yaml
