# LLM Gateway

Отдельный Go-сервис для серверных Chat Completions. Он принимает запросы по HTTP и gRPC, ставит их в долговечную очередь Redpanda и балансирует доступные слоты. Инстанс обслуживает запросы своего и более низких уровней сложности; обычные запросы более высокого уровня имеют приоритет на его слотах. `urgent` имеет приоритет над обычными запросами сквозь все уровни. Полный ответ отдаётся после сохранения результата.

## Локальный запуск и конфигурация

Основной запуск — ./dev.sh -d из корня после настройки .env с секретами БД и S3. Gateway всегда входит в общий dev stack и читает полный YAML из config/dev.yaml; в Docker Compose указатель GATEWAY_CONFIG указывает на смонтированный файл. Изменение YAML через atomic-save вызывает Air restart gateway-процесса в том же контейнере. Очередь Redpanda сохраняется, незавершённые запросы обрабатываются штатным механизмом восстановления gateway.

Файл содержит routes, pools, routing_profiles, instances и model_rules. Для общего приложения обязательны профили learning-assessment, learning-tutor и learning-taskgen; каждый должен иметь маршрут на инстанс из pools.high. У standalone-конфигурации required_routing_profiles: [] сохраняет необязательность этих требований.

По согласованному решению владельца upstream model tokens пока допустимы в YAML и предназначены для Git. Пустая строка отключает Bearer auth. Не печатайте содержимое токенов в логах, тестовых выводах и отчётах. Настройки gateway из прежних JSON файлов больше не загружаются. Loader проверяет типы YAML-скаляров: целые числа, строки и булевы значения не принимаются с coercion; свободные JSON body сохраняют свои типы. YAML merge keys (`<<`) не поддерживаются.

Правила модели находятся в model_rules.models.<model>.reasoning_effort_body того же YAML. Для Qwen3.6 reasoning отключён на low и medium через chat_template_kwargs.enable_thinking: false, поскольку Qwen игнорирует reasoning_effort. Правило инстанса имеет приоритет над правилом модели для того же уровня. Поля model, messages, stream, complexity, urgent и разрешённые клиентские параметры запрещены в extra body.

Runtime image остаётся для сборки и smoke-проверок. При запуске бинарника или runtime image смонтируйте YAML явно и передайте GATEWAY_CONFIG=/path/to/dev.yaml; YAML не встраивается в образ.

Для числовых лимитов отсутствие поля включает прежнее значение по умолчанию;
явный `0` сохраняется и отклоняется валидацией для положительных лимитов.
Нулевой `instances[].timeout` по-прежнему наследует общий `upstream_timeout`.

## Контракт

`POST /v1/chat/completions` требует `messages` и `complexity` (`low`, `medium`, `high`). Gateway не проверяет клиентский bearer-токен: в Compose он доступен сервисам внутренней сети, локальный HTTP-порт привязан к `127.0.0.1`, а Caddy проксирует HTTP и gRPC на настроенном HTTPS-порту. Необязательные поля и ошибки описаны в [openapi.yaml](openapi.yaml). `model` и streaming запрещены. `urgent` по умолчанию `false`; `Idempotency-Key` дедуплицируется в пределах внутреннего клиента gateway.

Upstream-токены читаются из runtime YAML; клиентских секретов нет. Gateway не пишет содержимое диалогов в логи. Внутренние адреса и секреты не возвращаются клиенту.

## Состояние v1

Consumer транзакционно сохраняет `queued` вместе с входным offset. Состояние `running` коммитится до вызова модели; `succeeded`/`failed` — до ответа ожидающему HTTP или gRPC-клиенту. При восстановлении незавершённый `running` становится `execution_interrupted` без повторной генерации. `timeout` и `upstream_timeout` ограничивают HTTP-клиент и закрывают upstream-запрос, но сами по себе не подтверждают остановку модели. Если конкретный upstream проверен на отмену генерации при disconnect, задайте ему `abort_on_disconnect: true`: при gateway timeout или восстановлении после рестарта его неопределённый слот освобождается. Проверку повторяйте после смены версии модели-сервера или промежуточного proxy. Если это поведение не подтверждено, слот остаётся занят. Альтернативный таймер `server_execution_limit` допустим только для серверного предела полного выполнения, включая очередь, который действует независимо от disconnect; `0s` отключает этот таймер. Для текущих проверенных endpoint таймер отключён. Уточните compatibility JSON mode, `reasoning_effort`, токенизатор и ограничения размера на реальных endpoint.

