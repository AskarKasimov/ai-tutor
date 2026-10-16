#!/bin/sh
set -eu
cd "$(dirname "$0")/../../.."
base="${1:-}"
[ -n "$base" ] || { echo "No base ref; first contract version is baseline"; exit 0; }
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
if ! git show "$base:services/shared/proto/llmgateway/v1/gateway.proto" > "$tmp/gateway.proto" 2>/dev/null; then
  echo "No contract in base ref; first contract version is baseline"; exit 0
fi
mkdir -p "$tmp/old/llmgateway/v1"
mv "$tmp/gateway.proto" "$tmp/old/llmgateway/v1/gateway.proto"
protoc -I "$tmp/old" --descriptor_set_out="$tmp/old.desc" "$tmp/old/llmgateway/v1/gateway.proto"
protoc -I services/shared/proto --descriptor_set_out="$tmp/new.desc" services/shared/proto/llmgateway/v1/gateway.proto
(cd services/shared && GOWORK=off go run ./tools/check_compat.go "$tmp/old.desc" "$tmp/new.desc")
