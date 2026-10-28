# Dev YAML configuration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Исполнитель:** `gpt-6-luna`, reasoning effort `high`; последовательно, без делегирования. Это план на оценку, реализацию начинать после одобрения владельца.

**Goal:** Оставить актуальную dev-конфигурацию в коммитных YAML внутри сервисов, применять изменения через автоматический перезапуск процессов в существующих контейнерах, убрать неподдерживаемую prod-конфигурацию и исправить найденные расхождения.

**Architecture:** Один полный `config/dev.yaml` на backend, course-graph и gateway. Настройки приложения читаются только из YAML; env — секреты и инфраструктура. Исключение по прямому решению владельца: ключи upstream-моделей пока находятся в YAML gateway. Backend/gateway используют Air, Python — watcher; изменения YAML перезапускают процесс без пересоздания контейнера и пересборки образа. Это не hot reload состояния внутри процесса.

**Tech Stack:** Go, Air, yaml.v3; Python, Pydantic, PyYAML и watchfiles; Docker Compose; существующий Vite.

**Spec:** Решения владельца в этом чате от 27.10.2026, зафиксированные ниже. Этот документ заменяет прежний план с dev/prod, token_env и отдельным models.json.

## Согласованные решения и границы

- YAML хранится внутри сервисов и коммитится. Сейчас только dev: требование двух файлов dev/prod отменено последующим решением убрать prod.
- Env не переопределяет настройки приложения. Секреты backend/БД/S3 остаются в env. Ключи моделей gateway пока остаются в YAML, поле `token` сохраняется; `token_env` не вводить.
- Правила из `models.json` объединяются с runtime YAML gateway. Отдельного model-rules файла и example больше нет.
- Frontend сохраняет `VITE_API_BASE_URL`, существующее поведение сборки и `VITE_API_PROXY_TARGET`. Исправляется документация пересборки.
- Изменение YAML автоматически перезапускает процесс watcher-ом в том же контейнере. Прерывание активных запросов при dev restart принято владельцем.
- Prod убрать «по-лайту»: удалить `prod.sh`, `.env.prod.example`, prod YAML (если успели появиться), активные инструкции и проверки prod deployment. Сохранить Docker build/runtime stages, сборки образов и полезные smoke/E2E тесты; это поддержка артефактов сборки, а не отдельного prod-окружения.
- Сохранить root/service Compose и dev override; не переделывать всю топологию ради этой задачи. Основная точка входа — `dev.sh`; standalone dev описать в сервисных README.
- Сохранить volumes и данные; без `down -v`, сбросов БД и изменения применённых миграций.
- Сохранить учебные правила, значения настроек, лимиты, промпты и маршрутизацию LLM. HTTP/protobuf контракты не менять.
- Стратегические развилки, меняющие согласованную архитектуру или границы задачи, обсуждать с владельцем, а не решать самостоятельно. Рутинные детали реализации внутри этих границ — ответственность исполнителя.
- Коммиты — только по отдельному поручению. «Коммитный YAML» означает файл, предназначенный для Git, а не разрешение автоматически сделать commit.

## Review Focus

1. Явные `0`, `false`, `0s` сохраняют значение, а отсутствие обязательного поля приводит к ошибке (задачи 1–3).
2. Чтение Python настроек происходит до импорта тяжёлых библиотек и вычисления module-level defaults; OCR cache соответствует эффективному конфигу (задача 3).
3. Atomic-save/rename YAML виден контейнеру и watcher-у; меняется PID приложения, container ID сохраняется (задача 4).
4. Restart gateway использует прежнюю очередь и восстановление; restart OCR не обещает сохранение незавершённой in-memory работы (задачи 2–4).
5. Изменение требований prod не отключает существующие функциональные smoke/E2E проверки и не удаляет данные (задачи 5–6).

## Целевые файлы и загрузка

Создать:

```text
services/backend/config/dev.yaml
services/course-graph/config/dev.yaml
services/llm-gateway/config/dev.yaml
```

