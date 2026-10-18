# LLM Gateway

Отдельный Go-сервис для серверных Chat Completions. Он принимает запросы по HTTP и gRPC, ставит их в долговечную очередь Redpanda и балансирует доступные слоты. Инстанс обслуживает запросы своего и более низких уровней сложности; обычные запросы более высокого уровня имеют приоритет на его слотах. `urgent` имеет приоритет над обычными запросами сквозь все уровни. Полный ответ отдаётся после сохранения результата.

## Локальный запуск

1. `./dev.sh` и `./prod.sh` копируют `config.example.json` в `config.json`, если локального файла ещё нет. При запуске Compose напрямую скопируйте его вручную. Уже существующий `config.json` скрипты сохраняют; он исключён из Git.
2. Укажите проверенные `base_url`, upstream `model`, контекстное окно и поддерживаемые параметры для каждого инстанса. Пулы сложности можно разделять; количество инстансов не ограничено.
3. При необходимости укажите upstream токены непосредственно в локальном `config.json` (пустое значение отключает upstream Bearer auth). Этот файл исключён из Git и не должен попадать в репозиторий.
4. `models.json` (в Git, зашит в образ как `/etc/llm-gateway/models.json`; другой путь — `GATEWAY_MODELS`) хранит правила по имени upstream-модели: какие поля добавить в тело запроса для уровня `reasoning_effort`. Сейчас там Qwen3.6 — на `low` `{"chat_template_kwargs": {"enable_thinking": false}}`: Qwen3 игнорирует `reasoning_effort` и думал ~40 с на вызов, с правилом отвечает за ~1 с. Правило действует у всех после пересборки образа, `config.json` трогать не нужно. При необходимости инстанс в `config.json` может задать своё поле `reasoning_effort_body` — оно главнее правила модели для того же уровня. Задать `model`, `messages`, `stream`, `complexity`, `urgent` или клиентский параметр нельзя ни там, ни там: gateway не стартует.
5. Запустите `COMPOSE_PROFILES=gateway docker compose up --build` из корня репозитория. Одноброкерная Redpanda нужна только для разработки и не защищает от потери диска.

В production разверните три Redpanda broker с replication factor 3. Создайте `llm.requests.v1` с одной partition, replication factor 3, retention не меньше 7 суток и `max.message.bytes` не меньше 2 MiB; `llm.state.v1` — с одной partition, replication factor 3, `cleanup.policy=compact`, без time retention актуальных записей и `max.message.bytes` не меньше 40 MiB. Топиковые пределы должны вместить payload запроса и полный ответ; Redpanda допускает задать `max.message.bytes` отдельно для topic. В каждый момент времени должен работать только один активный процесс gateway.

## Контракт

`POST /v1/chat/completions` требует `messages` и `complexity` (`low`, `medium`, `high`). Gateway не проверяет клиентский bearer-токен: в Compose он доступен сервисам внутренней сети, а опубликованный порт привязан к `127.0.0.1`. Необязательные поля и ошибки описаны в [openapi.yaml](openapi.yaml). `model` и streaming запрещены. `urgent` по умолчанию `false`; `Idempotency-Key` дедуплицируется в пределах внутреннего клиента gateway.

Upstream-токены читаются из локального JSON-конфига; клиентских секретов нет. Gateway не пишет содержимое диалогов в логи. Внутренние адреса и секреты не возвращаются клиенту.

## Состояние v1

Consumer транзакционно сохраняет `queued` вместе с входным offset. Состояние `running` коммитится до вызова модели; `succeeded`/`failed` — до ответа ожидающему HTTP или gRPC-клиенту. При восстановлении незавершённый `running` становится `execution_interrupted` без повторной генерации. Уточните compatibility JSON mode, `reasoning_effort`, токенизатор и ограничения размера на реальных endpoint.

