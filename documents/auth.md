# Авторизация

Состояние: 7 октября 2026. Описано реализованное поведение. Контракт — [api/openapi.yaml](../api/openapi.yaml). Авторизация и PostgreSQL настоящие также при `BACKEND_API_MODE=mock`.

## HTTP-контракт

Пути ниже обрабатывает backend. В браузере к ним добавляется `/api/v1`; Caddy удаляет этот префикс. Register/login публичные, refresh/logout используют refresh-cookie, `/auth/me` — access-cookie. `/health` публичный; действующие учебные маршруты защищены access-cookie.

| Метод и путь | Вход | Успешный ответ |
| --- | --- | --- |
| `POST /auth/register` | JSON: email, password, необязательный display_name | 201: `{user, session}` и две cookies; новая учётная запись student сразу входит в систему. |
| `POST /auth/login` | JSON: email, password | 200: `{user, session}` и две cookies; создаётся отдельная auth-сессия. |
| `POST /auth/refresh` | Без тела, refresh_token cookie | 200: `{access_expires_at, refresh_expires_at}` и новые cookies. |
| `POST /auth/logout` | Без тела, необязательная refresh_token cookie | 204 без JSON; отзыв найденной сессии и удаление обеих cookies. |
| `GET /auth/me` | access_token cookie | 200: объект user. |

`user`: id, email, display_name (string/null), role (student/admin), created_at. `session`: access_expires_at и refresh_expires_at. Все даты — Unix seconds. Значения токенов отсутствуют в JSON.

Register/login требуют `Content-Type: application/json`, UTF-8 и один JSON-объект; лишние поля, в том числе role, отклоняются. Общий лимит тела этих запросов — 32 КиБ. Любое непустое тело refresh/logout, включая `{}`, даёт 422. Auth-ответы получают `Cache-Control: no-store` через общий middleware.

## Валидация пользователя

- Email: внешние пробелы удаляются, регистр приводится к нижнему; адрес проверяется, максимум 254 байта. Нормализованный email уникален в БД.
- Пароль регистрации: 15–128 Unicode-символов; пробелы допускаются, пароль не обрезается и не нормализуется. Локальная проверка отклоняет строку из одинаковых символов, одни пробелы и небольшой список простых комбинаций. Проверки по внешней базе утечек нет.
- Пароль входа: 1–128 символов; политика регистрации повторно не применяется.
- display_name можно не передавать. Если передан, нужна строка из 1–200 символов, не только пробелы и без NUL. Явный null отклоняется; в ответе отсутствие имени представлено null.

## Cookies и сроки

Access/refresh — непрозрачные случайные строки: 32 байта из crypto/rand, кодировка base64url без padding. Браузер получает их через **два отдельных** заголовка Set-Cookie; объединять эти заголовки через запятую нельзя.

| Cookie | Срок | Атрибуты |
| --- | --- | --- |
| access_token | `min(now + 900, session.expires_at)` | `Path=/; Secure; HttpOnly; SameSite=Lax`, без Domain. |
| refresh_token | До исходного session.expires_at | Те же атрибуты. |

Сессия действует 30 суток от регистрации/входа. Refresh не продлевает её. Max-Age при выдаче равен оставшемуся сроку в секундах; при удалении отправляется пустое значение и Max-Age=0 с теми же Path и атрибутами. Истечение проверяется по условию `expires_at <= now`.

Frontend и API публикуются на одном HTTPS origin; клиент использует относительные пути и `credentials: 'include'`. JavaScript не читает HttpOnly-токены и не сохраняет их в localStorage. Для локального браузерного доступа нужен доверенный HTTPS; запись с микрофона также требует secure context.

## Жизненный цикл сессии

1. Register сохраняет пользователя, сессию и пару токенов одной транзакцией. Login создаёт новую сессию и пару токенов, не отзывая другие входы.
2. Защищённый запрос ищет хеш access-токена, проверяет срок токена, срок/отзыв сессии и читает текущего пользователя с ролью из БД.
3. Refresh блокирует строку auth_sessions (`FOR UPDATE OF s`), проверяет refresh.used_at и активность сессии. Затем отмечает старый refresh использованным и сохраняет новую пару токенов одной транзакцией. Старый access остаётся действительным до своего срока или отзыва сессии.
4. Повтор использованного refresh отзывает **всю эту сессию**, включая старые и новые access-токены. Отзыв коммитится, затем возвращается 401 INVALID_REFRESH_TOKEN и удаляются cookies. Другие сессии пользователя сохраняются.
5. Logout отзывает сессию, найденную по refresh-токену. Отсутствующий/неизвестный токен, повторный logout или истёкшая сессия дают 204 и удаляют cookies. Если cookie отсутствует, серверу нечем определить сессию для отзыва.

При конкурентных refresh один запрос может получить 200, а второй обнаружить повтор и отозвать сессию. Клиенту нужен один выполняющийся refresh на сессию, включая координацию вкладок. Повтор после потерянного ответа также может отозвать сессию. Текущий [use-auth.ts](../services/frontend/src/data/use-auth.ts) выполняет register/login/me/logout; автоматический refresh пока не реализован.

В PostgreSQL repository после LockRefreshState отдельно перечитывает used_at. Это учитывает обновление конкурента, пока запрос ждал блокировку сессии. При изменении SQL сохранить сериализацию и это повторное чтение. Для неизвестного, истёкшего или отозванного refresh также возвращается INVALID_REFRESH_TOKEN с удалением cookies; технический отказ БД сам по себе cookies не удаляет.