Полные YAML без include, overlays, `${VAR}` и универсальной env-интерполяции. Общий каталог config, `APP_ENV` и `APP_CONFIG_DIR` не нужны: окружение одно.

Монтировать каталоги `config/`, а не отдельные файлы, чтобы atomic-save редактора оставался видим контейнеру. В dev backend конфиг доступен в уже смонтированном `/work/backend/config/dev.yaml`, gateway — `/work/llm-gateway/config/dev.yaml`; course-graph — `/app/config/dev.yaml`. Process pointers `BACKEND_CONFIG`, `GATEWAY_CONFIG`, `COURSE_GRAPH_CONFIG` указывают на выбранный файл и позволяют CI выбрать временный fixture. Это указатели файлов, а не переопределение отдельных полей.

Runtime stages без watcher сохранить. Для прямого запуска бинарника/CLI или runtime-image документация задаёт путь к смонтированному dev YAML явно; не создавать отдельные «prod defaults».

Парсеры отвергают unknown/duplicate keys, несколько YAML-документов, пустой файл и неверные типы. Динамические словари pools/profiles/model body допускают свои ключи. Длительности — строки с единицами, например `120s`, `0s`, `24h`; Python поддерживает `ms`, `s`, `m`, `h`. Ошибки содержат путь YAML-поля, но не содержимое токенов/паролей.

### Backend: отображение env → YAML

Слева опущен префикс `BACKEND_`.

| Env | YAML |
| --- | --- |
| LISTEN_ADDRESS | server.listen_address |
| STT_URL / TTS_URL | voice.stt.url / voice.tts.url |
| TTS_SEED / TTS_CFG_VALUE / TTS_INFERENCE_TIMESTEPS | voice.tts.seed / cfg_value / inference_timesteps |
| VOICE_TIMEOUT | voice.timeout |
| MAX_UPLOAD_BYTES | uploads.max_bytes |
| S3_ENDPOINT / S3_REGION / S3_BUCKET | storage.s3.endpoint / region / bucket |
| S3_PATH_STYLE / S3_TIMEOUT / S3_CREATE_BUCKET | storage.s3.path_style / timeout / create_bucket |
| AUDIO_WORKERS / AUDIO_POLL_INTERVAL | workers.audio.count / poll_interval |
| TUTOR_ENABLED | tutor.enabled |
| TUTOR_TASK_MAX_CHARS / TUTOR_HISTORY_MAX_CHARS / TUTOR_SUMMARY_MAX_CHARS | tutor.context.task_max_chars / history_max_chars / summary_max_chars |
| COURSE_GRAPH_URL / COURSE_GRAPH_TIMEOUT | course_graph.url / timeout |
| LLM_GATEWAY_ADDR / LLM_TIMEOUT | llm_gateway.address / timeout |
| GEEK_QUESTION_LIMIT | geek.question_limit |

Env secrets: `BACKEND_DATABASE_URL`, `BACKEND_S3_ACCESS_KEY`, `BACKEND_S3_SECRET_KEY`. Compose сохраняет сборку DSN из `DB_PASSWORD` и mapping `S3_ACCESS_KEY`/`S3_SECRET_KEY`. Секретных полей backend в YAML нет. Defaults и validation bounds сохранить из текущего кода; различать пропущенное поле и explicit zero. `BACKEND_OCR_URL/TIMEOUT` не вводить в новую схему: это прежние aliases course-graph.

Старые app env загрузчики больше не используют. Их наличие в локальном `.env` не должно влиять на YAML: удалить mappings из Compose и обновить документацию миграции. Тесты явно подтверждают, что старый env не переопределяет YAML. Отдельный механизм ошибок на legacy env не добавлять: он не нужен для выполнения согласованного запрета overrides.

### Course-graph: отображение env → YAML

Слева опущен `COURSE_GRAPH_`, кроме `PORT`.