Контекст пока оценивается консервативно по UTF-8 размеру текста (байты/2) плюс резерв 8 токенов на сообщение. Это не tokenizer конкретной модели и может отвергать допустимые запросы; подключите адаптерный tokenizer до production-подключения. `/metrics` публикует состояние очереди и инстансов, transport/request counters, задержки и карту маршрутов. `gateway_route_eligible` показывает, какие инстансы с какими моделями допустимы для каждой входной сложности по конфигурации пулов; `gateway_route_selections_total` считает реальные назначения scheduler, включая повторные попытки. Labels содержат только complexity, tier, instance и model, без адресов upstream и секретов.

## Grafana и Prometheus

При профиле `gateway` вместе с gateway запускаются Prometheus и Grafana. Prometheus каждые 15 секунд собирает `/metrics`; Grafana автоматически подключает источник и дашборд `LLM Gateway · Routing`. Дашборд показывает настроенные маршруты, фактические назначения по моделям и сложности, очередь, занятые слоты, ожидаемую длительность вызова каждого инстанса (`gateway_instance_expected_seconds`) и ошибки.

Среди свободных совместимых инстансов планировщик выбирает тот, что ответит быстрее: сначала наименьшая загрузка, затем наименьшее скользящее среднее длительности upstream-вызовов (успехи и таймауты, α = 0,3; инстанс без замеров пробуется первым), затем менее мощный уровень и round-robin. Инстанс, ответивший `429`, `5xx` или упавший на транспорте, на `Retry-After` (без него — 15 с) уходит после исправных. Запрос `low` поэтому идёт на быструю свободную модель, а не всегда на самую слабую. Оценки живут в памяти и после перезапуска собираются заново. Подробности — в [спецификации](../../documents/llm-gateway-spec.md), раздел 5. Grafana опубликована на `3000:3000` для доступа по IP хоста; в прототипе включён анонимный доступ с ролью Admin. Prometheus доступен только внутри Compose-сети.

## Проверки

Из каталога сервиса запускаются `go test -race ./... -count=1`, `go vet ./...` и `go build ./cmd/llm-gateway`. Полный integration-тест очереди, сохранённого результата и replay после перезапуска запускается при заданном `TEST_KAFKA_BROKERS` и доступной Redpanda; CI поднимает Redpanda сам.


### Контракт и проверки

`openapi.yaml` генерируется из аннотаций и типов Go командой `bash scripts/generate-openapi.sh`. CI повторяет генерацию и проверяет, что файл уже актуален. Локальный конфиг upstream — `config.json`; он игнорируется Git.


### Dev Compose и VS Code preview

В `.env` задано `COMPOSE_PROFILES=swagger,gateway`, поэтому обычный `./dev.sh` запускает gateway вместе с Redpanda. API опубликован только на `127.0.0.1:8010`. `openapi.yaml` использует `https://localhost:8443/llm-gateway`; Caddy проксирует этот путь к gateway и разрешает CORS только для VS Code webview origins, чтобы работали preview и Try it.

## gRPC

`llmgateway.v1.Gateway/Complete` слушает `llm-gateway:8011` внутри Docker; protobuf-контракт — в [shared](../shared/README.md), HTTP-эндпоинт и OpenAPI остаются доступны. Через Caddy используется тот же HTTPS-хост и порт, что у сайта (например, `localhost:8443`) с доверенным CA проекта; путь — `/llmgateway.v1.Gateway/Complete`, префикс к адресу клиента не добавляется. Начальная metadata `idempotency-key` делит пространство ключей с HTTP, `x-request-id` возвращается в metadata ответа. Отмена последнего ожидающего клиента останавливает задание в очереди или у модели; повтор с тем же ключом возвращает сохранённую отмену.

`COMPOSE_PROFILES=gateway docker compose up --build` включает оба маршрута gateway в Caddy, `COMPOSE_PROFILES= docker compose up --build` — отключает; после смены значения пересоздайте proxy. `COMPOSE_PROFILES=swagger,gateway` дополнительно включает документацию. У gRPC-порта нет проброса на хост; gRPC health доступен внутри Docker, Caddy публикует только путь сервиса Gateway.

Backend (Geek, перевод OCR) ходит в gateway по gRPC (`BACKEND_LLM_GATEWAY_ADDR`), `services/course-graph` — тоже по gRPC (`COURSE_GRAPH_LLM_GATEWAY_ADDR`).