Контекст пока оценивается консервативно по UTF-8 размеру текста (байты/2) плюс резерв 8 токенов на сообщение. Это не tokenizer конкретной модели и может отвергать допустимые запросы; подключите адаптерный tokenizer до production-подключения. `/metrics` публикует состояние очереди и инстансов, transport/request counters, задержки и карту маршрутов. `gateway_route_eligible` показывает, какие инстансы с какими моделями допустимы для каждой входной сложности по конфигурации пулов; `gateway_route_selections_total` считает реальные назначения scheduler, включая повторные попытки. Labels содержат только complexity, tier, instance и model, без адресов upstream и секретов.

## Grafana и Prometheus

В standalone service compose профиль `gateway` запускает Prometheus и Grafana вместе с gateway. В корневом едином Compose мониторинг включается профилем `observability`; сам LLM gateway запускается всегда как обязательная зависимость приложения. Prometheus каждые 15 секунд собирает `/metrics`; Grafana автоматически подключает источник и дашборд `LLM Gateway · Routing`. Дашборд показывает настроенные маршруты, фактические назначения по моделям и сложности, очередь, занятые слоты, ожидаемую длительность вызова каждого инстанса (`gateway_instance_expected_seconds`) и ошибки.

Среди свободных совместимых инстансов планировщик выбирает тот, что ответит быстрее: сначала наименьшая загрузка, затем наименьшее скользящее среднее длительности upstream-вызовов (успехи и таймауты, α = 0,3; инстанс без замеров пробуется первым), затем менее мощный уровень и round-robin. Инстанс, ответивший `429`, `5xx` или упавший на транспорте, на `Retry-After` (без него — 15 с) уходит после исправных. Запрос `low` поэтому идёт на быструю свободную модель, а не всегда на самую слабую. Оценки живут в памяти и после перезапуска собираются заново. Подробности — в [спецификации](../../documents/llm-gateway-spec.md), раздел 5. Grafana опубликована на `3000:3000` для доступа по IP хоста; в прототипе включён анонимный доступ с ролью Admin. Prometheus доступен только внутри Compose-сети.

## Проверки

Из каталога сервиса запускаются `go test -race ./... -count=1`, `go vet ./...` и `go build ./cmd/llm-gateway`. Полный integration-тест очереди, сохранённого результата и replay после перезапуска запускается при заданном `TEST_KAFKA_BROKERS` и доступной Redpanda; CI поднимает Redpanda сам.


### Контракт и проверки

`openapi.yaml` генерируется из аннотаций и типов Go командой `bash scripts/generate-openapi.sh`. CI повторяет генерацию и проверяет, что файл уже актуален. Runtime-конфиг upstream — `config/dev.yaml` и хранится в Git.


### Dev Compose и VS Code preview

Gateway и Redpanda входят в dev stack; `COMPOSE_PROFILES` включает только дополнительные Swagger и observability сервисы. API опубликован только на `127.0.0.1:8010`. `openapi.yaml` использует `https://localhost:8443/llm-gateway`; Caddy проксирует этот путь к gateway и разрешает CORS только для VS Code webview origins, чтобы работали preview и Try it.

## gRPC

`llmgateway.v1.Gateway/Complete` слушает `llm-gateway:8011` внутри Docker; protobuf-контракт — в [shared](../shared/README.md), HTTP-эндпоинт и OpenAPI остаются доступны. Через Caddy используется тот же HTTPS-хост и порт, что у сайта (например, `localhost:8443`) с доверенным CA проекта; путь — `/llmgateway.v1.Gateway/Complete`, префикс к адресу клиента не добавляется. Начальная metadata `idempotency-key` делит пространство ключей с HTTP, `x-request-id` возвращается в metadata ответа. Отмена последнего ожидающего клиента останавливает задание в очереди или у модели; повтор с тем же ключом возвращает сохранённую отмену.

Gateway всегда входит в корневой Compose stack. `COMPOSE_PROFILES` включает Swagger и observability сервисы. У gRPC-порта нет проброса на хост; gRPC health доступен внутри Docker, Caddy публикует только путь сервиса Gateway.

Backend (Geek, перевод OCR) и `services/course-graph` ходят в gateway по gRPC; адреса задаются в их `config/dev.yaml`.


`routing_profiles` в `config/dev.yaml` связывает client profile IDs с разрешёнными instance IDs. Профиль фильтрует complexity pools, включая urgent; неизвестное имя отклоняется. Пустой профиль сохраняет прежнюю маршрутизацию и idempotency fingerprint. В корневом Compose обязательны `learning-assessment`, `learning-tutor` и `learning-taskgen`; после изменения mapping сохраните YAML: Air перезапустит gateway-процесс в том же контейнере. Файл `config/dev.yaml` содержит все runtime-настройки.