| Env | YAML |
| --- | --- |
| PORT | server.port |
| LLM_GATEWAY_ADDR / LLM_TIMEOUT | llm_gateway.address / timeout |
| EMBEDDING_MODEL / EMBEDDING_THREADS | embedding.model / threads |
| TOPIC_MAX_CHARS | topics.max_chars |
| MODEL_BOOTSTRAP / MODEL_BOOTSTRAP_ATTEMPTS | models.bootstrap.enabled / attempts |
| OCR_THREADS | ocr.threads |
| OCR_DOCUMENT_TIMEOUT | ocr.document_timeout |
| OCR_ENRICH_CODE / OCR_ENRICH_FORMULA | ocr.enrich_code / enrich_formula |
| OCR_PICTURE_TEXT / OCR_FULL_PAGE | ocr.picture_text / full_page |
| OCR_CACHE_DIR / RAPID_CACHE_DIR | cache.documents_dir / rapidocr_dir |

Сохранить defaults/границы. `COURSE_GRAPH_DEVICE`, `COURSE_GRAPH_WITH_DOCLING` остаются признаками возможностей образа и выбора Compose-варианта. `OMP_NUM_THREADS`, `MKL_NUM_THREADS`, `HF_*` остаются env сторонних библиотек; defaults OMP/MKL брать из `ocr.threads` до импорта библиотек, сохраняя явные библиотечные overrides. Embedded/local embedding model остаётся настройкой course-graph.

### Gateway: единый YAML

Перенести поля существующего runtime JSON с прежними snake_case именами, включая `instances[].token`; секретные значения не выводить в логах, тестах или отчёте. Пустой token по-прежнему означает отсутствие Bearer auth.

Добавить `model_rules` с содержимым прежнего `models.json` по схеме `model_rules.models.<model>.reasoning_effort_body`. Сохранить прежний приоритет: instance-specific правило выше общего правила модели для того же уровня. Свободные nested body сохраняют JSON-типы boolean/number/object, в частности `enable_thinking: false`.

`required_routing_profiles` перенести из root Compose env в YAML. Dev YAML общего приложения содержит `learning-assessment`, `learning-tutor`, `learning-taskgen`; standalone fixture с пустым списком сохраняет прежнюю необязательность требований. YAML не меняет сами маршруты/pools.

Удалить активное использование `GATEWAY_MODELS`, `GATEWAY_REQUIRED_ROUTING_PROFILES`, отдельного `models.json` и example. Временную приватную копию существующего локального config.json сохранить вне checkout, затем перенести эффективные настройки в YAML без потери routes/instances/tokens. Старый локальный JSON убрать из checkout после сверки; удалить его ignore-правило. Новые YAML предназначены для Git с разрешёнными владельцем ключами моделей; прочие секреты и локальный env остаются вне Git.

## Последовательность реализации

Перед изменениями прочитать корневой и сервисные AGENTS/README, backend architecture/auth и gateway HTTP/gRPC specs. Сначала реализация loaders с focused tests, затем dev wiring, CI и документация. Между этапами допустим незавершённый рабочий checkout; не объявлять запуск готовым до задачи 6.

### Задача 1: backend YAML и CLI

**Файлы:** `services/backend/internal/app/config.go`, `config_test.go`; новый `config_yaml.go`; `cmd/api/main.go`, `prepare.go`, новый `cmd/api/config_test.go`; `config/dev.yaml` внутри backend.

**Интерфейсы:** `app.LoadConfig(path string) (Config, error)` — parse + env secrets + validate. `app.LoadLLMGatewayConfig(path string) (address string, timeout time.Duration, err error)` использует общий parser, валидирует нужную CLI секцию без требования DB/S3 секретов. Общий CLI helper разрешает BACKEND_CONFIG.

- [ ] Добавить падающие table tests: valid YAML, missing required, unknown/duplicate/multiple documents, wrong scalar type, `0s`, question limit `0`, false, missing backend secrets, ошибки без секретов, legacy env не влияет на YAML.
- [ ] Выполнить `go test ./internal/app ./cmd/api -count=1`, подтвердить FAIL новых сценариев.
- [ ] Реализовать strict YAML loader на имеющемся yaml.v3; сохранить существующий Config для consumers и текущие validation bounds/defaults.
- [ ] Перевести startup, migrate и prepare-questions на YAML; тест подготовки без DB/S3 env.
- [ ] Исправить healthcheck: адрес брать из YAML без требования секретов; wildcard host → loopback с сохранением порта/IPv6. Тест с портом 18002 вместо 8002.
- [ ] Повторить focused tests: PASS.

