# Backend

Go API и PostgreSQL. Команды выполняются из `services/backend`, если не указано иначе.

## Реализовано и запланировано

Реализованы auth, синхронные STT/TTS, импорт текущей карты CSV/XLSX, карта/каталог,
assessment по снимкам вариантов, LLM-генерация задач, импорт материалов с FTS,
variantgen (сборка, снимки, список и чтение вариантов), постоянная очередь аудио в
PostgreSQL с фоновым TTS/S3 worker и HTTP API диагностических
сессий. Состояние сессий и принятые ответы хранятся в памяти процесса; store принимает
до 10 000 сессий за время жизни процесса, затем возвращает 503 до перезапуска. Перезапуск
backend завершает доступ к сессиям. Итоговый фидбэк и тренировка не реализованы.

Единый контракт — [api/openapi.yaml](../../api/openapi.yaml): implemented — работает,
planned — проект. `/tasks/generate` и `/admin/materials` сохраняются как API отдельного taskgen.
`/assessments/evaluate` принимает позицию сохранённого варианта. Диагностическая сессия вызывает тот же assessment
application по `variant_id`, `variant_task_id` и `transcription_id`, без HTTP-вызова
собственного backend. В mock и real режимах используются соответствующие assessment graders.
Assessment напрямую реализует порт сессии; оба модуля используют результат из `entities/assessment`.

Сессии доступны через `POST /diagnostic-sessions`, `GET /diagnostic-sessions/{id}`,
`GET /diagnostic-sessions/{id}/current/audio?variant_task_id=...`,
`POST /diagnostic-sessions/{id}/current/audio/regenerate`,
`GET /task-audio/{id}/file`, `POST /diagnostic-sessions/{id}/answers`
и `GET /diagnostic-sessions/{id}/result`. Ответы проходят STT и, при доступном
грейдере, сохраняют score/max_score/verdict/criterion_results/feedback от assessment.
Шкала уже ограничена ролью задания и сессией не преобразуется. Повтор ключа идемпотентен; параллельный ответ
отклоняется, пока текущий обрабатывается. Assessment возвращает `max_score` по роли:
2 для main и 1 для basic; сессия сохраняет эту оценку без повторного преобразования.
Результат доступен только после завершения.
Контракт — единый [OpenAPI](../../api/openapi.yaml).

### Сохранённая озвучка заданий

Импорт карты и taskgen в той же транзакции создают audio asset и ставят его в
`pending`. Worker стартует только из `cmd/api` после миграций; `Handler()` и
healthcheck его не запускают. Он ограничен `BACKEND_AUDIO_WORKERS`, проверяет S3
объект перед TTS, сохраняет WAV и завершает запись со стабильным file URL. Пять
неудачных попыток дают `failed`; после рестарта истёкшая lease актуального задания
восстанавливается. Claim и проверка перед TTS требуют связь с активной ревизией
карты; при замене карты устаревшие pending/processing assets переходят в
терминальный `cancelled`. Уже готовые assets доступны историческим снимкам. S3 outage не блокирует импорт, текстовые задания или
ответы. `/health` проверяет только API и PostgreSQL.

По умолчанию локальный Compose запускает RustFS. Настройки `BACKEND_S3_*` и
`BACKEND_AUDIO_*` описаны в корневых `.env.example` и `.env.prod.example`;
внешний endpoint должен указывать на существующую закрытую корзину, с
`BACKEND_S3_CREATE_BUCKET=false`. Браузер обращается к
`GET /diagnostic-sessions/{id}/current/audio?variant_task_id=...` за статусом, затем
к authenticated `GET /task-audio/{id}/file` только для готовой озвучки.
Если сохранённый WAV отсутствует или повреждён, клиент может один раз вызвать
`POST /diagnostic-sessions/{id}/current/audio/regenerate` с текущим
`variant_task_id`. Сервер проверяет сохранённый объект и возвращает его без TTS,
если файл исправен; иначе синхронно восстанавливает WAV из сохранённой инструкции,
только пока задание принадлежит активной ревизии карты. Вызов требует cookie-auth
и ограничен общим лимитом TTS worker; `401`/`403` и ошибки S3 не приводят к синтезу.

Для безопасной диагностики очереди можно прочитать только метаданные, не текст
инструкции:

```sql
SELECT id, status, attempts, last_error_code, bucket, storage_uri, updated_at
FROM audio_assets
ORDER BY created_at, id;
```

