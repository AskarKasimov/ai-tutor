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
несовпадение завершает проверку с ошибкой. Скрипт запускает `swag` и `yq` через
`go run` с закреплёнными версиями; их зависимости не входят в `go.mod` backend.
Отдельная установка CLI не нужна. Первый запуск скачивает инструменты и их зависимости;
следующие используют кэш Go.
Генератор не читает этапные контракты из монорепозитория. YAML не встраивается в бинарник.

HTTP-обёртки распознавания и синтеза речи выделены в отдельный репозиторий
[tts-stt](https://github.com/AskarKasimov/tts-stt), локально — `../../../tts-stt`.
Модели разворачивает отдельная команда на выделенном сервере.

## Что работает в v0

- Регистрация, вход, refresh с ротацией и обнаружением повторного использования, выход и текущий пользователь.
- Синхронное распознавание с сохранением текста и владельца в PostgreSQL.
- Синхронный синтез: готовый `audio/wav` в теле ответа.
- Синхронная прототипная оценка сохранённой расшифровки через локальный `gpt-oss-120b`: балл 0/1/2 и три строки фидбэка без сохранения результата.
- Импорт одного CSV карты компетенций с атомарной заменой учебной базы; доступен только admin.

Frontend обращается только к этому backend. GigaAM и VoxCPM2 остаются внутренними HTTP API;
их адреса и параметры моделей не передаются браузеру. Пример `STT_URL` использует
`/transcribe`, который выбирает обычное распознавание до 25 секунд и longform
для более длинных записей: серверная команда должна установить long-form
зависимости GigaAM и настроить VAD-веса для длинных записей. Записи длиннее
25 секунд не ограничиваются публичным API.
Оценивание принимает произвольный текст задания, варианты (необязательно), голосовую инструкцию,
эталон (необязательно) и `transcription_id`. Backend берёт текст ответа из PostgreSQL по владельцу,
передаёт его модели и проверяет структуру результата. Это прототипный синхронный маршрут,
а не целевая оценка по опубликованной рубрике и учебным материалам.

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

Все настройки backend обязательны: без них API останавливается при запуске.
Скопируйте полный набор из `.env.example`; скрытых значений по умолчанию нет.
В `STT_URL` и `TTS_URL` укажите адреса серверов моделей: DNS-имена
или IP, доступные из контейнера backend. Адреса в `.env.example` служат примерами
и требуют замены. Контейнеры моделей запускаются отдельно из `../../../tts-stt`.
`ASSESSMENT_BASE_URL`, `ASSESSMENT_MODEL` и `ASSESSMENT_TIMEOUT` также
задаются явно в окружении; укажите доступный адрес, имя модели и таймаут.
Для корпоративной сети сертификат должен содержать IP в SAN, а CA должен быть доверенным
на клиентских машинах. Frontend и `/api/v1` публикуются через общий HTTPS reverse proxy;
клиент обращается к API по относительным путям. Caddy удаляет `/api/v1` перед
проксированием к маршрутам от корня (`/auth/*`, `/voice/*`, `/admin/*`).
Обе cookies устанавливаются и удаляются с `Path=/`.

## Запуск Go-процесса

Команды ниже выполняются из `services/backend`.
Требуются Go 1.26 и PostgreSQL. Приложение не загружает веса моделей и не требует Python.

```bash
export LISTEN_ADDRESS=':8002'
export DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
export STT_URL='http://localhost:8000/transcribe'
export TTS_URL='http://localhost:8001/synthesize'
export ASSESSMENT_BASE_URL='http://localhost:30245/v1'
export ASSESSMENT_MODEL='gpt-oss-120b'
export PROCESSING_TIMEOUT='120s'
export ASSESSMENT_TIMEOUT='90s'
export MAX_UPLOAD_BYTES='26214400'
go run ./cmd/api migrate
go run ./cmd/api
```

`sslmode=disable` подходит для локальной/закрытой сети Compose. Для удалённой БД настройте
TLS в `DATABASE_URL`. Go-процесс слушает HTTP за HTTPS reverse proxy.
`LISTEN_ADDRESS` и `PROCESSING_TIMEOUT` задаются явно. Завершение по SIGTERM/SIGINT даёт
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

curl 'https://localhost:8443/api/v1/assessments/evaluate' \
  -H 'Content-Type: application/json' -b cookies.txt \
  -d '{"transcription_id":"tr-from-stt","question":"Банк прогнозирует возврат кредита. Какой тип задачи?","options":["Классификация","Регрессия"],"voice_instruction":"Назовите тип задачи и объясните выбор.","correct_answer":"Классификация: два класса."}'

curl 'https://localhost:8443/api/v1/auth/refresh' \
  -X POST -b cookies.txt -c cookies.txt

curl 'https://localhost:8443/api/v1/auth/logout' \
  -X POST -b cookies.txt -c cookies.txt
```

Cookies содержат секреты: не коммитьте `cookies.txt`. JSON не содержит значений токенов.
Пароли хешируются Argon2id (64 МиБ, 3 прохода, 2 потока); токены — SHA-256.
Исходный пароль сохраняет пробелы и Unicode. Регистрация выполняет только
локальную валидацию пароля и сохраняет его хеш в PostgreSQL.

Вход ограничен 10 попытками в минуту на нормализованный email.
Счётчики хранятся в PostgreSQL и переживают перезапуск.

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
сохраняет прежнюю базу. Отдельного курса/каталога и целевого грейдинга по рубрикам в v0 ещё нет.

## Проверки

Интеграционные тесты используют настоящий PostgreSQL и отдельную случайную schema на каждый тест.
Модели заменяются тестовыми HTTP-серверами; сетевой доступ и веса не нужны.

```bash
export TEST_DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Без `TEST_DATABASE_URL` интеграционные тесты явно пропускаются; parser/config unit tests выполняются.
CI всегда поднимает PostgreSQL. `/health` проверяет API и БД; доступность моделей проверяется
при голосовом запросе и отражается кодами `502`, `503`, `504`.
