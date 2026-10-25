# Архитектура backend

Код находится в `services/backend`, Go-модуль — `github.com/AskarKasimov/ai-tutor/services/backend`. Geek — модуль `features/geek`; legacy обучение восстановлено в собственных `features/learning`, `features/diagnostic`, `features/training`, `features/taskgen`, `features/variantgen`, `features/competency`, `features/taskbank`, `features/taskaudio` и assessment/feedback slices. Режимы работают в одном router и PostgreSQL, но учебные таблицы, subject state, snapshots и очереди аудио принадлежат `learning_*`. Geek использует собственные exam sessions/graph. LLM вызываются через отдельный LLM gateway (`services/llm-gateway`) с HTTP-контрактом в `openapi.yaml` и gRPC-контрактом в `services/shared/proto/llmgateway/v1/gateway.proto`; обучение выбирает настроенный routing profile вместо provider/model. Этапы обработки course-graph выполняет Python-сервис `services/course-graph`. Видение — в [README](../README.md).

## Реализованные части

| Путь | Ответственность |
| --- | --- |
| `cmd/api` | Запуск, миграции, healthcheck, фоновая проверка S3-бакета, shutdown. |
| `internal/app` | Конфигурация, зависимости, router, auth middleware и HTTP logging. |
| `features/auth` | Регистрация/вход, refresh/logout/me, общий аккаунт админки, PostgreSQL и Argon2id. |
| `features/voice` | STT/TTS, проверка входа, сохранение расшифровки владельца. |
| `features/subject` | Каталог предметов (курсов) и создание предмета admin. |
| `features/{diagnostic,assessment,diagnosticfeedback,learning,training}` | Legacy диагностика, старые шкалы оценивания, feedback, план и тренировка обучения; изолированные SQL объекты и snapshots. |
| `features/{competency,variantgen,taskbank,taskgen,taskaudio}` | Импорт второго листа XLSX карты, генерация/выдача вариантов, банк заданий, подготовка задач через gateway и audio worker поверх общего S3 клиента. |
| `features/materials` | Загрузка материалов курса, фоновые OCR через course-graph, перевод через LLM gateway и очистка с темами, хранение в S3 и PostgreSQL, список, справочник видов, неделя/тип, правка метаданных, замена PDF, удаление и выдача исходника. Метаданные сохраняют готовые задания; замена отменяет старые leases и запускает обработку нового файла. |
| `features/coursegraph` | Сборка графа курса по кнопке (очередь с lease в PostgreSQL, вызов `/graph/build`), публикация и чтение снимка графа для админки. |
| `features/geek` | Диагностика Geek: сценарии старта, ответа, итога и озвучки реплик; порты графа, хранилища, расшифровок, модели (Examiner), TTS и кэша аудио. Адаптеры: PostgreSQL, цепочка источников графа (БД, затем фикстура), gRPC-клиент LLM gateway, кэш WAV в S3. |
| `entities/exam` | Граф курса (проверка ацикличности, верхний уровень, замыкание пререквизитов), таблица оценок A–D, обход экзамена, прогресс, пробелы и итог. Чистый код без IO. |
| `entities` | Пользователь, auth-сессия, аудио (проверка, разбиение и склейка WAV), расшифровка, предмет, материал и экзамен. |
| `shared` | Fault, security, HTTP helpers, S3-клиент, миграции и SQLC. |

Каждый feature содержит `application`, `infrastructure`, `transport/http`. Application определяет сценарии и порты; инфраструктура реализует порты; HTTP переводит запросы/ответы. `app` связывает зависимости. Сценарии и entities не импортируют HTTP, pgx или собственные адаптеры. Features не импортируют сценарии друг друга; общие доменные типы находятся в entities.

SQL находится в `shared/postgres/queries`; PostgreSQL-адаптеры вызывают сгенерированные SQLC-методы. Генератор ID — в `shared/security`. S3-клиент `shared/s3` реализует общие операции с объектами; адаптер материалов в `app` передаёт ему бакет.

Авторизация выполняется middleware: auth возвращает principal, HTTP handler передаёт его сценарию. `subjects.mode` разделяет общий каталог (`learn`/`exam`), а active learning map revision принадлежит `learning_subject_state`; exam handlers проверяют режим курса перед чтением и изменением материалов или графа. `shared` не содержит учебных правил и не зависит от features/entities.

## API и модели

Единственный контракт — [api/openapi.yaml](../api/openapi.yaml). `x-implementation` отличает работающие операции от планируемых; наличие planned в Swagger не означает наличие handler. Внутренний API Python-сервиса — [api/course-graph.yaml](../api/course-graph.yaml).

Frontend вызывает общий HTTPS API `/api/v1`; Caddy удаляет префикс. Learning operations находятся под `/learn`, Geek под `/exam`, auth и voice общие. STT/TTS вызываются backend через HTTP-адаптеры, OCR — через сервис course-graph; настройки моделей находятся только на сервере. `/health` проверяет backend/БД, не модели.
