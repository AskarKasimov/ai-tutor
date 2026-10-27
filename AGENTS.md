# Правила работы в репозитории

Правила для агентов и разработчиков во всём репозитории. Описание продукта, статус, запуск и решения — в [README](README.md); здесь только то, что нужно выполнять. Локальный AGENTS.md сервиса дополняет эти правила для своего каталога: [backend](services/backend/AGENTS.md), [frontend](services/frontend/AGENTS.md), [llm-gateway](services/llm-gateway/AGENTS.md), [shared](services/shared/AGENTS.md), [course-graph](services/course-graph/AGENTS.md). Перед изменением сервиса прочитайте его.

## Перед изменением

Определите затронутый слой, прочитайте его документацию и после изменения выполните его проверки.

| Слой | Документация | Проверки (из каталога слоя) |
| --- | --- | --- |
| `services/backend` | [AGENTS.md](services/backend/AGENTS.md), [README](services/backend/README.md), [архитектура](documents/backend-architecture.md), [auth](documents/auth.md) | `go test -race ./... -count=1`, `go vet ./...`, `go build -o bin/api ./cmd/api` |
| `services/frontend` | [AGENTS.md](services/frontend/AGENTS.md), [README](services/frontend/README.md) | `npm run format:check`, `npm run lint`, `npm run typecheck`, `npm test -- --run`, `npm run build` |
| `services/llm-gateway` | [AGENTS.md](services/llm-gateway/AGENTS.md), [README](services/llm-gateway/README.md), спецификации [HTTP](documents/llm-gateway-spec.md) и [gRPC](documents/llm-gateway-grpc-spec.md) | `GOWORK=off go test -race ./... -count=1`, `GOWORK=off go vet ./...`, `GOWORK=off go build ./cmd/llm-gateway`, `bash scripts/generate-openapi.sh` без изменений |
| `services/shared` | [AGENTS.md](services/shared/AGENTS.md), [README](services/shared/README.md) | Из корня: `services/shared/tools/generate.sh --check`, `services/shared/tools/check-compat.sh <base-sha>` |
| `services/course-graph` | [AGENTS.md](services/course-graph/AGENTS.md), [README](services/course-graph/README.md), [api/course-graph.yaml](api/course-graph.yaml) | `uv run --locked --extra cpu --extra ocr python -m pytest tests` |
| `api` | [api/README](api/README.md) | Из `services/backend`: `bash scripts/generate-openapi.sh` без лишних изменений |
| Compose, `dev.sh`, `.env.example` | [README, «Запуск»](README.md#запуск), README сервиса | Из корня: `sh -n dev.sh`, root Compose с .env.example с dev override и без него, external-S3 override; `config --quiet` каждого изменённого service Compose с .env.example |

## Источники истины

- Учебная логика (граф, экзамен, оценки, тренировка) — README: «Курс», «course-graph», «Geek» и таблицы решений. [Концепция проректора](documents/Концепция_Репетитор_устного_экзамена.md) — исходник; при расхождении действует README.
- HTTP — [api/openapi.yaml](api/openapi.yaml): `implemented` — подключённая операция, `planned` — проект без handler. Внутренний API course-graph — [api/course-graph.yaml](api/course-graph.yaml), gRPC — [gateway.proto](services/shared/proto/llmgateway/v1/gateway.proto).
- Схема БД — [backend README, «БД и импорт»](services/backend/README.md#бд-и-импорт).
- Код расходится с документацией или требованием — не выбирайте молча: назовите расхождение в отчёте. Если нужно решение, спросите владельца продукта и запишите ответ с датой в таблицу решений README.
- Открытые вопросы — в [README](README.md#открытые-вопросы); не закрывайте их в коде без записанного решения.

## Продукт

- Учебное содержание (вопросы, тексты тем, оценки) берите только из материалов курса. Исключение — ответы на вопросы студента в тренировке по правилам README.
- Не фиксируйте в коде число недель и виды материалов.
- Правила экзамена (критерии, A–D, статусы, обход, пробелы, оценка курса) меняйте только вместе с README и тестами `internal/entities/exam`. Оценку A–D, статусы и обход считает код; модель возвращает только вердикт, К1–К4 и строки-основания.
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