## Ошибки

Формат: `{code, message, details?}`; detail содержит code, message и необязательный path. Клиенту следует различать ошибки по code.

| HTTP / code | Условие |
| --- | --- |
| 401 UNAUTHORIZED | Access-cookie отсутствует/неизвестна, истёк токен/сессия или сессия отозвана. |
| 401 INVALID_CREDENTIALS | Корректно оформленный вход с неизвестным email или неверным паролем; ответ одинаковый. |
| 401 INVALID_REFRESH_TOKEN | Refresh отсутствует/неизвестен, использован повторно или сессия неактивна. |
| 403 FORBIDDEN | Авторизованный пользователь без admin пытается импортировать карту/материалы. |
| 409 EMAIL_ALREADY_REGISTERED | Нормализованный email уже занят. |
| 422 VALIDATION_ERROR | Неверные поля, JSON, Content-Type или запрещённое тело. |
| 422 PASSWORD_TOO_WEAK | Пароль регистрации не проходит локальную политику. |
| 413 UPLOAD_TOO_LARGE | Превышен лимит тела запроса. |
| 503 PROCESSING_UNAVAILABLE | Необработанная техническая ошибка, например отказ БД. |

## Роли и доступ к данным

Регистрация назначает student. Admin получает права импорта карты и материалов, остальные учебные операции также доступны ему. Роль не принимается из публичного запроса; она перечитывается из users на каждом защищённом запросе. Admin назначается после обычной регистрации через SQL:

```sql
UPDATE users SET role = 'admin' WHERE email = 'teacher@example.edu';
```

`protect` в app получает user через auth.Me и передаёт principal в request context. Handler извлекает principal и передаёт его application; проверка admin выполняется также внутри application соответствующего модуля. Владельца ресурса берут из principal, не из клиентского user_id. Доступ к расшифровкам проверяет владельца.

Для будущих вариантов проверяются ownerID, variantID и variantTaskID; чужие/несогласованные ID должны возвращать 404. Эти маршруты пока planned. Auth-сессия и учебная сессия — разные сущности. Подтверждение email, сброс пароля, MFA, CORS и проверка Origin/CSRF-token не реализованы; ограничения по IP/числу попыток входа также отсутствуют.

## Карта кода для изменений

Пути относительно services/backend:

| Файл | Ответственность |
| --- | --- |
| [auth/application/service.go](../services/backend/internal/features/auth/application/service.go), [ports.go](../services/backend/internal/features/auth/application/ports.go) | Политика входа/регистрации, сроки, ротация/отзыв и порты. Время внедрено через now. |
| [auth/transport/http/handler.go](../services/backend/internal/features/auth/transport/http/handler.go) | DTO, Set-Cookie/удаление cookies, HTTP-контракт и Swag-аннотации. |
| [auth/infrastructure/postgres/repository.go](../services/backend/internal/features/auth/infrastructure/postgres/repository.go), [queries/auth.sql](../services/backend/internal/shared/postgres/queries/auth.sql) | Транзакции, блокировки, чтение роли и сохранение хешей. |
| [auth/infrastructure/argon2/hasher.go](../services/backend/internal/features/auth/infrastructure/argon2/hasher.go) | Argon2id: m=65536 КиБ, t=3, p=2; salt 16 байт, hash 32 байта. Verify поддерживает этот формат; неизвестный email также оплачивает вычисление хеша. |
| [shared/security/token.go](../services/backend/internal/shared/security/token.go) | Генерация токенов/ID и SHA-256 токенов. |
| [app/app.go](../services/backend/internal/app/app.go), [shared/httpx](../services/backend/internal/shared/httpx) | Подключение маршрутов, protect/principal, декодирование и ошибки. |
| [00001_initial.sql](../services/backend/internal/shared/postgres/migrations/00001_initial.sql) | users; auth_sessions с expires_at/revoked_at; access_tokens с expires_at; refresh_tokens с used_at. Удаление пользователя/сессии каскадно удаляет зависимые токены. |

В БД хранятся Argon2id-пароли и SHA-256 токенов; открытые значения токенов в таблицы не записываются. Использованные refresh сохраняются для обнаружения повторов. SQLC-код генерируется из queries и migrations, вручную не редактируется.

## Проверки при изменении auth

Из services/backend:

```bash
go test ./internal/features/auth/... -count=1
# Для PostgreSQL, HTTP-контракта, конкурентного refresh и HTTPS cookies:
TEST_DATABASE_URL='postgres://USER:PASSWORD@HOST:5432/TEST_DB?sslmode=disable' \
  go test ./internal/app -count=1
bash scripts/generate-openapi.sh
```

[service_test.go](../services/backend/internal/features/auth/application/service_test.go) проверяет фиксированный конец сессии, старый access и коммит отзыва при повторе. [auth_test.go](../services/backend/internal/app/auth_test.go) проверяет БД, роли, конкуренцию и валидацию; [contract_test.go](../services/backend/internal/app/contract_test.go) — OpenAPI и HTTPS cookie lifecycle. Без TEST_DATABASE_URL интеграционные DB-проверки пропускаются. При изменении DTO/аннотаций обновить единый OpenAPI; при изменении SQL запустить `bash scripts/generate-sql.sh`.
