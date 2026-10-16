# Backend

Go API и PostgreSQL. Команды выполняются из `services/backend`, если не указано иначе. Видение (Geek, course-graph, LLM gateway) — в [корневом README](../../README.md).

## Реализовано и запланировано

Реализованы auth с cookies, каталог и создание предметов (курсов), синхронные STT/TTS, загрузка материалов курса с OCR через `services/course-graph` и хранением в S3 и PostgreSQL. Прежняя модель (карта компетенций, variantgen, assessment, taskgen, диагностика по варианту, фидбэк, план обучения, тренировка, озвучка заданий) удалена 16.10.2026; её код остался в истории git и в выпущенном релизе.

Реализована диагностика Geek — устный экзамен по графу курса (раздел ниже).

Запланированы (planned в [OpenAPI](../../api/openapi.yaml)):

- материалы с неделей и видом, правка материала;
- сборка и чтение графа курса (`/admin/subjects/{subject_id}/graph`);
- пакет для преподавателя в админке (данные для него уже хранятся).

## Диагностика Geek

Модуль `features/geek`, правила — в [README](../../README.md#geek-устный-экзамен-и-тренировка) и решениях 16.10.2026. Обход графа и оценка — чистый код в `entities/exam`: модель возвращает только вердикт (`answered` / `dont_know` / `insufficient`), К1–К4 и строки-основания, а A–D, статусы, засчитывание пререквизитов, спуск, пробелы, прогресс и оценку курса считает код.

- `POST /diagnostic-sessions` — начать или продолжить (одна активная на курс), `Idempotency-Key` обязателен; без графа — `409 GRAPH_NOT_READY`.
- `POST /diagnostic-sessions/{id}/answers` — синхронный ответ (расшифровка, текст или «не знаю»): реплика студента и следующая реплика Geek в том же ответе. Модель вызывается до сохранения: при сбое `503 MODEL_UNAVAILABLE`, сессия не меняется, повтор с тем же ключом. Повтор после успеха возвращает сохранённый результат; тот же ключ с другим телом — `422 IDEMPOTENCY_KEY_REUSED`.
- `GET /diagnostic-sessions?subject_id=…` — свои диагностики курса, новые первыми (до 50), с оценкой у завершённых; `GET /diagnostic-sessions/{id}`, `GET /diagnostic-sessions/{id}/result` (только завершённая).
- `GET /diagnostic-sessions/{id}/turns/{turn_id}/audio` — WAV реплики Geek: синтез частями до 500 символов, склейка и кэш в S3 (`geek/{session}/{turn}.wav`).

Сессия хранится одной строкой `geek_sessions` (снимок графа, состояние обхода и транскрипт в `state`) с версией для конкурентных обновлений; ключи повтора — в `geek_requests`. Если граф курса пересобран (сменилась версия), активная диагностика получает статус `cancelled`.

Граф берётся адаптером-цепочкой: сначала из БД (появится с хранением графа, поток A), затем из фикстуры `BACKEND_GEEK_GRAPH_FIXTURE`. LLM — gRPC-клиент LLM gateway (`llmgateway.v1.Gateway/Complete` из модуля `services/shared`, адрес `BACKEND_LLM_GATEWAY_ADDR`): без `model`, `complexity` — оценка `high`, основной вопрос `medium`, дополнительный вопрос и переформулировка `low`; `RESPONSE_FORMAT_JSON_OBJECT`; невалидный ответ повторяется один раз. Отмена контекста (студент ушёл, истёк таймаут) отменяет запрос и в gateway. Промпты, ответы модели, `request_id` gateway и тексты студентов логируются. В тестах gateway заменяет локальный gRPC-сервер с детерминированными ответами.

Для подготовки основных вопросов реализована функция `PrepareQuestions` (application Geek): она заполняет отсутствующий первый вопрос через gateway, сохраняя остальные вопросы и исходный граф. При ошибке вопрос остаётся пустым — Geek генерирует его в рантайме. Сейчас функция вызывается только из CLI; демо-фикстура подготовлена этой командой. Автоматический вызов перед сохранением собранного графа должен быть подключён в будущем модуле оркестрации потока A и пока не реализован. Для файла графа есть подкоманда:

```bash
BACKEND_LLM_GATEWAY_ADDR=127.0.0.1:8011 go run ./cmd/api prepare-questions ../../api/fixtures/course-graph.json /tmp/graph.json
```

Сквозной прогон для репетиции: `scripts/geek-smoke.sh [base-url] [strong|weak|mixed]` создаёт курс от админки, регистрирует студента и отвечает на все реплики до итога.

`GET /subjects` требует авторизацию. Студентам возвращаются только готовые курсы, admin видит все предметы и поле `ready`. Пока граф курса не хранится, курс готов, если у него есть материал со статусом `parsed`; после появления графа готовность будет означать собранный граф.

## Единый OpenAPI

```bash
bash scripts/generate-openapi.sh
```

Swag генерирует реализованный API во временный файл; merge-openapi обновляет implemented-операции, уже включённые в единый контракт, и сохраняет planned. Исключённые операции не возвращаются автоматически. Новую работающую ручку нужно явно добавить/перевести из planned в implemented в контракте. Неиспользуемые схемы удаляются. Swagger использует этот же YAML.

CI проверяет повторяемость генерации и валидирует реальные ответы по implemented-схемам. Planned-схемы проверяются отдельно без объявления их работающими. Загрузка и список материалов уже работают, но в контракте описаны как planned (с неделей и видом), поэтому контрактный тест их ответы пока не проверяет. Swag/yq запускаются через go run с закреплёнными версиями, первый запуск требует загрузки инструментов.

## Запуск

Общий dev-стек запускается из корня через `./dev.sh -d` после настройки `.env`. Нужны Docker Compose и mkcert. Серверный `./prod.sh` использует готовые сертификаты и сеть `ai-tutor_default`; параметры находятся в `.env.prod.example`.

Backend импортирует сгенерированный клиент gateway из соседнего модуля `services/shared` (`replace ../shared` в `go.mod`), поэтому Docker собирает его из корня репозитория (`Dockerfile.dockerignore` пропускает только `services/backend` и `services/shared`). Dev-контейнер монтирует оба модуля в `/work/backend` и `/work/shared`, и Air следит за обоими: изменение контракта в `services/shared` тоже пересобирает API. Gateway в production поднимает `prod.sh` (проект `services/llm-gateway` с Redpanda); соединение с ним закрывается после остановки HTTP-сервера.

Для отдельного backend Compose из этой папки:

```bash
docker compose --env-file ../../.env up -d --build
```

Уже должны работать db и внешняя сеть. BACKEND_DATABASE_URL переопределяет подключение; по умолчанию db:5432 и DB_PASSWORD. API слушает :8002 за Caddy HTTPS. Публичный префикс /api/v1 удаляется proxy. Secure cookies требуют HTTPS.

Обязательны BACKEND_STT_URL, BACKEND_TTS_* (URL и параметры голоса), BACKEND_VOICE_TIMEOUT, адрес прослушивания, лимит загрузки и настройки S3 из корневого шаблона. BACKEND_COURSE_GRAPH_URL — адрес сервиса course-graph для OCR. Geek: BACKEND_LLM_GATEWAY_ADDR (обязателен, gRPC host:port; в Compose — `llm-gateway:8011`, gateway запускается профилем `gateway`), BACKEND_LLM_TIMEOUT (по умолчанию 120s), BACKEND_GEEK_GRAPH_FIXTURE (Compose монтирует `api/fixtures` в `/fixtures` и по умолчанию берёт `/fixtures/course-graph.json`), BACKEND_GEEK_MAX_QUESTIONS (0 — без ограничения). После изменения env пересоздайте API.

Запросы VoxCPM передают `BACKEND_TTS_SEED`, `BACKEND_TTS_CFG_VALUE` (0–5) и `BACKEND_TTS_INFERENCE_TIMESTEPS` (1–50); шаблоны env задают 510, 2 и 20. Перед отправкой текста backend добавляет `!!!...`, чтобы озвучка завершала последнее слово. По решению встречи 10.10.2026 голос наставника — мужской, взрослый, низкий; сервис VoxCPM клонирует его по референсу, сохранённому в своём volume.

При старте API в фоне проверяет S3-бакет и создаёт его при `BACKEND_S3_CREATE_BUCKET=true`, повторяя попытки до успеха. HTTP write timeout — максимум из STT с записью в S3, OCR загруженного материала, ответа Geek (оценка и следующий вопрос, каждый с одним повтором) и синтеза длинной реплики плюс 30 секунд. Health проверяет API и БД, не модели и не S3.

COMPOSE_PROFILES=swagger включает Swagger под /docs/. После обновления YAML пересоздайте swagger, потому что образ копирует спецификацию при старте.

Из этой папки для Go-процесса нужны Go из `go.mod` и PostgreSQL. Полный набор env — в корневом .env.example; приложение само файл .env не читает.

```bash
go run ./cmd/api migrate
go run ./cmd/api
```

STT принимает multipart audio (WAV/Ogg/WebM) до 25 МиБ, сохраняет расшифровку и возвращает id/text/created_at. TTS принимает text до 500 символов и возвращает WAV. Без longform/VAD весов удалённый GigaAM отклоняет записи длиннее ~25 с (503), поэтому 16-bit PCM WAV длиннее 20 с распознаётся частями до 20 с, разрезанными по самому тихому месту, и текст частей склеивается; Ogg/WebM отправляются как есть. HTTP модели разворачиваются отдельно; frontend не знает их адресов.

## Live reload в Docker

Корневой `./dev.sh -d` использует Dockerfile target `dev` с Air. Исходники backend смонтированы в `/work/backend`, общий модуль контрактов — в `/work/shared`; Air пересобирает и перезапускает API при изменениях Go-файлов, SQL, `go.mod` и `go.sum` в обоих. Конфигурация watcher — `.air.toml`; polling позволяет подхватывать изменения через Docker Desktop. Модули Go, кеш сборки и каталог `.air` с dev-бинарником находятся в Docker volumes. Миграции применяются штатным запуском API; SQLC-запросы требуют отдельной генерации Go-кода. Локальный Compose backend использует готовый production-бинарник.

## БД и импорт

Goose применяет встроенные SQL-миграции при старте или migrate; запуск сериализован advisory lock. `00001_initial.sql` создаёт пользователей, auth-сессии и токены, расшифровки, предметы и материалы курса (`subject_materials`: файл, S3-ключи исходника и распознанного текста, статус OCR). `00002_geek.sql` — диагностики Geek (`geek_sessions`, одна активная на студента и курс) и ключи повтора (`geek_requests`). Номер новой миграции — следующий свободный на момент мержа: goose не применяет миграцию с номером меньше уже применённого.

Схема собрана заново 16.10.2026 при удалении прежней модели. С БД, где применена прежняя история из десяти миграций, она несовместима: dev-БД и БД стенда нужно сбросить (`docker compose down -v` удаляет volumes PostgreSQL и S3). Для БД с настоящими данными новые изменения оформляются следующими миграциями.

Админ-панель (`/admin`) сама вызывает `POST /auth/teacher` без тела и пароля, передавая заголовок `X-AI-Tutor-Portal: admin`; без него маршрут отвечает 404. С этим заголовком auth-маршруты и защищённые запросы используют отдельные cookies `admin_access_token`/`admin_refresh_token`, поэтому сессии ученика и админки в одном браузере независимы. Backend атомарно создаёт общий аккаунт `user:shared-teacher` с ролью admin и обычную cookie-сессию. Повторные входы используют тот же аккаунт и независимые сессии; logout отзывает только текущую. Общий адрес `shared-teacher@ai-tutor.invalid` зарезервирован и не принимается обычной регистрацией/парольным входом.

Дополнительный персональный admin может назначаться после регистрации:

```bash
docker compose -f ../../docker-compose.yaml exec db psql -U ai_tutor -d ai_tutor \
  -c "UPDATE users SET role='admin' WHERE email='teacher@example.edu';"
```

Admin создаёт предмет через `POST /admin/subjects` с JSON-телом `{"name":"..."}`; устойчивый ID создаёт backend. Материалы загружаются `POST /admin/subjects/{subject_id}/materials` (multipart, поле `file`, несколько файлов): исходник сохраняется в S3, OCR выполняет `services/course-graph`, распознанный Markdown сохраняется в S3 и PostgreSQL. `GET` на тот же путь возвращает список, `DELETE .../materials/{material_id}` удаляет материал, `GET .../materials/{material_id}/raw` отдаёт исходный файл.

## Проверки

```bash
export TEST_DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Интеграционные тесты создают отдельную PostgreSQL schema, модели и S3 заменяются локальными HTTP-серверами. Без TEST_DATABASE_URL DB-тесты пропускаются; CI поднимает PostgreSQL. Общая HTTP-фикстура использует быстрый тестовый хешер паролей; тесты регистрации и входа используют настоящий Argon2id с рабочими параметрами. Независимые HTTP DB-тесты выполняются параллельно, каждый в своей схеме и с пулом до четырёх соединений; рабочий migrator с блокировкой проверяется отдельно в `shared/postgres`. Тесты, меняющие окружение процесса, и внешний S3 smoke (`TEST_S3_ENDPOINT`) остаются последовательными. SQLC: `bash scripts/generate-sql.sh`. Архитектура: [backend-architecture.md](../../documents/backend-architecture.md).
