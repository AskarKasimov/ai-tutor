# protobuf-контракты LLM gateway

`proto/llmgateway/v1/gateway.proto` — источник сгенерированных Go client и server stubs в `gen/go` и Python stubs в `gen/python` (их использует `services/course-graph`). Путь Go-модуля: `github.com/AskarKasimov/ai-tutor/services/shared`. Gateway и backend подключают его локальным `replace` на соседний модуль `../shared` (gateway собирается с `GOWORK=off`); пока модуль не опубликован, для отдельной сборки потребителя нужны оба каталога.

Из корня репозитория `services/shared/tools/generate.sh services/shared/gen` генерирует Go- и Python-код, а `services/shared/tools/generate.sh --check` сравнивает свежую генерацию с закоммиченной, не меняя файлы. Python-интерпретатор с `grpcio-tools` задаёт переменная `PYTHON` (по умолчанию `python3`). Закреплённые версии компилятора и плагинов — в `tools/versions.env`; Go-плагины устанавливаются командами `go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10` и `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1`. Компилятор должен выводить `libprotoc 35.0`. Сгенерированные Go-файлы коммитятся и вручную не редактируются.

Совместимость с контрактом базовой ветки проверяет `services/shared/tools/check-compat.sh <base-ref>`.

Для Python нужны `grpcio-tools` и `protobuf` версий из `tools/versions.env` (`pip install grpcio-tools==1.75.1 protobuf==6.32.1`); сгенерированный код требует в рантайме `grpcio>=1.75.1` и `protobuf>=6.32.1`. Пакет импортируется как `llmgateway.v1`; клиент — `services/course-graph/course_graph/llm/client.py`.
