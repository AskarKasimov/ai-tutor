#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../../.."
# Python: activate a venv with grpcio-tools==1.70.0 before invoking this script.
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.1
PATH="$PATH:$(go env GOPATH)/bin" protoc -I api/proto \
  --go_out=services/backend/internal/shared/llmpb --go_opt=paths=source_relative \
  --go-grpc_out=services/backend/internal/shared/llmpb --go-grpc_opt=paths=source_relative \
  api/proto/llm.proto
python -m grpc_tools.protoc -Icourse_graph/llm=api/proto \
  --python_out=services/course-graph --grpc_python_out=services/course-graph \
  course_graph/llm/llm.proto