### Задача 2: gateway YAML и объединение model rules

**Файлы:** `services/llm-gateway/internal/gateway/config.go`, `config_test.go`, `cmd/llm-gateway/main.go`, `go.mod/go.sum`; новый `config/dev.yaml`; позднее cleanup Dockerfile/models/example в задачах 4–5.

**Интерфейс:** `LoadConfig(path string) (Config, error)` читает один YAML, применяет embedded model_rules, defaults и validation, включая required profiles. Main больше не загружает второй файл; callers старого `Load(configPath, rulesPath, rulesRequired)` перевести на единый интерфейс.

- [ ] Добавить падающие tests: strict schema, duration, сохранение пустого/nonempty token, ошибки без token, nested body с false/number, precedence model rules, required profiles и standalone empty requirements.
- [ ] Выполнить `GOWORK=off go test ./internal/gateway -count=1`, подтвердить FAIL новых сценариев.
- [ ] Добавить прямую yaml.v3 зависимость и реализовать loader; сохранить формат JSON body для существующих upstream consumers и validation запрещённых request fields.
- [ ] Перевести main/tests с example и отдельного models.json на единый YAML/локальные fixtures; сохранить текущие routing semantics.
- [ ] Запустить focused tests: PASS. Проверить существующие queue/recovery tests; при недоступной Kafka явно назвать integration skip.

### Задача 3: единая конфигурация Python

**Файлы:** новый `course_graph/config.py`, `tests/test_config.py`; `api.py`, `model_runtime.py`, `llm/client.py`, `topics/extract.py`, `topics/retrieval.py`, `ocr/converter.py`; `pyproject.toml/uv.lock`; `config/dev.yaml` внутри course-graph.

**Интерфейсы:** `load_config(path: str | Path) -> Settings`; `get_settings() -> Settings` — один process snapshot; `configure_library_environment(settings: Settings) -> None` — до torch/Docling. Settings immutable, Pydantic extra=forbid. После watcher restart snapshot создаётся заново.

- [ ] Добавить падающие tests типов/дубликатов/unknown/multiple docs, durations, false/zero, bounds, legacy env precedence и трёх ранее не проброшенных параметров bootstrap attempts/picture text/rapid cache dir.
- [ ] Выполнить config tests и подтвердить FAIL нового поведения.
- [ ] Добавить PyYAML через uv, реализовать parser и перевести consumers; конфиг загружать до module-level defaults ExtractionSettings и тяжёлых импортов.
- [ ] Перевести OCR fingerprint на эффективные Settings: одинаковые значения → одинаковый ключ; изменение enrich/full_page/picture_text/timeout → другой ключ. Пути cache сами не меняют результат.
- [ ] Проверить порядок инициализации subprocess test с подменёнными тяжёлыми модулями; explicit OMP/MKL override сохраняется.
- [ ] Выполнить focused config/cache/runtime/OCR tests: PASS, без скачивания моделей.

### Задача 4: dev watchers и mounts

**Файлы:** backend `.air.toml`, Dockerfile/Compose; gateway новый `.air.toml`, Dockerfile/Compose; course-graph Dockerfile/Compose и dev dependency watchfiles; root Compose/dev override; `dev.sh`, `.env.example`.

