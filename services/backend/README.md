# Backend

Сервис публичного API голосового AI-репетитора на Go и PostgreSQL.
Команды в этом документе выполняются из `services/backend`, если не указано иначе.

## Документация

- [OpenAPI реализованного backend](backend.api.yaml).
- [Диагностика и тренировка](../../documents/voice-trainer-scenarios.md).
- [Авторизация, сессии и права](../../documents/auth.md).
- [Методика разработки заданий и оценивания](../../documents/AIAssistantGrading.md).
- [Учебные термины](../../CONTEXT.md).
- [Архитектура backend и правила зависимостей](../../documents/backend-architecture.md).

Реализованный HTTP API описан в [`backend.api.yaml`](backend.api.yaml).

### Генерация OpenAPI

`backend.api.yaml` генерируется из Go-типов и аннотаций HTTP-обработчиков.
Из `services/backend`:

```bash
bash scripts/generate-openapi.sh
```

В репозиторий коммитится актуальный `backend.api.yaml` вместе с изменениями кода.
CI сохраняет SHA-256 файла, запускает генерацию и сравнивает хэши;
несовпадение завершает проверку с ошибкой. Используются закреплённые в `go.mod`
Go tools `swag` и `yq`; отдельная установка CLI не нужна.
Генератор не читает этапные контракты из монорепозитория. YAML не встраивается в бинарник.