Правила variantgen — [алгоритм](../../documents/Алгоритм_составления_варианта.md).
Преподаватель не размечает базовые связи и не загружает дополнительные документы.
Компетенция должна иметь хотя бы один готовый TRUE-ОР с допустимыми таксономией и важностью.
Основной выбирается по сумме ранга Блума и важности, к нему добавляются до двух
базовых с меньшим Блумом. При отсутствии базовых основной остаётся. Версия алгоритма —
`competency-map-v2`; сохранённые варианты v1 сохраняют прежний состав.
Снимки сохраняются в PostgreSQL и читаются независимо от активной карты; публичный API
возвращает main и массив basic длиной 0–2, но
скрывает эталоны, criteria и ОС. Внутренний reader с проверкой владельца отдаёт
грейдингу полный снимок, включая связанные свойства карты; composition root предоставляет
его через доменный `variant.TaskReader`. В исходной карте ОС хранится текстом в `educational_content`, отдельного ID ОС нет. Ответы, оценка и тренировка остаются вне variantgen.

## Единый OpenAPI

```bash
bash scripts/generate-openapi.sh
```

Swag генерирует реализованный API во временный файл; merge-openapi обновляет
implemented-операции, уже включённые в единый контракт, и сохраняет planned.
Исключённые операции не возвращаются автоматически. Новую работающую ручку
нужно явно добавить/перевести из planned в implemented в контракте.
Неиспользуемые схемы удаляются. Swagger использует этот же YAML.

CI проверяет повторяемость генерации и валидирует реальные ответы по implemented-
схемам. Planned-схемы проверяются отдельно без объявления их работающими.
Swag/yq запускаются через go run с закреплёнными версиями, первый запуск требует
загрузки инструментов. YAML не встраивается в API-бинарник.

## Запуск

Общий dev-стек запускается из корня через `./dev.sh -d` после настройки `.env`.
Нужны Docker Compose и mkcert. Серверный `./prod.sh` использует готовые сертификаты
и сеть `ai-tutor_default`; параметры находятся в `.env.prod.example`.

Для отдельного backend Compose из этой папки:

```bash
docker compose --env-file ../../.env up -d --build
```

Уже должны работать db и внешняя сеть. BACKEND_DATABASE_URL переопределяет
подключение; по умолчанию db:5432 и DB_PASSWORD. API слушает :8002 за Caddy HTTPS.
Публичный префикс /api/v1 удаляется proxy. Secure cookies требуют HTTPS.

BACKEND_API_MODE=mock подменяет модели, auth/БД/S3 настоящие. В real нужны все
BACKEND_STT_URL, BACKEND_TTS_* (URL и параметры голоса), BACKEND_ASSESSMENT_* и BACKEND_TASKGEN_* из
корневого шаблона: taskgen сохраняется отдельным действующим модулем. Таймауты, адрес
прослушивания и лимит загрузки обязательны. После изменения env пересоздайте API.
Все запросы VoxCPM, включая фоновую озвучку заданий, передают `BACKEND_TTS_SEED`,
`BACKEND_TTS_CFG_VALUE` (0–5) и `BACKEND_TTS_INFERENCE_TIMESTEPS` (1–50).
Шаблоны env задают соответственно 17, 2 и 50. Уже сохранённые WAV в S3 сохраняют прежнее звучание.
HTTP write timeout учитывает последовательные STT и грейдинг: максимум суммы их
таймаутов и таймаута taskgen, плюс 30 секунд для обработки и записи ответа.
Аудиоответ диагностики имеет тот же лимит загрузки, что `/voice/transcriptions`.
VITE_API_MODE=real нужен для проверки backend из frontend; иначе браузер использует свои моки.

COMPOSE_PROFILES=swagger включает Swagger под /docs/. После обновления YAML
пересоздайте swagger, потому что образ копирует спецификацию при старте.
Frontend и API публикуются одним HTTPS origin. Health проверяет API/БД, не модели.

Из этой папки для Go-процесса нужны Go 1.26 и PostgreSQL. Полный набор env — в
корневом .env.example; приложение само файл .env не читает.

```bash
go run ./cmd/api migrate
go run ./cmd/api
```

STT принимает multipart audio (WAV/Ogg/WebM) до 25 МиБ, сохраняет расшифровку и
возвращает id/text/created_at. TTS принимает text до 500 символов и возвращает WAV.
Для длинных записей удалённому GigaAM нужны longform/VAD зависимости.
HTTP модели разворачиваются отдельно; frontend не знает их адресов.