- [ ] Backend: добавить yaml/yml в Air include_ext; config находится в уже смонтированном backend-каталоге и входит в watch scope. Изменение YAML может выполнять обычный Air build/restart, оптимизация без go build не требуется.
- [ ] Gateway: добавить dev stage с Air по существующему backend-паттерну; сохранить build/runtime stages. Watch scope — gateway source/config и shared contracts; временные Go/cache/bin volumes отделить от checkout.
- [ ] Course-graph: dev stage с watchfiles и source/config bind mounts, shared generated Python stubs доступны как сейчас. Watcher запускает `python -m course_graph.api`, отслеживает source/config, исключает data/model/cache directories; polling включить для Docker Desktop. Watchfiles добавить через uv как dev dependency, dev stage устанавливает эту группу; исходный runtime stage не требует watcher.
- [ ] В root dev override выбрать dev targets и добавить mounts для gateway/course-graph; standalone dev service Compose должен использовать те же targets/mounts, без требования prod env. Проверить разрешение путей через extends.
- [ ] Сохранять SIGTERM/graceful shutdown существующих процессов; watcher не должен накапливать дочерние процессы. Порты stack остаются согласованными: API 8002, gateway 8010/8011, course-graph 8000; изменение внутренних портов требует изменения инфраструктурных адресов вместе с YAML.
- [ ] Удалить mappings перенесённых app env, добавить process config pointers. Env backend secrets и инфраструктурные параметры сохранить; frontend VITE env сохранить как согласованное исключение для существующей сборки.
- [ ] Убрать из Compose принудительные defaults OMP/MKL, иначе они перекроют YAML-derived defaults. Возможность явных library env overrides сохранить.
- [ ] Проверять наличие YAML в dev.sh вместо копирования gateway example; исправить печатаемый URL с учётом эффективного PROXY_HTTPS_PORT, без source/eval .env.
- [ ] На изолированном dev stack выполнить atomic-save каждого YAML с безопасным изменением настройки, зафиксировать новый PID процесса и прежний container ID, проверить новое эффективное значение без model calls; затем вернуть настройку. Неверный YAML даёт явную ошибку, последующее исправление снова запускает процесс. Исключить restart loops от кешей/логов.

### Задача 5: лёгкое удаление prod, перенос локальной конфигурации и CI

**Файлы:** удалить `prod.sh`, `.env.prod.example`; gateway `config.example.json`, `models.json` после слияния; `.gitignore`; `.github/workflows/ci.yaml`; gateway Dockerfile; smoke/E2E references.

- [ ] Перенести существующий gateway JSON и model rules в dev YAML после приватного backup и сверки эффективных настроек; только затем убрать legacy файлы/ignore-rule. Backend/course-graph YAML заполнить текущими dev значениями, не prod placeholders.
- [ ] Удалить prod.sh/.env.prod.example и ссылки на них в активных инструкциях/CI/AGENTS. Отдельные production deployment jobs/configs убрать; image build/runtime stages и проверки компиляции образов сохранить.
- [ ] Gateway Dockerfile больше не копирует models.json. YAML не зашивается в образ: mounts дают актуальный dev config; прямой runtime-image запускается с явно смонтированным dev YAML.
- [ ] CI Compose validation использует только .env.example и временные env fixtures. Change detection учитывает сервисные YAML и watcher configs, не удалённые prod files.
- [ ] Smoke CI больше не копирует example и не удаляет tracked gateway config при cleanup. Создать временные YAML fixtures для smoke с прежними STT/TTS заглушками, disabled tutor/bootstrap, LLM timeout; обеспечить выбор fixtures через process pointers/mounts без редактирования committed YAML.
- [ ] Сохранить существующие functional stack smoke и browser E2E. Убрать из их названий предположение об отдельном production-окружении; при переименовании файла production-stack.spec.ts обновить все references. Smoke может проверять сохранённые runtime images; watcher-тест задачи 4 отдельно проверяет dev targets.
- [ ] Проверить resolved Compose JSON с тестовыми placeholders: нужные config directories, process pointers и secrets mapping, отсутствие старых app env. Не печатать реальные секреты.

### Задача 6: документация, правила и итоговая проверка

**Файлы:** root/backend/frontend/course-graph/gateway README, root/backend/gateway AGENTS, checklist.html; сервисные инструкции конфигурации co-located в README.

