# LLM Gateway

Отдельный Go-сервис для серверных Chat Completions. Он принимает запросы по HTTP и gRPC, выбирает только из пула заданной `complexity`, ставит запросы в долговечную очередь Redpanda, балансирует доступные слоты и отдаёт полный ответ после сохранения результата.

## Локальный запуск

1. Скопируйте `config.example.json` в `config.json` (этот файл исключён из Git).
2. Укажите проверенные `base_url`, upstream `model`, контекстное окно и поддерживаемые параметры для каждого инстанса. Пулы сложности можно разделять; количество инстансов не ограничено.
3. При необходимости укажите upstream токены непосредственно в локальном `config.json` (пустое значение отключает upstream Bearer auth). Этот файл исключён из Git и не должен попадать в репозиторий.
4. Запустите `COMPOSE_PROFILES=gateway docker compose up --build` из корня репозитория. Одноброкерная Redpanda нужна только для разработки и не защищает от потери диска.

В production разверните три Redpanda broker с replication factor 3. Создайте `llm.requests.v1` с одной partition, replication factor 3, retention не меньше 7 суток и `max.message.bytes` не меньше 2 MiB; `llm.state.v1` — с одной partition, replication factor 3, `cleanup.policy=compact`, без time retention актуальных записей и `max.message.bytes` не меньше 40 MiB. Топиковые пределы должны вместить payload запроса и полный ответ; Redpanda допускает задать `max.message.bytes` отдельно для topic. В каждый момент времени должен работать только один активный процесс gateway.

## Контракт

`POST /v1/chat/completions` требует `messages` и `complexity` (`low`, `medium`, `high`). Gateway не проверяет клиентский bearer-токен: в Compose он доступен сервисам внутренней сети, а опубликованный порт привязан к `127.0.0.1`. Необязательные поля и ошибки описаны в [openapi.yaml](openapi.yaml). `model` и streaming запрещены. `urgent` по умолчанию `false`; `Idempotency-Key` дедуплицируется в пределах внутреннего клиента gateway.

Client и upstream секреты читаются из локального JSON-конфига. Gateway не пишет содержимое диалогов в логи. Внутренние адреса и секреты не возвращаются клиенту.

## Состояние v1

Consumer транзакционно сохраняет `queued` вместе с входным offset. Состояние `running` коммитится до вызова модели; `succeeded`/`failed` — до ответа ожидающему HTTP или gRPC-клиенту. При восстановлении незавершённый `running` становится `execution_interrupted` без повторной генерации. Уточните compatibility JSON mode, `reasoning_effort`, токенизатор и ограничения размера на реальных endpoint до перевода backend-клиентов.

Контекст пока оценивается консервативно по UTF-8 размеру текста (байты/2) плюс резерв 8 токенов на сообщение. Это не tokenizer конкретной модели и может отвергать допустимые запросы; подключите адаптерный tokenizer до production-подключения. `/metrics` пока публикует базовые gauges очереди и занятых слотов; расширенные метрики из спецификации (consumer lag, latency histograms, per-instance breaker/retry counters) ещё нужно добавить.

## Проверки

Из каталога сервиса запускаются `go test -race ./... -count=1`, `go vet ./...` и `go build ./cmd/llm-gateway`. Полный integration-тест очереди, сохранённого результата и replay после перезапуска запускается при заданном `TEST_KAFKA_BROKERS` и доступной Redpanda; CI поднимает Redpanda сам.


### Контракт и проверки

`openapi.yaml` генерируется из аннотаций и типов Go командой `bash scripts/generate-openapi.sh`. CI повторяет генерацию и проверяет, что файл уже актуален. Локальный конфиг upstream — `config.json`; он игнорируется Git.


### Dev Compose и VS Code preview

В `.env` задано `COMPOSE_PROFILES=swagger,gateway`, поэтому обычный `./dev.sh` запускает gateway вместе с Redpanda. API опубликован только на `127.0.0.1:8010`. `openapi.yaml` использует `https://localhost:8443/llm-gateway`; Caddy проксирует этот путь к gateway и разрешает CORS только для VS Code webview origins, чтобы работали preview и Try it.

## gRPC

`llmgateway.v1.Gateway/Complete` listens on `llm-gateway:8011` inside Docker. Its protobuf contract is in [shared](../shared/README.md); the existing HTTP endpoint and OpenAPI remain available. Through Caddy, use the same HTTPS host and port as the site, for example `localhost:8443`, with the project CA trusted. The gRPC path is `/llmgateway.v1.Gateway/Complete`; no prefix is added to the client target. The initial `idempotency-key` metadata shares the HTTP key namespace. `x-request-id` is returned as response metadata. A canceled last waiter stops queued or active work and a retry with the same key returns the saved cancellation.

`COMPOSE_PROFILES=gateway docker compose up --build` enables both Caddy gateway routes. `COMPOSE_PROFILES= docker compose up --build` disables both; recreate proxy when changing the value. `COMPOSE_PROFILES=swagger,gateway` enables docs as well. The gRPC listener has no host port mapping. The service exports gRPC health inside Docker; Caddy exposes only the Gateway service path.

Existing backend and course-graph adapters are not switched to gRPC by this change.