## Live reload в Docker

Корневой `./dev.sh -d` использует Dockerfile target `dev` с Air.
Исходники смонтированы в `/src`; Air пересобирает и перезапускает API при изменениях
Go-файлов, SQL, `go.mod` и `go.sum`. Конфигурация watcher — `.air.toml`;
polling позволяет подхватывать изменения через Docker Desktop.
Модули Go, кеш сборки и каталог `.air` с dev-бинарником находятся в Docker volumes.
Миграции применяются штатным запуском API; SQLC-запросы требуют отдельной генерации Go-кода.
Режим моделей остаётся значением `BACKEND_API_MODE` из `.env`.
Локальный Compose backend продолжает использовать готовый production-бинарник.

## БД и импорт

Goose применяет встроенные SQL-миграции при старте или migrate; запуск сериализован
advisory lock. Базовая схема находится в 00001_initial.sql; варианты добавлены
миграцией 00002_variants.sql, а `audio_assets`, очередь и links — миграцией
00003_task_audio.sql. Существующий volume обновляется без удаления пользователей,
расшифровок или активной карты.

Admin назначается после регистрации:

```bash
docker compose -f ../../docker-compose.yaml exec db psql -U ai_tutor -d ai_tutor \
  -c "UPDATE users SET role='admin' WHERE email='teacher@example.edu';"
```

Затем загрузите текущую карту ML:

```bash
curl 'https://localhost:8443/api/v1/admin/competency-map/import' \
  -b teacher-cookies.txt -F 'file=@map.xlsx'
```

CSV — UTF-8, разделитель запятая или точка с запятой, необязательный BOM.
XLSX — один лист карты. Текущий ML формат сохраняет колонки компетенции, уровня темы,
составляющей, ОР, TRUE/FALSE, таксономии, ALDs, важности, РПД, Задание N и ОС.
Парсер распознаёт произвольное число Задание N. Пустые родительские поля наследуются
внутри блока; профиль ОР нормализуется в типизированные свойства.
Полные ячейки содержат Экран:, необязательные варианты, Голосовая инструкция: и Ответ:.
Неполные сохраняются как source с предупреждением, не попадают в tasks.
Старый парный CSV с Ком/Сост/ОР и Критерии N также поддержан нынешним кодом.

Импорт атомарно заменяет карту и выдаёт новые ID/revision; ошибка сохраняет прежнюю
активную карту. Пользователи и расшифровки не удаляются при обычном импорте.
Текущий CSV даёт 13 компетенций, 17 составляющих, 101 ОР, 18 TRUE, 13 полных
заданий и 41 неполную ячейку. Для варианта отдельно подсчитываются готовые TRUE-ОР каждой компетенции.

GET /competency-map читает активную карту одним запросом; GET /tasks фильтрует
задания; GET /tasks/{id} возвращает профиль. Эталоны/критерии студенту не выдаются.
Фильтры `/tasks` и пагинация `/variants` отклоняют неизвестные, повторные, пустые
и некорректно закодированные query-параметры с 422.
`POST /variants` создаёт снимок, `GET /variants` возвращает cursor-страницы истории,
`GET /variants/{id}` — весь план main/basic, а `GET /variants/{id}/tasks/{task_id}` —
отдельную позицию. Все ручки требуют авторизацию и ограничивают чтение владельцем.
Повтор успешного POST с тем же `Idempotency-Key` возвращает тот же вариант.
`POST /assessments/evaluate` принимает `transcription_id` и пару `variant_id` /
`variant_task_id`, читает доверенный снимок варианта и возвращает `score`, `max_score`,
`verdict`, результаты критериев и три строки feedback. Результат не сохраняется
assessment-модулем; диагностическая сессия хранит принятую оценку в своём процессе.

## Проверки

```bash
export TEST_DATABASE_URL='postgres://ai_tutor:YOUR_PASSWORD@localhost:5432/ai_tutor?sslmode=disable'
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
```

Интеграционные тесты создают отдельную PostgreSQL schema, модели заменяются локальными
HTTP-серверами. Без TEST_DATABASE_URL DB-тесты пропускаются; CI поднимает PostgreSQL.
SQLC: `bash scripts/generate-sql.sh`.
Архитектура: [backend-architecture.md](../../documents/backend-architecture.md).
