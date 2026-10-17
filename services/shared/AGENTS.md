# Правила shared

Дополняют [корневой AGENTS.md](../../AGENTS.md) для `services/shared`. Генерация и версии инструментов — в [README](README.md); решения — в [gRPC-спецификации](../../documents/llm-gateway-grpc-spec.md), раздел 2.

- Единственный источник контракта — `proto/`. Код в `gen/go` и `gen/python` генерируйте из корня командой `services/shared/tools/generate.sh services/shared/gen` и коммитьте; вручную не правьте.
- Версии `protoc` и плагинов меняйте только в `tools/versions.env`.
- Совместимость: номера и имена удалённых полей резервируйте; смысл существующих полей не меняйте. Несовместимое изменение — новый package `llmgateway.v2`.
- В shared только контракты и сгенерированный код: без планировщика, конфигурации моделей, доступа к Redpanda и типов backend. Shared не импортирует backend и gateway.
- Не копируйте `.proto` в сервисы-потребители: Go-сервисы подключают модуль через `replace ../shared`, `services/course-graph` — каталог `gen/python` (pytest `pythonpath`, в образ — из корневого контекста сборки).
- Проверки из корня: `services/shared/tools/generate.sh --check`, `services/shared/tools/check-compat.sh <base-sha>`, `cd services/shared && GOWORK=off go test ./...`. После изменения контракта проверьте backend и gateway их командами.
