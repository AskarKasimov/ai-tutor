# Архитектура backend

Код находится в `services/backend`, Go-модуль — `github.com/AskarKasimov/ai-tutor/services/backend`. Прежняя модель (карта компетенций, варианты, assessment, taskgen, тренировка, озвучка заданий) удалена 16.10.2026. Geek и оркестрация course-graph будут отдельными модулями backend с теми же слоями; LLM вызываются через отдельный сервис LLM gateway; этапы обработки course-graph выполняет Python-сервис `services/course-graph`. Видение — в [README](../README.md).

## Реализованные части

| Путь | Ответственность |
| --- | --- |
| `cmd/api` | Запуск, миграции, healthcheck, фоновая проверка S3-бакета, shutdown. |
| `internal/app` | Конфигурация, зависимости, router, auth middleware и HTTP logging. |
| `features/auth` | Регистрация/вход, refresh/logout/me, общий аккаунт админки, PostgreSQL и Argon2id. |
| `features/voice` | STT/TTS, проверка входа, сохранение расшифровки владельца. |
| `features/subject` | Каталог предметов (курсов) и создание предмета admin. |
| `features/materials` | Загрузка материалов курса, OCR через course-graph, хранение в S3 и PostgreSQL, список, удаление и выдача исходника. |
| `entities` | Пользователь, auth-сессия, аудио, расшифровка, предмет и материал. |
| `shared` | Fault, security, HTTP helpers, S3-клиент, миграции и SQLC. |

Каждый feature содержит `application`, `infrastructure`, `transport/http`. Application определяет сценарии и порты; инфраструктура реализует порты; HTTP переводит запросы/ответы. `app` связывает зависимости. Сценарии и entities не импортируют HTTP, pgx или собственные адаптеры. Features не импортируют сценарии друг друга; общие доменные типы находятся в entities.

SQL находится в `shared/postgres/queries`; PostgreSQL-адаптеры вызывают сгенерированные SQLC-методы. Генератор ID — в `shared/security`. S3-клиент `shared/s3` реализует общие операции с объектами; адаптер материалов в `app` передаёт ему бакет.

Авторизация выполняется middleware: auth возвращает principal, HTTP handler передаёт его сценарию. Загрузка материалов повторно проверяет admin внутри application. `shared` не содержит учебных правил и не зависит от features/entities.

## API и модели

Единственный контракт — [api/openapi.yaml](../api/openapi.yaml). `x-implementation` отличает работающие операции от планируемых; наличие planned в Swagger не означает наличие handler. Внутренний API Python-сервиса — [api/course-graph.yaml](../api/course-graph.yaml).

Frontend вызывает общий HTTPS API `/api/v1`; Caddy удаляет префикс. STT/TTS вызываются backend через HTTP-адаптеры, OCR — через сервис course-graph; настройки моделей находятся только на сервере. `/health` проверяет backend/БД, не модели.
