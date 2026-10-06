# Голосовой AI-репетитор

Приложение для диагностики и тренировки знаний: студент читает задание,
отвечает голосом и получает оценку с объяснением.
Backend — Go/PostgreSQL, frontend — React/TypeScript/Vite.
Подробный контекст, состояние реализации и правила изменений — в [AGENTS.md](AGENTS.md).

## Структура проекта

| Каталог | Что находится внутри |
| --- | --- |
| `services/backend/` | API, миграции, SQLC-запросы и Swagger. |
| `services/frontend/` | Интерфейс, запись голоса и клиент API. |
| `services/postgresql/` | Compose для PostgreSQL. |
| `services/caddy/` | HTTPS-прокси и его Compose. |
| `api/` | Контракты API по этапам. |
| `documents/` | Сценарии, архитектура и предметная документация. |

У каждого сервиса свой Compose. Корневой `docker-compose.yaml` собирает
единый dev-стек через `extends` и запускает Vite и Air с исходниками через bind mount.

## Запуск и окружение

Для dev нужны Docker с Compose и [mkcert](https://github.com/FiloSottile/mkcert).
Команды выполняются из корня репозитория:

```bash
cp .env.example .env
# Задайте DB_PASSWORD и выберите режимы API в .env.
./dev.sh -d
```

`dev.sh` установит доверие к CA mkcert, выпустит localhost-сертификаты в `certs/`
и запустит корневой Compose с пересборкой. Compose создаст dev-сеть.
На Windows запускайте shell-скрипты через Git Bash.

- Приложение: [https://localhost:8443](https://localhost:8443).
- Health API: [https://localhost:8443/api/v1/health](https://localhost:8443/api/v1/health).
- Swagger: [https://localhost:8443/docs/](https://localhost:8443/docs/), если `COMPOSE_PROFILES=swagger`.

Микрофон и auth cookies требуют доверенного HTTPS. `.env` и сертификаты
не коммитьте. Данные PostgreSQL сохраняются между перезапусками.

### Моки и реальные API

| Настройка | Поведение |
| --- | --- |
| `VITE_API_MODE=mock` | Браузер использует локальные ответы API. |
| `VITE_API_MODE=real` | Браузер отправляет HTTP-запросы backend. |
| `BACKEND_API_MODE=mock` | Backend подменяет STT/TTS/LLM; auth и БД настоящие. |
| `BACKEND_API_MODE=real` | Backend обращается к удалённым моделям. |
| `COMPOSE_PROFILES=swagger` | Запускается контейнер Swagger; пустое значение выключает профиль. |

Шаблон `.env.example` включает моки в обоих слоях. Чтобы проверить backend
без удалённых моделей, задайте `VITE_API_MODE=real`, `BACKEND_API_MODE=mock`.
Учебная сессия frontend пока остаётся моковой в обоих режимах.

Для реальных моделей настройте `BACKEND_STT_URL`, `BACKEND_TTS_URL`,
`BACKEND_ASSESSMENT_*` и отдельные `BACKEND_TASKGEN_*` из шаблона.
Адреса должны быть доступны из контейнера backend.
В dev после изменения `.env` пересоздайте нужный контейнер через `docker compose up -d frontend`
или `api`. В production изменения `VITE_*` требуют пересборки frontend: значения встраиваются в образ.

### Обновление кода без пересборки образов

После `./dev.sh -d` изменения `services/frontend/` подхватывает Vite HMR,
а изменения Go-кода и SQL в `services/backend/` — Air: он пересобирает
и перезапускает API. Оба работают через bind mount; polling включён для Docker Desktop.
HTTPS и WebSocket HMR проходят через Caddy по прежнему адресу.
Режимы `mock`/`real` берутся из `.env`; live reload работает в обоих.

`node_modules`, модули Go, кеш сборки и dev-бинарник находятся в volumes контейнеров.
При изменении `package-lock.json` перезапустите frontend:
`docker compose restart frontend` — при старте выполняется `npm ci`.
Go-зависимости подхватывает следующая сборка. Изменения Dockerfile требуют
`docker compose up -d --build api frontend`.
Локальные Compose для production продолжают запускать готовый бинарник и nginx.

### Повседневные команды

```bash
docker compose ps
docker compose logs -f api frontend proxy
docker compose up -d --build --no-deps api
docker compose up -d --build --no-deps frontend
docker compose down
```

`down` сохраняет volumes. `down -v` удаляет данные БД.
Swagger читает `services/backend/backend.api.yaml`; после генерации схемы
пересоздайте его: `docker compose up -d --force-recreate swagger`.
Смена профиля не останавливает уже запущенный контейнер; остановить Swagger
можно командой `docker compose --profile swagger stop swagger`.

## Серверный запуск

```bash
cp .env.prod.example .env
# Замените пароль БД и адреса моделей.
# Подготовьте certs/server.pem и certs/server-key.pem для имени сервера.
# Один раз создайте сеть вручную, если её ещё нет:
docker network create ai-tutor_default
./prod.sh
```

Production-шаблон включает реальные API, HTTPS на `0.0.0.0:443` и выключает Swagger.
Сертификат с цепочкой и ключ должны быть в PEM; `prod.sh` их не выпускает.
Скрипт запускает локальные Compose по очереди: PostgreSQL → backend → frontend → Caddy,
дожидаясь готовности каждого проекта. При ошибке он останавливается.
Настройки берутся из `.env`; например, `./prod.sh --wait-timeout 180`
задаёт время ожидания для каждого проекта.

Dev-стек и серверные проекты на одном хосте одновременно не запускайте:
они используют одинаковые порты, имена сервисов и volumes.
После серверного запуска управляйте сервисом через его локальный Compose, например:

```bash
docker compose --env-file .env -f services/backend/docker-compose.yaml logs -f api
```

## Проверки перед PR

Для разработки вне Docker нужны Go версии из `services/backend/go.mod`
и Node.js 22 с npm. Frontend:

```bash
cd services/frontend
npm ci
npm run lint
npm run typecheck
npm test -- --run
npm run build
```

Backend:

```bash
cd services/backend
# Для интеграционных тестов задайте TEST_DATABASE_URL локальной тестовой БД.
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Без `TEST_DATABASE_URL` интеграционные тесты пропускаются. Пример подключения
и генерация OpenAPI — в [backend README](services/backend/README.md).
CI поднимает PostgreSQL для тестов и проверяет оба сервиса по изменённым путям,
а также Compose с dev/prod-шаблонами.

## Подробная документация

- [Backend](services/backend/README.md): API, импорт данных, генерация, настройки и тесты.
- [Frontend](services/frontend/README.md): локальный Vite, режимы API и инструменты.
- [PostgreSQL](services/postgresql/README.md) и [Caddy](services/caddy/README.md): отдельный запуск.
- [Правила и контекст проекта](AGENTS.md), [термины](CONTEXT.md), [план БД и генерации](PLAN.md).
- [Текущий OpenAPI](services/backend/backend.api.yaml) и [контракты по этапам](api/README.md).
