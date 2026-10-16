# LLM Gateway protobuf contracts

`proto/llmgateway/v1/gateway.proto` is the source for the generated Go client and server stubs in `gen/go`. Go module path: `github.com/AskarKasimov/ai-tutor/services/shared`. Gateway builds with `GOWORK=off` and a local `replace` to the sibling `../shared` module; standalone builds of a consumer need both directories until the module is published.

From the repository root, run `services/shared/tools/generate.sh services/shared/gen/go` to generate Go code or `services/shared/tools/generate.sh --check` to compare fresh output without changing committed files. The pinned compiler and plugin versions are in `tools/versions.env`; install the Go plugins with `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10` and `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1`. The compiler must report `libprotoc 35.0`. Generated Go files are committed and must not be edited manually.

For Python, pin `grpcio-tools==1.75.1` and `protobuf==6.32.1`, then generate using the same include root:

```sh
python -m grpc_tools.protoc -I services/shared/proto --python_out=generated --grpc_python_out=generated services/shared/proto/llmgateway/v1/gateway.proto
```

The repository does not publish a Python package or client example yet.
