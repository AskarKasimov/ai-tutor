# ai-tutor

Монорепозиторий голосового AI-репетитора: диагностика и тренировка по готовым заданиям преподавателя.

## Структура

```text
services/
  backend/       Go + PostgreSQL: публичный API, авторизация, голос и компетенции
  frontend/      React + TypeScript + Vite + Radix Themes: каркас приложения
api/             общие OpenAPI-контракты по этапам разработки
documents/       предметная область и архитектура
Caddyfile        HTTPS reverse proxy
docker-compose.yaml     локальный запуск frontend, backend, PostgreSQL и HTTPS proxy
```

Каждый сервис располагается в `services/<name>` и владеет своим кодом, зависимостями,
Dockerfile и инструкциями.

## Документация

- [Frontend: запуск, структура и проверки](services/frontend/README.md).

- [Backend: возможности API v0, запуск, настройки и проверки](services/backend/README.md).
- [HTTP API по этапам разработки](api/README.md).
- [Архитектура backend и правила зависимостей](documents/backend-architecture.md).
- [Диагностика и тренировка](documents/voice-trainer-scenarios.md).
- [Авторизация, сессии и права](documents/auth.md).
- [Методика разработки заданий и оценивания](documents/AIAssistantGrading.md).
- [Учебные термины](CONTEXT.md).

Frontend обращается к backend; тот вызывает внутренние сервисы распознавания и синтеза.
HTTP-обёртки моделей пока находятся в отдельном репозитории
[tts-stt](https://github.com/AskarKasimov/tts-stt).

## Локальный запуск

Требуются Docker с Compose и [mkcert](https://github.com/FiloSottile/mkcert).
Установка mkcert: на macOS — `brew install mkcert`, на Windows — `choco install mkcert`
или `scoop install mkcert` (bucket `extras`), на Linux — пакет дистрибутива или
[готовый бинарник](https://github.com/FiloSottile/mkcert/releases).
Для Firefox на macOS нужен `brew install nss`, на Debian/Ubuntu — `sudo apt install libnss3-tools`.

Один раз скопируйте `.env.example` в `.env` и задайте `POSTGRES_PASSWORD`, `STT_URL`,
`TTS_URL`. Адреса моделей должны быть доступны из backend-контейнера.

macOS / Linux / Windows (Git Bash):

```bash
cp .env.example .env
# Заполните .env, затем:
./run.sh
```

Скрипт работает из любой текущей директории. Сначала проверяет конфигурацию,
затем выполняет `mkcert -install`, выпускает сертификат для `localhost`, `127.0.0.1`
и `::1` в `certs/` и запускает `docker compose up --build`. mkcert может запросить
права администратора для установки доверия. При ошибке mkcert контейнеры не запускаются.
Для запуска в фоне добавьте `-d`: `./run.sh -d`.
На Windows выполняйте скрипт в Git Bash; `mkcert` и `docker` должны быть доступны в его PATH.

Приложение: `https://localhost:8443`. Caddy направляет `/api/v1` и `/api/v1/*`
в backend, удаляя префикс `/api/v1`; остальные пути — во frontend. Проверка API: `GET /api/v1/health`. Сертификаты и ключи
в `certs/` исключены из Git. Caddy использует сертификат mkcert; установка локальной
CA Caddy больше не требуется. После первой установки доверия перезапустите браузер.

## HTTPS в production

Локальный Caddyfile загружает сертификаты mkcert из `certs/`. Для публичного домена
используйте автоматическое получение и продление сертификатов Caddy:

```caddyfile
tutor.example.com {
    @api path /api/v1 /api/v1/*
    handle @api {
        uri strip_prefix /api/v1
        reverse_proxy api:8002
    }
    handle {
        reverse_proxy frontend:80
    }
}
```

Замените домен своим, направьте DNS на сервер и опубликуйте порты `80:80` и `443:443`
у `proxy`. Для HTTPS задайте `HTTPS_BIND=0.0.0.0`, `HTTPS_PORT=443`. Уберите файловую директиву `tls` и mount `certs`: сертификат выпустит публичная CA.
Volume `caddy_data` сохраняет выданные сертификаты между перезапусками.

## Проверки backend

```bash
cd services/backend
export TEST_DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Без `TEST_DATABASE_URL` интеграционные тесты пропускаются. CI запускает их с PostgreSQL.
