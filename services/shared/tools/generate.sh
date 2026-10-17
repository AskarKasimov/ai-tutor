#!/bin/sh
set -eu
cd "$(dirname "$0")/../../.."
. services/shared/tools/versions.env
PYTHON="${PYTHON:-python3}"
[ "$(protoc --version)" = "libprotoc $PROTOC_VERSION" ] || { echo "нужен protoc $PROTOC_VERSION" >&2; exit 1; }
[ "$(protoc-gen-go --version)" = "protoc-gen-go $PROTOC_GEN_GO_VERSION" ] || { echo "нужен protoc-gen-go $PROTOC_GEN_GO_VERSION" >&2; exit 1; }
[ "$(protoc-gen-go-grpc --version)" = "protoc-gen-go-grpc ${PROTOC_GEN_GO_GRPC_VERSION#v}" ] || { echo "нужен protoc-gen-go-grpc $PROTOC_GEN_GO_GRPC_VERSION" >&2; exit 1; }
[ "$("$PYTHON" -c 'from importlib.metadata import version; print(version("grpcio-tools"))' 2>/dev/null)" = "$PYTHON_GRPCIO_TOOLS_VERSION" ] || { echo "нужен grpcio-tools $PYTHON_GRPCIO_TOOLS_VERSION в $PYTHON" >&2; exit 1; }
generate() {
  out="$1"
  mkdir -p "$out/go" "$out/python"
  protoc -I services/shared/proto --go_out="$out/go" --go_opt=module=github.com/AskarKasimov/ai-tutor/services/shared/gen/go --go-grpc_out="$out/go" --go-grpc_opt=module=github.com/AskarKasimov/ai-tutor/services/shared/gen/go services/shared/proto/llmgateway/v1/gateway.proto
  "$PYTHON" -m grpc_tools.protoc -I services/shared/proto --python_out="$out/python" --grpc_python_out="$out/python" services/shared/proto/llmgateway/v1/gateway.proto
}
case "${1:-}" in
  --check)
    tmp=$(mktemp -d)
    trap 'rm -rf "$tmp"' EXIT
    generate "$tmp"
    diff -ru services/shared/gen/go "$tmp/go"
    diff -ru -x __pycache__ services/shared/gen/python "$tmp/python"
    ;;
  "")
    echo "использование: $0 <каталог-вывода>|--check" >&2
    exit 2
    ;;
  *) generate "$1" ;;
esac
