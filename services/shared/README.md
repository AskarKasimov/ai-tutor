# protobuf-контракты LLM gateway

`proto/llmgateway/v1/gateway.proto` — источник сгенерированных Go client и server stubs в `gen/go`. Путь Go-модуля: `github.com/AskarKasimov/ai-tutor/services/shared`. Gateway и backend подключают его локальным `replace` на соседний модуль `../shared` (gateway собирается с `GOWORK=off`); пока модуль не опубликован, для отдельной сборки потребителя нужны оба каталога.

Из корня репозитория `services/shared/tools/generate.sh services/shared/gen/go` генерирует Go-код, а `services/shared/tools/generate.sh --check` сравнивает свежую генерацию с закоммиченной, не меняя файлы. Закреплённые версии компилятора и плагинов — в `tools/versions.env`; Go-плагины устанавливаются командами `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10` и `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1`. Компилятор должен выводить `libprotoc 35.0`. Сгенерированные Go-файлы коммитятся и вручную не редактируются.

Совместимость с контрактом базовой ветки проверяет `services/shared/tools/check-compat.sh <base-ref>`.

Для Python закрепите `grpcio-tools==1.75.1` и `protobuf==6.32.1` и генерируйте с тем же include root:

```sh
python -m grpc_tools.protoc -I services/shared/proto --python_out=generated --grpc_python_out=generated services/shared/proto/llmgateway/v1/gateway.proto
```

Python-пакета и примера клиента в репозитории пока нет.
