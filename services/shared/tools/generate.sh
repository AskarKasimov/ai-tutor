#!/bin/sh
set -eu
cd "$(dirname "$0")/../../.."
. services/shared/tools/versions.env
[ "$(protoc --version)" = "libprotoc $PROTOC_VERSION" ] || { echo "protoc $PROTOC_VERSION required" >&2; exit 1; }
[ "$(protoc-gen-go --version)" = "protoc-gen-go $PROTOC_GEN_GO_VERSION" ] || { echo "protoc-gen-go $PROTOC_GEN_GO_VERSION required" >&2; exit 1; }
[ "$(protoc-gen-go-grpc --version)" = "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION#v}" ] || { echo "protoc-gen-go-grpc $PROTOC_GEN_GO_GRPC_VERSION required" >&2; exit 1; }
generate() {
  out="$1"
  mkdir -p "$out"
  protoc -I services/shared/proto --go_out="$out" --go_opt=module=github.com/AskarKasimov/ai-tutor/services/shared/gen/go --go-grpc_out="$out" --go-grpc_opt=module=github.com/AskarKasimov/ai-tutor/services/shared/gen/go services/shared/proto/llmgateway/v1/gateway.proto
}
case "${1:-}" in
  --check)
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    generate "$tmp"
    diff -ru services/shared/gen/go "$tmp"
    ;;
  "")
    echo "usage: $0 <output-directory>|--check" >&2
    exit 2
    ;;
  *) generate "$1" ;;
esac