HTTP-обёртки распознавания и синтеза речи выделены в отдельный репозиторий
[tts-stt](https://github.com/AskarKasimov/tts-stt), локально — `../../../tts-stt`.
Модели разворачивает отдельная команда на выделенном сервере.

## Что работает в v0

- Регистрация, вход, refresh с ротацией и обнаружением повторного использования, выход и текущий пользователь.
- Синхронное распознавание с сохранением текста и владельца в PostgreSQL.
- Синхронный синтез: готовый `audio/wav` в теле ответа.
- Импорт одного CSV карты компетенций с атомарной заменой учебной базы; доступен только admin.

Frontend обращается только к этому backend. GigaAM и VoxCPM2 остаются внутренними HTTP API;
их адреса и параметры моделей не передаются браузеру. Для STT по умолчанию используется
`/transcribe/longform`: серверная команда должна установить long-form зависимости GigaAM
и настроить VAD-веса. Записи длиннее 25 секунд не ограничиваются публичным API.

## Запуск через Docker Compose

Требуются Docker Compose и mkcert. Из `services/backend`:

```bash
../../run.sh
```

На Windows выполняйте `run.sh` в Git Bash. Скрипт сначала устанавливает
доверие к CA mkcert и выпускает сертификаты, затем запускает Compose с пересборкой.
Подготовка `.env`, установка mkcert и команды для каждой ОС описаны
[в README монорепозитория](../../README.md#локальный-запуск).

API доступен по `https://localhost:8443/api/v1`, проверка — `GET /api/v1/health`.
HTTP-порт `127.0.0.1:8002` нужен для внутренних проверок и reverse proxy;
браузер использует HTTPS. Миграция применяется автоматически перед запуском API.
PostgreSQL хранит данные в постоянном volume. Обычный `docker compose down` сохраняет его;
`down -v` удаляет данные.

В `.env` обязательно укажите `STT_URL` и `TTS_URL` серверов моделей: DNS-имена
или IP, доступные из контейнера backend. Адреса в `.env.example` служат примерами
и требуют замены. Контейнеры моделей запускаются отдельно из `../../../tts-stt`.
Для корпоративной сети сертификат должен содержать IP в SAN, а CA должен быть доверенным
на клиентских машинах. Frontend и `/api/v1` публикуются через общий HTTPS reverse proxy;
клиент обращается к API по относительным путям. Caddy удаляет `/api/v1` перед
проксированием к маршрутам от корня (`/auth/*`, `/voice/*`, `/admin/*`).
Обе cookies устанавливаются и удаляются с `Path=/`.

## Запуск Go-процесса

Команды ниже выполняются из `services/backend`.
Требуются Go 1.26 и PostgreSQL. Приложение не загружает веса моделей и не требует Python.

```bash
export DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
export STT_URL='http://localhost:8000/transcribe/longform'
export TTS_URL='http://localhost:8001/synthesize'
go run ./cmd/api migrate
go run ./cmd/api
```

`sslmode=disable` подходит для локальной/закрытой сети Compose. Для удалённой БД настройте
TLS в `DATABASE_URL`. Go-процесс слушает HTTP за HTTPS reverse proxy. `LISTEN_ADDRESS`
по умолчанию `:8002`; `PROCESSING_TIMEOUT` — `120s`. Завершение по SIGTERM/SIGINT даёт
активным запросам до 15 секунд. Корпоративный reverse proxy должен ограничивать размер
запроса с учётом multipart overhead и иметь timeout не меньше `PROCESSING_TIMEOUT`.

## Миграции БД

Миграции выполняет Goose; SQL-файлы из `internal/shared/postgres/migrations`
встроены в бинарник. Они применяются при старте API или отдельно командой
`go run ./cmd/api migrate`. История хранится в `goose_db_version`.
PostgreSQL advisory lock сериализует запуск нескольких процессов API.

Для следующего изменения добавьте файл с новой последовательной версией,
например `internal/shared/postgres/migrations/00002_add_user_timezone.sql`:

```sql
-- +goose Up
ALTER TABLE users ADD COLUMN timezone text;

-- +goose Down
ALTER TABLE users DROP COLUMN timezone;
```

Применённые миграции не редактируйте. Каждый файл по умолчанию выполняется
в отдельной транзакции; ошибка откатывает этот файл, и следующий запуск повторяет
его. Для SQL, которому запрещена транзакция (например, `CREATE INDEX CONCURRENTLY`),
добавьте `-- +goose NO TRANSACTION` и обеспечьте безопасный повторный запуск.
Начальная миграция содержит только `Up`; автоматического удаления всех данных нет.
Команда приложения применяет только `Up`; пример `Down` предназначен для явного
отката через Goose CLI.

## Пример запросов

```bash
curl 'https://localhost:8443/api/v1/auth/register' \
  -H 'Content-Type: application/json' \
  -d '{"email":"student@example.edu","password":"My unique learning phrase 2026!","display_name":"Иван"}' \
  -c cookies.txt

curl 'https://localhost:8443/api/v1/auth/me' -b cookies.txt

curl 'https://localhost:8443/api/v1/voice/transcriptions' \
  -b cookies.txt \
  -F 'audio=@answer.webm;type=audio/webm'

curl 'https://localhost:8443/api/v1/voice/syntheses' \
  -H 'Content-Type: application/json' \
  -b cookies.txt -d '{"text":"Что обозначает дробь?"}' --output question.wav

curl 'https://localhost:8443/api/v1/auth/refresh' \
  -X POST -b cookies.txt -c cookies.txt

curl 'https://localhost:8443/api/v1/auth/logout' \
  -X POST -b cookies.txt -c cookies.txt
```

Cookies содержат секреты: не коммитьте `cookies.txt`. JSON не содержит значений токенов.
Пароли хешируются Argon2id (64 МиБ, 3 прохода, 2 потока); токены — SHA-256.
Исходный пароль сохраняет пробелы и Unicode. Помимо локального отсева слабых паролей,
регистрация проверяет утечки через [HIBP Pwned Passwords](https://haveibeenpwned.com/API/v3#PwnedPasswords).
Наружу отправляется только пятисимвольный префикс SHA-1 с padding; пароль и полный хеш
не передаются. Нужен исходящий HTTPS к `api.pwnedpasswords.com`.
Если проверка недоступна, регистрация возвращает `503 PROCESSING_UNAVAILABLE` и не создаёт аккаунт.
Вход и существующие сессии от этого сервиса не зависят.

Все auth POST имеют общий лимит 30 запросов в минуту на IP; login дополнительно — 10 в минуту
на нормализованный email. Счётчики хранятся в PostgreSQL и переживают перезапуск.
`AUTH_RATE_LIMIT` меняет общий лимит. API использует адрес TCP-клиента и не доверяет
клиентскому `X-Forwarded-For`: за reverse proxy общий лимит считается на адрес proxy;
при большом числе клиентов настройте IP-лимит на proxy и увеличьте лимит API.

## Преподаватель и CSV

Зарегистрируйте аккаунт и назначьте admin через SQL:

```bash
docker compose -f ../../docker-compose.yaml exec db psql -U ai_tutor -d ai_tutor \
  -c "UPDATE users SET role='admin' WHERE email='teacher@example.edu';"
```

Новый вход не нужен: роль применяется со следующего запроса. Затем импортируйте файл:

```bash
curl 'https://localhost:8443/api/v1/admin/competency-map/import' \
  -b teacher-cookies.txt \
  -F 'file=@internal/app/testdata/example-map.csv;type=text/csv'
```

CSV: UTF-8 с необязательным BOM, разделитель `,` или `;`, обязательные заголовки `Ком`, `Сост`, `ОР`,
далее пары `Задание N`/`Критерии N`. Пустые `Ком`, `Сост`, `ОР` наследуются независимо вниз.
Критерии сохраняются как текст с пробелами и переводами строк, без интерпретации.
Дополнительные колонки сохраняются в `outcomes.attributes` массивом атрибутов исходных строк.
Одна служебная строка распознаётся по точным пояснениям «Компетенция», «Составляющая»
(или «Составляющая РПД»), «Образовательный результат» в обязательных колонках. Поля заданий
в ней должны быть пустыми или содержать подписи «Текст задания»/«Текст вопроса»/«Задание»/«Вопрос»,
поля критериев — пустыми или «Критерии оценивания»/«Критерии оценки»/«Текст критериев»/«Критерии».
Имена вроде «Компетенция 1» считаются учебными данными. Если экспорт использует другие
пояснения, удалите служебную строку перед импортом.
Ошибки содержат физический номер CSV-строки и название колонки в `details`.
Конкурентные импорты получают разные последовательные revisions. При любой ошибке транзакция
сохраняет прежнюю базу. Отдельного курса/каталога/грейдинга в v0 ещё нет.

## Проверки

Интеграционные тесты используют настоящий PostgreSQL и отдельную случайную schema на каждый тест.
Модели и HIBP заменяются тестовыми HTTP-серверами; сетевой доступ и веса не нужны.

```bash
export TEST_DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Без `TEST_DATABASE_URL` интеграционные тесты явно пропускаются; parser/config unit tests выполняются.
CI всегда поднимает PostgreSQL. `/health` проверяет API и БД; доступность моделей проверяется
при голосовом запросе и отражается кодами `502`, `503`, `504`.
