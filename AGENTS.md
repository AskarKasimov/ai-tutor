# Правила работы в репозитории

Правила для агентов и разработчиков во всём репозитории. Описание продукта, статус, запуск и решения — в [README](README.md); здесь только то, что нужно выполнять. Локальный AGENTS.md сервиса дополняет эти правила для своего каталога: [backend](services/backend/AGENTS.md), [frontend](services/frontend/AGENTS.md), [llm-gateway](services/llm-gateway/AGENTS.md), [shared](services/shared/AGENTS.md), [course-graph](services/course-graph/AGENTS.md). Перед изменением сервиса прочитайте его.

## Перед изменением

Определите затронутый слой, прочитайте его документацию и выберите объём локальных проверок по правилам ниже. Таблица перечисляет полный набор проверок слоя; он не обязателен локально для каждой мелкой правки.

### Локальные проверки и нагрузка

- Выполняйте тяжёлые команды последовательно: тесты, линтер, проверку типов, сборку и Docker build. Не запускайте одновременно несколько таких команд или проверки нескольких сервисов. Независимое чтение файлов можно выполнять параллельно.
- Локально Vitest запускайте с `--maxWorkers=1`, Playwright — с `--workers=1`. Для Go используйте `GOMAXPROCS=2` и `-p 1` в `go test`, `go vet` и `go build`; у тестов дополнительно `-parallel 2`. `-race` сохраняйте при проверке Go-кода. Эти ограничения относятся к локальному запуску; полный набор CI сохраняется.
- Документация и инструкции без изменения кода: проверьте diff, ссылки и соответствие описанных команд текущим конфигам. Тесты приложения и сборка не нужны.
- Текст, переводы и локальные стили без изменения поведения: проверьте форматирование затронутых файлов, для TS/TSX — линтер этих файлов, и связанные существующие тесты, если они проверяют изменённый экран. Для стилей дополнительно осмотрите экран. Полный прогон, проверка типов и сборка локально не обязательны.
- Изменения логики: проверьте затронутые сценарии и использующие их модули. Для frontend выполните форматирование, линтер, проверку типов и связанные тесты; сборку — при изменении маршрутов, импортов, зависимостей или настроек сборки. Для Go выполните тесты с `-race`, vet и сборку затронутого сервиса с указанными лимитами. Если область влияния неясна или затронут общий код, выполните полный набор слоя.
- Контракты, миграции, auth, правила оценивания и конфигурация требуют полного набора затронутых слоёв, включая соответствующие интеграционные проверки при наличии окружения. Если ресурсов не хватает, выполните доступные целевые проверки, а оставшиеся явно перечислите в PR и проверьте их результат в CI; не выдавайте неполный прогон за полный.
- Для обычного запуска уже собранного проекта с настроенными `.env` и сертификатами используйте `docker compose --env-file .env -f docker-compose.yaml -f docker-compose.dev.yaml up -d --no-build`. `./dev.sh -d` включает сборку; она нужна при первом запуске и изменении Dockerfile/зависимостей. Изменения исходников в dev подхватывают watchers. Не сбрасывайте volumes или кеши ради ускорения проверок.
- После успешной проверки не повторяйте её без новых изменений или выявленной проблемы. В отчёте указывайте фактический объём локальных проверок и что оставлено CI.