- [ ] Обновить AGENTS: YAML в Git, model tokens пока в gateway YAML по решению владельца; прочие секреты env. Новое поле обновляется в loader/tests/dev YAML/README; prod template и example больше не требуются.
- [ ] Описать только актуальный dev запуск и изменение YAML через watcher; VITE_API_BASE_URL сохранить. Для существующего build/runtime artifact точно указать, что VITE_API_BASE_URL применяется при rebuild, а не одном recreate; не заводить отдельную инструкцию prod deployment.
- [ ] Описать миграцию локального .env/JSON и backup без раскрытия токенов. `.env` и секреты БД/S3 по-прежнему игнорируются Git. Исторические отчёты не переписывать как текущие инструкции.
- [ ] В README таблице решений записать фактически согласованные решения от 27.10.2026; обновить «Текущая реализация», «План работ» и checklist. Этот план оставить историей реализации, не вторым источником текущей конфигурации.
- [ ] Закрыть матрицу расхождений ниже; repository search старых имён не должен находить активные инструкции прежнего запуска.

## Матрица найденных расхождений

| Расхождение | Решение / доказательство |
| --- | --- |
| Gateway/course-graph README требуют профиль gateway | Gateway обязателен root stack; мониторинг через observability. Исправить также Kafka test command в gateway AGENTS. |
| Пересоздание frontend объявлено достаточным для production VITE env | Настройку сохранить; документация build/runtime artifact требует rebuild. |
| Backend listen address свободный, healthcheck фиксирует 8002 | Healthcheck читает YAML; test на 18002. В Compose внутренние адреса согласованы. |
| prepare-questions отдельно читает gateway env | Общий YAML parser/gateway section; test без DB/S3 env. |
| Python читает параметры, не проброшенные Compose | Все доступны через dev YAML; consumer tests. |
| AGENTS запрещает коммит gateway config и адреса вне env/JSON | Правила привести к согласованным YAML и исключению model tokens. |
| Startup/tests/CI используют gateway example; cleanup удаляет config.json | Единый tracked YAML + временные test fixtures; cleanup удаляет только временное. |
| dev.sh печатает фиксированный 8443 | Учитывать эффективный proxy port; проверить стандартное и нестандартное значения. |
| YAML сейчас не отслеживается Air; другие сервисы без watchers | Task 4: atomic-save test подтверждает новый PID при прежнем container ID для всех трёх сервисов. |

## Финальные проверки и критерий завершения

- [ ] Backend: `go test -race ./... -count=1`, `go vet ./...`, `go build -o bin/api ./cmd/api`.
- [ ] Gateway: `GOWORK=off go test -race ./... -count=1`, `GOWORK=off go vet ./...`, `GOWORK=off go build ./cmd/llm-gateway`, `bash scripts/generate-openapi.sh` без diff OpenAPI.
- [ ] Course-graph: `uv lock --check`, `uv run --locked --extra cpu --extra ocr python -m pytest tests`.
- [ ] Frontend: `npm run format:check`, `npm run lint`, `npm run typecheck`, `npm test -- --run`, `npm run build`.
- [ ] Из корня: `sh -n dev.sh`; root Compose с .env.example с dev override и без него, external-S3 override, каждый изменённый service Compose — `config --quiet`. Удалённый prod.sh не проверяется.
- [ ] Сохранённые image builds, isolated stack smoke/browser E2E и dev watcher tests по задачам 4–5.
- [ ] `git diff --check`, diff/список файлов: три dev YAML, нет prod deployment files/example/model-rules duplication; env и backend/DB/S3 secrets вне Git, model tokens допускаются только YAML gateway и не печатаются при проверке.
- [ ] Отчёт: фактический результат всех проверок и причины пропусков; отдельно DB tests без TEST_DATABASE_URL, Kafka integration без TEST_KAFKA_BROKERS. Health не подтверждает модели/миграции: их доступность/применение подтверждать только фактическим запросом, если он выполнялся.

**Готово, когда:** чистый checkout с заполненными dev secrets запускается через dev.sh; все три сервиса читают свои YAML и реагируют на их изменения автоматическим restart процесса без пересоздания контейнера; legacy app env не влияет на YAML; gateway использует объединённые rules; отдельный prod deployment больше не поддерживается; полезные build/smoke/E2E проверки сохранены; найденные расхождения закрыты, оставшиеся и пропущенные проверки явно названы.