| Слой | Документация | Проверки (из каталога слоя) |
| --- | --- | --- |
| `services/backend` | [AGENTS.md](services/backend/AGENTS.md), [README](services/backend/README.md), [архитектура](documents/backend-architecture.md), [auth](documents/auth.md) | `GOMAXPROCS=2 go test -p 1 -parallel 2 -race ./... -count=1`, `GOMAXPROCS=2 go vet -p 1 ./...`, `GOMAXPROCS=2 go build -p 1 -o bin/api ./cmd/api` |
| `services/frontend` | [AGENTS.md](services/frontend/AGENTS.md), [README](services/frontend/README.md) | `npm run format:check`, `npm run lint`, `npm run typecheck`, `npm test -- --run --maxWorkers=1`, `npm run build`; E2E в CI, локально по затронутым сценариям с `--workers=1` |
| `services/llm-gateway` | [AGENTS.md](services/llm-gateway/AGENTS.md), [README](services/llm-gateway/README.md), спецификации [HTTP](documents/llm-gateway-spec.md) и [gRPC](documents/llm-gateway-grpc-spec.md) | `GOWORK=off GOMAXPROCS=2 go test -p 1 -parallel 2 -race ./... -count=1`, `GOWORK=off GOMAXPROCS=2 go vet -p 1 ./...`, `GOWORK=off GOMAXPROCS=2 go build -p 1 ./cmd/llm-gateway`, `bash scripts/generate-openapi.sh` без изменений |
| `services/shared` | [AGENTS.md](services/shared/AGENTS.md), [README](services/shared/README.md) | Из корня: `services/shared/tools/generate.sh --check`, `services/shared/tools/check-compat.sh <base-sha>`; из `services/shared`: `GOWORK=off GOMAXPROCS=2 go test -p 1 -parallel 2 ./...` |
| `services/course-graph` | [AGENTS.md](services/course-graph/AGENTS.md), [README](services/course-graph/README.md), [api/course-graph.yaml](api/course-graph.yaml) | `uv lock --check`, `uv run --locked --extra cpu --extra ocr python -m pytest tests`, `docker compose --env-file ../../.env.example config --quiet` |
| `api` | [api/README](api/README.md) | Из `services/backend`: `bash scripts/generate-openapi.sh` без лишних изменений |
| Compose, `dev.sh`, `.env.example` | [README, «Запуск»](README.md#запуск), README сервиса | Из корня: `sh -n dev.sh`, root Compose с .env.example с dev override и без него, external-S3 override; `config --quiet` каждого изменённого service Compose с .env.example |

## Источники истины

- Учебная логика (граф, экзамен, оценки, тренировка) — README: «Курс», «course-graph», «Geek» и таблицы решений. [Концепция проректора](documents/Концепция_Репетитор_устного_экзамена.md) — исходник; при расхождении действует README.
- HTTP — [api/openapi.yaml](api/openapi.yaml): `implemented` — подключённая операция, `planned` — проект без handler. Внутренний API course-graph — [api/course-graph.yaml](api/course-graph.yaml), gRPC — [gateway.proto](services/shared/proto/llmgateway/v1/gateway.proto).
- Схема БД — [backend README, «БД и импорт»](services/backend/README.md#бд-и-импорт).
- Код расходится с документацией или требованием — не выбирайте молча: назовите расхождение в отчёте. Если нужно решение, спросите владельца продукта и запишите ответ с датой в таблицу решений README.
- Открытые вопросы — в [README](README.md#открытые-вопросы); не закрывайте их в коде без записанного решения.

## Продукт

- Учебное содержание берите из материалов курса или загруженной карты компетенций согласно README; ответы на вопросы студента в тренировке — по правилам README.
- Не фиксируйте в коде число недель и виды материалов.
- Правила экзамена (критерии, A–D, статусы, обход, пробелы, оценка курса) меняйте только вместе с README и тестами `internal/entities/exam`. Оценку A–D, статусы и обход считает код; модель возвращает вердикт, полноту ответа, баллы пунктов числовой рубрики либо К1–К4 для нечисловых правил и строки-основания. Режимы и допустимые шкалы описаны в README.
- Пользовательские тексты, граф, вопросы и диалог — на русском; строки frontend — через i18n.

## Код и контракты

- LLM вызывайте только через LLM gateway: без поля `model`, с обязательным `complexity`. Backend ходит в gateway по gRPC через `services/shared`; прямые вызовы моделей запрещены.
- Озвучка — только TTS backend; браузерный синтез речи не используйте.
- Настройки приложения backend, gateway и course-graph храните в их коммитных `config/dev.yaml`; переменные env не переопределяют YAML. Секреты backend/БД/S3 остаются в локальном env. По решению владельца upstream model tokens пока допустимы в gateway YAML. В `VITE_*` — только то, что можно отдать браузеру.
- Сгенерированные файлы (SQLC, `services/shared/gen`, `routeTree.gen.ts`, `openapi.yaml` gateway) вручную не правьте — меняйте источник и перегенерируйте. Сгенерированную часть `api/openapi.yaml` обновляйте только генератором backend.
- Применённые миграции не меняйте (подробности — в [backend](services/backend/AGENTS.md)). Переработка начальной схемы — только при согласованном сбросе dev-БД.
- Изменили контракт между сервисами (`api/*.yaml`, `.proto`) — обновите обе стороны и проверьте обе их командами.
- Поведение или статус реализации изменились — обновите README («Текущая реализация», «План работ»), README затронутого сервиса и [checklist.html](checklist.html). Новые config fields требуют loader, tests, dev YAML и README update; prod templates и gateway example больше не используются.

## Требует согласования

- Удаление модулей, таблиц, миграций, ручек, экранов и документов — сначала спросите владельца продукта.
- `docker compose down -v` и удаление volumes — только по явной просьбе: это удаляет данные PostgreSQL и S3.

## Отчёт о работе

- Перечислите выполненные проверки с фактическим результатом и пропущенные — с причиной. Пропуск DB-тестов без `TEST_DATABASE_URL` называйте явно.
- Доступность моделей и применение миграций подтверждайте фактическим запросом, а не чтением кода. Health проверяет только backend и БД.
- Перечислите найденные расхождения кода и документации, даже если не исправляли их.
