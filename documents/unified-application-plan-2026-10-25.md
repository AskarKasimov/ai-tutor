# Объединение двух версий AI Tutor без изменения пользовательского поведения

Дата: 25.10.2026. Статус: передан к исполнению Luna по поручению владельца; реализация на момент передачи не начата.

**Исполнение:** Luna выполняет задачи самостоятельно и последовательно, без дальнейшего делегирования. Владелец поручил исполнение 25.10.2026 в ветке `feat/unify-learning-exam` на том же checkout; дополнительные worktree не создавать. Использовать `superpowers:executing-plans` с этими явными ограничениями владельца. Этот документ содержит и проектное решение, и план его реализации.

**Цель:** перенести нынешние «Обучение» и «Симуляцию экзамена» в одну кодовую базу frontend и backend, сохранив их экраны, учебные алгоритмы и пользовательские сценарии.

**Архитектура:** один React frontend, один Go backend, одна PostgreSQL, общий auth, STT/TTS, S3 и техническая инфраструктура. Учебные модули, исходные данные, списки курсов, прогресс и административные сценарии режимов остаются раздельными. Выбор режима — обычная навигация приложения, а не выбор запущенной версии Git.

**Стек:** существующие React/TanStack Router/React Query/Radix/i18n, Go/pgx/SQLC/Goose, Python course-graph, LLM gateway/gRPC, RustFS, Caddy, Docker Compose. Новые платформы оркестрации не нужны.

Документ лежит в `documents/`, поскольку `docs/superpowers/` в этом репозитории исключён из Git. Существующий незакоммиченный `documents/ci-cd-audit-2026-10-23.md` не изменять.

## 1. Что действительно согласовано

Последняя формулировка цели пользователя имеет приоритет над промежуточными ответами.

| Решение | Требование |
| --- | --- |
| Внешнее поведение | Сохранить то, что пользователи видят и делают на нынешнем dual-стенде. Объединение не является перепроектированием продукта. |
| Режимы | Две параллельные опции: прежнее «Обучение» и нынешняя «Симуляция экзамена». |
| Диагностика обучения | Сохранить правила v0.0.2.5: основные/базовые задания, баллы, переходы, отчёт и основание учебного плана. Geek её не заменяет. |
| Экзамен | Сохранить нынешний Geek, его граф, оценки A–D, обход и результаты. |
| Данные режимов | Сохранить раздельную подготовку данных преподавателем. Ранее обсуждавшиеся общий граф и общий каталог не являются требованиями. |
| Адреса | Разрешено заменить поддомены режимов на `/learn` и `/exam` одного сайта. Это согласованное внешнее отличие. |
| Аккаунты | Один настоящий аккаунт открывает оба режима. Внешне это сохраняет нынешний замысел единого входа; синхронизация двух аккаунтов исчезает. |
| Старые данные | Реальных пользователей и развёрнутых реальных инстансов нет. Миграция аккаунтов, паролей и истории между двумя релизами не нужна. |
| Запуск | Удалить обвязку запуска двух версий; оставить `dev.sh` и `prod.sh`. Не добавлять замену в виде новых deployment-команд и Windows-скриптов. |
| Объём работы | План подготовлен; следующим сообщением владелец поручил создать ветку в этом checkout и передать исполнение Luna. Перенос и удаление выполняются в рамках реализации, не выдаются за уже сделанное при составлении плана. |

«Всё кроме dev.sh/prod.sh» относится к обвязке деплоя двух версий. Compose, Dockerfile, конфигурация Caddy и служебные генераторы нужны этим двум командам и сохраняются. Иначе сами команды перестанут работать.

### Глобальные ограничения

- Одна исходная версия приложения и одна сборка каждого собственного сервиса; никаких release worktree в штатном запуске.
- Никакого iframe, второго React-приложения, отдельного legacy backend или runtime-проксирования на старую версию.
- Одна цепочка миграций. Старые миграции с номерами 1–9 не добавлять рядом с нынешними 1–10.
- Учебные различия не устранять ради одинаковых интерфейсов. Общими делать только действительно одинаковые технические части.
- Русские строки frontend — через i18n; сохранить Markdown, формулы, варианты ответа и форматирование заданий.
- Сгенерированные SQLC, OpenAPI, protobuf и `routeTree.gen.ts` менять через генераторы.
- Не менять правила преподавательского входа, парольную политику и разделение student/admin sessions в этой работе.
- Не удалять локальные volumes, секреты и чужие worktree автоматически.

## 2. На чём основан анализ

Анализ выполнен по локальному Git и исходникам, без агентов.

| Источник | Зафиксированная версия |
| --- | --- |
| Основной checkout `ai-tutor/`, master | `c8f63935a8a80c0d890be70f46479c3bae343129` |
| Обучение, тег `v0.0.2.5` | `f915c9cc79710d5d7b5c43ac82190f8ca86e9a15` |
| `ai-tutor-ci-refresh/` | Отдельный worktree той же истории, на момент анализа тот же HEAD; не второе продуктовое приложение. |
| Текущая обвязка | [deploy/dual/README.md](../deploy/dual/README.md), [Caddyfile](../deploy/dual/Caddyfile), [стартовая](../deploy/dual/start/index.html), [linux-dual.sh](../deploy/linux-dual.sh), PowerShell- и reset-скрипты. |
| Правила проекта | [AGENTS.md](../AGENTS.md), правила backend/frontend/course-graph, [README](../README.md), [auth](auth.md), [архитектура backend](backend-architecture.md). |

Старая версия изучалась через `git show v0.0.2.5:<путь>`, без создания или запуска второй копии приложения. Перед реализацией повторно зафиксировать SHA: HEAD может измениться, а `DUAL_EXAM_REF` у конкретного разработчика может отличаться от значения по умолчанию `master`.

Ограничение доказательств: Docker CLI доступен, но daemon во время анализа не запущен. Работающие экраны, реальные модели и сквозные сценарии сейчас не проверены. Визуальный эталон и baseline-прогоны — первая задача реализации, а не уже полученный результат.

### 2.1. Как устроено сейчас

1. Стартовая — отдельный статический HTML с собственными формами и JS авторизации.
2. `learn.<host>` ведёт на v0.0.2.5, `exam.<host>` — на master/выбранный release ref.
3. Каждый релиз имеет свой backend/frontend/Caddy, БД, S3, сеть и `.env`; общий Caddy выбирает релиз по имени хоста.
4. Вход выполняется в оба backend. Если аккаунта во втором нет, стартовая пытается его создать. Частичная регистрация и несовпадение паролей — последствия этой архитектуры.
5. `VITE_MODE_SELECT_URL` заставляет приложения делать полную загрузку стартовой; отсутствие сессии в режиме приводит туда с `?logout`.
6. Учебный код обучения удалён из master 16.10.2026. Его нельзя включить флагом — его нужно перенести из тега и адаптировать.
7. Даже обычный `prod.sh` сейчас поднимает сервисы отдельными Compose-проектами. Это самостоятельный слой сложности; второй механизм развёртывания вместо dual вводить не нужно.

### 2.2. Реальные коллизии при переносе

| Место | Почему механическое объединение сломает продукт |
| --- | --- |
| `/diagnostic-sessions` | В обеих версиях одинаковый URL обозначает разные DTO, состояния и алгоритмы. |
| `features/diagnostic-session`, `pages/trainer`, `entities/competency-map` frontend | Имена совпадают, ответственность и содержание различаются. В master `competency-map` обслуживает также материалы курса. |
| `subjects` и `ready` | В master готовность определяется графом. В обучении — пригодной картой и заданиями. Один фильтр `ready` скроет часть курсов либо даст нерабочую кнопку старта. |
| Миграции Goose | В обеих историях есть `00001_initial.sql`, а номера 2–9 имеют разный смысл. Старую БД нельзя открыть новым бинарником как будто это та же схема. |
| Auth и session cache | Две отдельные копии frontend могли иметь одинаковые query/storage keys. В одном origin они начнут пересекаться. |
| XLSX | Обучение читает второй лист многолистовой книги; экзамен — первый. Парсеры и правила выбора ОР различаются. |
| Bloom | В обучении `оценивать` и `создавать` нормализуются в `analysis` (четырёхуровневый выбор). В нынешнем графе карты — шесть рангов. Это влияет на выбор основного задания. |
| Модели | Обучение вызывает модели напрямую через `BACKEND_ASSESSMENT_*`, `BACKEND_TUTOR_*`, `BACKEND_TASKGEN_*`; master — gateway. Замена на текущий Geek adapter меняет промпты и параметры. |
| Таймауты | Старый `/open` и проверка практики выполняют несколько последовательных LLM-вызовов. Нынешний `HTTPWriteTimeout()` не гарантирует их полный бюджет. |
| S3 | Старому taskaudio нужны `HEAD`, metadata `audio-asset-id`, различение отсутствующих объекта/бакета. Нынешний общий S3 adapter этого полностью не предоставляет. |
| Старые snapshot | Переимпорт карты удаляет живые задания, но вариант/диагностика/план должны продолжать работать по сохранённым снимкам. |

Различие с текущим README: таблица решений от 19.10 требует запускать релизы отдельно и не возвращать старый код в master. Новый запрос явно заменяет это архитектурное решение. До реализации README остаётся описанием существующего состояния; при реализации записать новое решение с датой, не выдавая план за уже работающее поведение.

## 3. Пользовательский контракт, который нельзя потерять

### 3.1. Стартовая, вход и навигация

- До входа — нынешние формы входа/регистрации и их тексты; после входа — профиль и карточка выбора из двух режимов.
- Одна регистрация создаёт одного пользователя. Нет второго пароля, второго аккаунта и ожидания двух backend.
- Прямой заход в любой режим без сессии приводит на стартовую, как сейчас. Не вводить несогласованный сценарий автоматического возврата в глубоко вложенный экран после входа.
- В обоих режимах остаются «К выбору режима» и «Выйти». Выход завершает общую student session и делает оба режима недоступными.
- Сетевой сбой проверки сессии показывает ошибку с повтором, а не имитирует logout.
- Сессия преподавателя остаётся отдельной от student session: открытие админки не подменяет аккаунт студента в соседней вкладке.
- После разрешённой замены поддоменов обычные переходы выполняет TanStack Router. Полные переходы между версиями исчезают.

### 3.2. «Обучение»: сохраняем именно v0.0.2.5

- Сохраняются главная режима, продолжение незавершённого процесса, диагностика, её итог, выбор времени и план с теорией/практикой.
- Отбор варианта: по компетенциям в порядке карты; минимум три разных пригодных ОР; основное — максимальный нормализованный Bloom, затем важность, затем прежние tie-breakers; два базовых — самые простые из остальных, по прежнему порядку.
- Вариант выбирает и сохраняет конкретное задание каждого ОР. Повтор запроса не перебрасывает случайный выбор.
- Основное оценивается только 0/1/3; базовое — 0/1. Полностью верное основное пропускает оба базовых. Частичное, неверное и пропуск продолжают прежний обход. Максимум — 3 балла на включённую компетенцию, а не сумма максимальных баллов всех предъявляемых заданий.
- Пропуск не вызывает STT/LLM и не превращается в доказательство полного отсутствия знаний.
- «Изучить полностью» выводится только из нулевого основного и двух разных нулевых базовых ОР одной известной составляющей, без пропуска. Нельзя расширять вывод на всю компетенцию по одному ответу.
- Переимпорт не изменяет уже созданные варианты, принятые ответы, фидбэк, планы и сообщения уроков.
- План по времени сохраняет прежние пределы 1–100 часов, значения минут из карты, defaults 15+15, уровни `brief/standard/deep`, порядок и правила `in_plan`. Не улучшать этот алгоритм одновременно с переносом.
- Теория сохраняется и показывается до завершения подготовки практики; повтор `/open` продолжает с неё. Сохраняются три учебные попытки, проверка по уже объяснённой теории, фидбэк, новый случай после ошибки и состояние повторения.
- Не добавлять голосовую инструкцию в видимый текст заданий, если в v0.0.2.5 она там не показывается. Внутреннее поле, используемое snapshot/аудио, не является разрешением менять разметку задания.

По исходникам frontend цепочка после диагностики ведёт именно в `LearningPlanEntry`: `pages/trainer/ui/learning-home.tsx` → `learning-plan.tsx`. `bootstrap/dependencies.ts` подключает диагностический API и API плана; самостоятельные `training-sessions`/Socratic chat и taskgen в этот student flow не подключены. Админка обучения подключает создание курса, чтение и импорт карты. Это уже установлено чтением кода, а не оставлено будущему исполнителю как загадка.

Однако старый backend регистрирует эти дополнительные implemented API. Чтобы объединение не превратилось в скрытое урезание старого приложения, план сохраняет и их под `/api/v1/learn`: `training`, `taskbank`, `taskgen`, `assessment` и полный variant API. Новые кнопки к ним не добавляются. Их последующее удаление — отдельное решение, не часть этой интеграции.

### 3.3. «Симуляция экзамена»: сохраняем master

- Главная, выбор курса, история, продолжение, диалог Geek, озвучка частей реплики и переход к итогу после завершающего аудио.
- Существующие К1–К4, `answer_complete`, непроверенные К3/К4, шкала A–D, лимит вопросов, обход и зачёт пререквизитов.
- Материалы/карта как альтернативные источники опубликованного графа; последняя успешная сборка заменяет граф по действующим правилам.
- Пересборка графа, отмена активной диагностики, сохранение исторического результата и проверки редакций raw-материалов работают как сейчас.
- Ссылки на источники и разборы не становятся доступны без проверки владельца сессии.

### 3.4. Преподаватель и исходные данные

- У режимов остаются свои административные экраны и отдельные списки курсов. Совпадающее название курса не объединяет записи.
- В обучении — прежний импорт CSV/XLSX, выбор второго листа при наличии нескольких и первого при единственном; те же предупреждения и правила разбора заданий/критериев.
- В экзамене — нынешние загрузка материалов, метаданные недели/вида, OCR, граф и отдельный импорт XLSX с нынешним выбором первого листа.
- Загрузка/переимпорт в одном режиме не создаёт, не заменяет и не инвалидирует данные другого.
- Не вводить общий парсер предметных полей, общий граф, новые требования к шаблону карты или конвейер генерации базовых вопросов из PDF.

## 4. Целевая структура приложения

### 4.1. Что объединяем, а что сохраняем отдельным

| Общее | Раздельное |
| --- | --- |
| Пользователь, пароль, auth API, refresh/revoke | Диагностика по карте и экзамен Geek |
| HTTP server, logging, request ID, fault envelope | DTO, состояния, оценки и результаты двух диагностик |
| PostgreSQL pool и цепочка миграций | Таблицы и запросы учебных данных режимов |
| STT/TTS clients, browser recorder как технический адаптер | Жизненный цикл голоса/ответа конкретного учебного процесса |
| S3 transport, секреты подключения | Ключи объектов, очереди и правила владения/инвалидации |
| Frontend providers, auth/session epoch, API refresh lock, базовый UI | Страницы режимов, hooks, query keys, типы и переводные строки предметных сценариев |
| Caddy и Compose stack | Наполнение админок и критерии готовности курса |
| Gateway transport | Промпты, валидация результатов, бюджеты и политики вызовов обучения/Geek |

### 4.2. URL

| Экран/контракт | Целевой путь |
| --- | --- |
| Стартовая: auth или выбор режима | `/` |
| Обучение | `/learn` |
| Экзамен | `/exam` |
| Админка обучения | `/admin/learn/competency-map` |
| Админка экзамена | `/admin/exam/courses`, `/admin/exam/competency-map`, `/admin/exam/competency-map/$subjectId` |
| Служебный `/admin` | Redirect на `/admin/exam/courses`; второй административный адрес печатает launcher. |
| Общие auth и voice | `/api/v1/auth/*`, `/api/v1/voice/*` |
| Учебный API обучения | `/api/v1/learn/*` |
| Учебный API экзамена | `/api/v1/exam/*` |

Одинаковые `/diagnostic-sessions` получают разные namespace и разные схемы. Маршруты режимов собираются явно в `internal/app`, а не выбираются переменной `RELEASE`, Host header или переданным клиентом именем модели. `/api/v1/health` остаётся технической общей проверкой; gateway/docs сохраняют собственные действующие служебные пути.

Старые неименованные API-пути не обслуживать условно по Referer. Реальных инстансов нет: frontend и контракт переключаются одним изменением. Исторические UI-ссылки в сохранённых audio DTO должны формироваться сразу с правильным namespace.

### 4.3. Backend

- Существующий `features/geek` и `entities/exam` остаются владельцами экзамена. Не называть старую диагностику «ещё одним режимом Geek».
- Возвращаются отдельные `entities/{assessment,audioasset,competencymap,diagnostic,learning,variant,training}` и features `assessment`, `competency`, `diagnostic`, `diagnosticfeedback`, `learning`, `taskaudio`, `taskbank`, `taskgen`, `training`, `variantgen` из тега. Имена отсутствующих ныне модулей можно сохранить.
- `features/{auth,voice,subject}` не копировать из старого релиза поверх нынешних. Адаптировать их потребителей к одной действующей реализации.
- Composition root разделить на `internal/app/learning.go` и `internal/app/exam.go` с регистрацией `registerLearningRoutes(mux *http.ServeMux)` и `registerExamRoutes(mux *http.ServeMux)`. Инициализация общих ресурсов — в существующем `New/Close/RunWorkers`.
- Features связывает app через порты; не ослаблять `architecture_test.go`, не добавлять импорты между feature slices.
- Не переносить целиком старый `app.go`, старую конфигурацию, `.env` или сгенерированные SQLC-файлы.
- В lifecycle приложения включить очередь taskaudio и необходимые training workers; их остановка, освобождение lease и закрытие соединений должны работать вместе с нынешними workers.

### 4.4. Frontend

- Одна bootstrap-композиция, один QueryClient на экземпляр приложения, общий auth и один route tree.
- `pages/mode-selection` воспроизводит нынешнюю стартовую, используя общий auth; статический HTML переносится по поведению/виду, а не встраивается отдельным документом.
- Существующий `pages/trainer` оставить владельцем экзамена, подключить к `/exam`. Избежать ненужного массового переименования работающего Geek.
- Страницы обучения перенести в `pages/learning-trainer`; предметные slices — `entities/learning-diagnostic`, `features/learning-diagnostic`, `entities/learning-plan`, `features/learning-plan`, `entities/learning-map`, `features/import-learning-map`, `entities/learning-assessment` (форматирование старого фидбэка и связанные типы).
- Для административного экрана обучения — `pages/learning-competency-map`; нынешние страницы материалов/графа не перезаписывать.
- Общий компонент записи можно переиспользовать через публичный shared API, но orchestration hooks разных диагностик не объединять насильно.
- Разнести i18n namespaces старого и нового trainer. Не делать поверхностный merge объектов переводов с молчаливой заменой одинаковых ключей.
- Пример query key: `['learn','diagnostic-session', userId, sessionId]` против `['exam','diagnostic-session', userId, sessionId]`. Все history/start/result/answer/plan keys содержат режим и владельца; course-specific keys — ещё и course ID.
- Browser storage также разделить по режиму и user ID. Переход между режимами не должен стереть intent обучения или восстановить чужую сессию.
- Auth cache/epoch и незавершённый refresh дополнительно различают portal `student/admin`. Portal фиксируется при старте запроса: ответ на student request не должен повторяться с admin cookies только потому, что адрес успел измениться. Нельзя использовать один закэшированный `auth` user для student и teacher при переходе в пределах одного SPA.

Технические интерфейсы общего каталожного транспорта: `type StudyMode = 'learn' | 'exam'`, `listSubjects(mode: StudyMode, signal: AbortSignal): Promise<Subject[]>`, `createSubject(mode: StudyMode, name: string, signal: AbortSignal): Promise<Subject>`, `useSubjectsQuery(userId: string, mode: StudyMode)`. Тип `Subject` сохраняет `{id,name,ready}`; mode выбирает API namespace и query key. `readLearningState` принадлежит learning-diagnostic slice, не общему экземпляру Geek API. Все нынешние exam call sites передают `exam` явно.

### 4.5. Данные

База физически одна. Для явного владения таблицами обучения использовать префикс `learning_`; нынешние `geek_*`, `course_graph_*` и `subject_materials` сохраняются. Отдельные схемы с разным `search_path` и две независимые Goose history не нужны.

Таблица `subjects` остаётся общей технической сущностью, но получает `mode IN ('learn','exam')`. Существующие записи относятся к `exam`; создание и чтение из namespace режима используют фиксированный mode на сервере. Это два раздельных каталога внутри одной таблицы, а не общий продуктовый каталог. Клиент не задаёт произвольный mode в теле создания курса. Scope проверяется не только в списке: каждый start/import/upload/build/read/delete обязан отвергать ID курса другого режима до запуска модели или изменения данных; то же относится к admin, имеющему доступ к обеим панелям.

Основные группы новых таблиц:

| Группа | Таблицы/данные |
| --- | --- |
| Карта | `learning_map_state`, `learning_map_imports`, `learning_map_source_rows`, `learning_competencies`, `learning_constituents`, `learning_outcomes`, связанные справочники и curriculum-связи, необходимые реальному парсеру/запросам. |
| Задания и варианты | `learning_tasks`, `learning_variants`, `learning_variant_tasks`; immutable task/profile snapshot и nullable ссылка на живое задание. |
| Диагностика | `learning_diagnostic_sessions`, `learning_diagnostic_answers`, `learning_diagnostic_feedback`; start/answer idempotency, reservation/lease и сохранённые разборы. |
| План | `learning_plans`, `learning_messages`; snapshot тем/ОР, статусы, часы, pending answer и сообщения. |
| Аудио | `learning_audio_assets`, очередь, приоритеты, claim token, попытки и ссылки на объекты. |
| Дополнительная тренировка | `learning_training_sessions`, `learning_training_start_requests`, `learning_training_targets`, `learning_training_exercises`, `learning_training_attempts`, `learning_training_chats`, `learning_training_chat_messages`, `learning_training_chat_turns`. |
| Taskgen/FTS | `learning_generation_runs`, `learning_generation_run_examples`, `learning_generation_run_chunks`, `learning_material_chunks`, `learning_material_chunk_outcomes`; taskbank читает общий для обучения `learning_tasks`. |

Не восстанавливать старые `users`, auth/token tables, `subjects`, `transcriptions` второй раз. Старое `subjects.active_revision` заменить чтением состояния обучения; в экзаменный каталог эту колонку и её семантику не протаскивать.

SQLC query names — с префиксом `Learning`, файлы — `learning_*.sql`. Исторические foreign keys, unique constraints, CAS, idempotency и `ON DELETE SET NULL` для snapshot-ссылок сохранить по смыслу после переименования таблиц.

Полный набор старых query owners проверен: `competency.sql` содержит также taskbank/taskgen/FTS, отдельно существуют `variants.sql`, `diagnostic.sql`, `diagnostic_feedback.sql`, `task_audio.sql`, `training.sql`, `training_chat.sql`, `learning.sql`. `auth.sql` и `voice.sql` не копируются. Старые initial-таблицы `learning_sessions`, `task_assignments`, `assignment_responses`, `material_embeddings` не имеют владельцев в изученном implemented call graph: не восстанавливать их только потому, что они есть в DDL. На шаге SQLC проверить отсутствие ссылок; при обнаруженной транзитивной зависимости перенести её вместе с конкретным владельцем и описать причину.

Готовность learning subject воспроизводит старый `queries/subject.sql`: хотя бы одна компетенция с тремя разными TRUE ОР, допустимыми taxonomy/importance и готовыми authored/ai_generated tasks с непустыми question/voice_instruction/reference_answer. Это не проверка наличия строк в карте. Экзаменный `ready` остаётся основанным на опубликованном графе по нынешнему SQL. Admin видит и неготовые курсы своего режима; student — только готовые.

Продолжать текущую цепочку новыми миграциями после 10; точные номера назначить по HEAD при реализации. Предложенная разбивка: curriculum/tasks → variants/diagnostic/audio → plan/training. Не переписывать нынешнюю initial migration: отсутствие реальных пользователей позволяет не делать ETL, но само по себе не требует уничтожать рабочую цепочку миграций.

## 5. Модели, аудио и длительные операции

### 5.1. Перенос LLM — отдельная задача совместимости

Нельзя заменить старые model clients вызовом `geek.Gateway.Evaluate`: это другой prompt, другой результат и другая шкала.

Для `assessment`, `diagnosticfeedback`, `learning`, `training` и `taskgen` сохранить собственные prompts, последовательность вызовов, форматы ответов и validators. Меняется transport на gRPC gateway. `model` и адрес провайдера в backend не возвращаются.

Важное ограничение текущего gateway: `urgent` может брать любой свободный совместимый instance; одного `complexity=high` недостаточно, чтобы гарантировать прежнюю модель обучения. В `.env.example` тега assessment/taskgen используют `gpt-oss-120b`, tutor наследует assessment при пустом значении. Реальную конфигурацию конкретного разработчика нужно сверить отдельно, не выводя токены в отчёт.

**Предлагаемое техническое решение для проверки владельцем:** добавить optional `routing_profile` в gateway, значения `default`, `learning-assessment`, `learning-tutor`, `learning-taskgen`. Backend выбирает профиль операции, gateway config сопоставляет его с разрешёнными instances; имя модели остаётся только в gateway. `urgent` не выходит за ограничения профиля. Пустой профиль полностью сохраняет нынешнее поведение Geek/course-graph. Неизвестный профиль — явная ошибка, без молчаливого fallback на другую модель.

Это ограниченное расширение транспортного контракта, необходимое при требовании сохранить выбор модели. Если владелец разрешит объединение модельных пулов и изменение выбора провайдера, задачу профилей можно исключить; по умолчанию такое разрешение не предполагается. Временно оставлять прямые вызовы «навсегда» либо менять политику Geek ради обучения не предлагается.

Проверить точное сохранение параметров:

- assessment: JSON, temperature 0, `max_tokens=4096`, reasoning medium;
- learning teacher: JSON, temperature 0.4, `max_tokens=4096`, reasoning medium, отдельные вызовы theory/practice/review/check;
- diagnosticfeedback: прежний формат/промпт, temperature 0.2, `max_tokens=4096`, reasoning low;
- taskgen: JSON, temperature 0, `max_tokens=1400`;
- training tutor: параметры и prompt version из конкретного перенесённого клиента, с отдельным тестом serialization.

Не использовать обрезанный JSON как успех. Сохранить проверки finish reason, обязательных полей, score/verdict, русскоязычности текста, Markdown и literal `\\n`. Не складывать разные системные промпты в общий system prompt Geek.

Бюджеты должны учитывать gateway queue + provider calls + STT + S3. Старый timeout 60/90 секунд и нынешний gateway `request_deadline=180s` не являются автоматически согласованными. Использовать отдельные deadlines операций; итоговый HTTP budget вычислять как максимум бюджетов обоих режимов плюс существующий запас. Не вводить один глобальный timeout, обрезающий многошаговую проверку.

Смена транспорта не гарантирует дословное совпадение недетерминированной генерации. Доказательство: одинаковые prompts/parameters/provider policy, одинаковые результаты на записанных ответах и отдельная проверка качества на тех же материалах/ответах.

### 5.2. STT/TTS и S3

- Один backend voice service; подтверждённая transcription передаётся оценщику без повторного распознавания. Владелец transcription проверяется сервером.
- Нынешний `internal/shared/s3.Client` расширить техническими операциями head/metadata вместо копирования второго AWS SDK transport из тега. Feature adapter переводит технические ошибки в `taskaudio` error types.
- Новый storage API: `HeadObject(ctx,bucket,key) (ObjectInfo,error)`, `PutObjectWithMetadata(ctx,bucket,key,contentType,data,metadata) error`, `OpenObjectWithMetadata(ctx,bucket,key) (io.ReadCloser,ObjectInfo,error)`, `BucketExists(ctx,bucket) (bool,error)`. Существующие методы сохранить для нынешних потребителей.
- `ObjectInfo` в shared содержит только size/content type/metadata; не содержит учебных сущностей. Metadata `audio-asset-id` интерпретирует taskaudio adapter.
- Для обучения использовать ключи `learn/task-audio/...`; нынешние ключи экзамена не менять без необходимости. Один bucket допустим, если namespace и проверки владельца исключают пересечение.
- Проверить восстановление объекта при потере S3, истёкший lease, stale worker token, повтор генерации, удаление исходного задания после переимпорта и разницу `404 object` / `404 bucket`.
- Не заменять backend TTS браузерным speech synthesis и не менять момент включения записи/перехода к результату.

## 6. План реализации

У каждой задачи: сначала конкретный regression/contract test, затем минимальное изменение, затем названные проверки. Коммиты — по завершённым задачам; не смешивать перенос с форматированием всего проекта. SHA и список файлов фиксировать в описании изменения.

### Задача 1. Зафиксировать эталон и замкнуть объём переноса

**Создать:** `documents/integration-parity.md`, `services/frontend/tests/fixtures/integration-parity/`, backend fixtures рядом с тестами владельцев. Если browser E2E отсутствует — добавить `playwright.config.ts`, `tests/e2e/` и script `test:e2e` в frontend как часть этой задачи.

- [ ] Составить таблицу: экран/действие → исходный SHA/файл → вызываемые implemented endpoints → новый владелец → parity test. Пройти обе админки, оба student home и переходы из итогов.
- [ ] Записать уже найденное различие UI reachability и registered API из §3.2; все перечисленные implemented API сохранить, проверить их прямыми HTTP-тестами. Новые UI-сценарии к ним не придумывать.
- [ ] Запустить эталонные сценарии на подготовленных тестовых данных и моделях-заглушках. Временно использовать исходные refs только для получения эталона, не добавляя их как runtime-зависимость новой системы.
- [ ] Сохранить screenshots основных экранов при одинаковом viewport, шрифтах и теме, а также ожидаемые DTO/state transitions. Не сравнивать screenshot с разными случайными задачами/датами.
- [ ] Перенести характерные фикстуры старого парсера: два листа, один лист, неизвестные колонки, несколько пар заданий/критериев, три блока задания и варианты «A.».
- [ ] Снять исходные результаты старых domain/application tests и нынешних exam tests. Отдельно отметить уже существующие failures.

**Приёмка:** у каждого доступного действия и зарегистрированного implemented API есть источник и конкретная проверка; dependency inventory сверен с перечнем §4.3–4.5. Зелёный baseline на заглушках не записывать как проверку настоящих моделей.

### Задача 2. Развести контракты двух режимов

**Изменить:** `api/openapi.yaml`, `services/backend/internal/app/app.go`, создать `internal/app/{learning,exam}.go`; `internal/app/contract_test.go` и route tests. Frontend consumers переключать вместе с соответствующим working slice.

- [ ] Зафиксировать карту всех нынешних exam endpoints под `/exam` и старых implemented endpoints под `/learn`, кроме общих auth/voice/health.
- [ ] Тест `TestLearningAndExamDiagnosticContractsDoNotOverlap`: разные request/response schema, неуспех обращения с ID другого режима, отсутствие выбора backend по Host/Referer.
- [ ] Сохранить отдельные пути файлов/аудио, включая URL, возвращаемые в JSON, а не только path регистрации handler.
- [ ] Учесть namespaced пути в middleware body limits: CSV/XLSX import, multipart audio, material upload/replace. Проверить граничные размеры и 413 вместо преждевременного малого общего лимита.
- [ ] Обновить Swag-аннотации и генерацию, проверить повторную генерацию без diff.

**Интерфейс результата:** явные route groups `/api/v1/learn`, `/api/v1/exam`; общий fault envelope остаётся прежним, domain-specific коды сохраняются.

### Задача 3. Единая схема без смешивания учебных данных

**Изменить:** новые `internal/shared/postgres/migrations/*_learning_*.sql`, `queries/{subject,learning_*}.sql`, `internal/shared/postgres/migrate_test.go`; генерируемый `sqlcgen`. Документировать таблицы в backend README.

- [ ] Тест `TestUnifiedSchemaFreshAndRepeat`: пустая БД → вся схема → повтор миграций без изменений.
- [ ] Тест `TestCurrentExamSchemaUpgradesWithoutChangingExamData`: нынешняя цепочка до 10 → новые миграции; auth, граф и Geek session не повреждены.
- [ ] Тест `TestCatalogsAndImportsAreModeScoped`: одинаковые имена курсов в режимах допустимы; импорт/список/готовность каждого режима изолированы.
- [ ] Тест `TestWrongModeCourseCannotBeMutated`: подставить learning subject ID в exam upload/build/import/delete/start и exam ID в learning import/variant/start; везде отказ до записи данных и вызова модели, в том числе у admin.
- [ ] Добавить `subjects.mode` и таблицы обучения из замкнутого списка задачи 1; адаптировать старый SQL, views, constraints и sequences, не вставлять старые migration files как есть.
- [ ] Старый map revision counter принадлежит learning; изменение карты не трогает `course_graphs` и наоборот. Проверить последовательные импорты двух разных learning subjects.
- [ ] Запустить SQLC и DB-тесты с настоящим `TEST_DATABASE_URL`; отсутствие переменной означает пропуск проверки, а не выполнение задачи.

**Не делать:** ETL двух БД, reconciliation email/password, импорт session cookies, объединение одноимённых курсов или автоматический reset volumes.

### Задача 4. Перенести карту обучения и генератор варианта

**Восстановить/адаптировать:** `entities/{competencymap,variant}`, `features/competency/{application,infrastructure,transport}`, `features/variantgen/*`, необходимые task profile helpers из тега; queries — только `learning_*`.

- [ ] Перенести parser tests до адаптации repository. Сохранить `taxonomyCode`, порядок исходных строк, совместимость листов и правила Markdown.
- [ ] Тесты `TestLearningStillReadsSecondSheet`, `TestExamStillReadsFirstSheet`, `TestLearningBloomNormalizationAndTiesMatchLegacy`.
- [ ] В `variantgen` сохранить `AlgorithmVersion=competency-map-v4`, chooser, ограничения готовности и selection algorithm. Тесты: <3 ОР, равные ранги/важность, несколько заданий ОР, пропущенные компетенции, ни одной пригодной компетенции.
- [ ] Repository использует общий `subjects` только с `mode=learn`, сохраняет task/profile snapshots и проверяет owner. Повтор start key с другим subject — прежний conflict.
- [ ] Тест `TestVariantSurvivesMapReimport`: live task удалён/изменён, старое задание и критерии читаются из snapshot.

**Результат:** из API обучения получается тот же вариант на той же карте и chooser; экзаменный импорт не изменён.

### Задача 5. Обеспечить совместимую маршрутизацию моделей через gateway

**Изменить:** `services/shared/proto/llmgateway/v1/gateway.proto`, gateway config/validation/request conversion/scheduler в `services/llm-gateway/internal/gateway`, `config.example.json`, HTTP/gRPC specifications и их contract tests; generated stubs — генератором.

- [ ] Сначала тесты: пустой profile полностью повторяет текущую маршрутизацию; explicit learning profile ограничивает instances даже при `urgent`; unknown profile отклоняется.
- [ ] Добавить optional profile следующим свободным protobuf field number, не меняя существующих номеров. Внутренний HTTP request/config contract обновить согласованно.
- [ ] Profile входит в fingerprint/idempotency и durable queue state; восстановление запроса после restart не теряет его. Старые записи без profile читаются как default.
- [ ] В metadata генерации задач сохранять фактический provider/model из gateway completion, если он нужен прежнему response/storage contract; не подменять его строкой profile и не возвращать model selection в backend config.
- [ ] Проверить отсутствие разрешённого instance, несовместимые parameters и освобождение слота после cancel. Не отправлять запрос в произвольную модель при проблеме конфигурации.
- [ ] Пример конфигурации сопоставить с исходными legacy model defaults; реальные model IDs и tokens настраиваются в gateway, не попадают в frontend.
- [ ] Прогнать shared generation/compatibility, gateway tests с настоящей Redpanda и проверки обеих Go-сторон. Если Python stubs изменены, проверить Python consumer даже при использовании им пустого profile.

**Результат:** backend может сохранить политику провайдера обучения без прямого model API. Это задача с отдельной проверкой; не прятать её в механическом копировании клиентов.

### Задача 6. Общие voice/S3 и независимые очереди аудио

**Изменить:** `internal/shared/s3/{client,client_test}.go`, создать learning taskaudio adapter поверх него; восстановить `entities/audioasset`, `features/taskaudio/*`, необходимые enqueue helpers и workers wiring.

- [ ] Реализовать технический storage API из §5.2, проверить metadata и missing bucket/object на локальном S3 fixture, затем имеющимся real RustFS smoke.
- [ ] Восстановить очередь, приоритеты и retry policy, CAS по claim token. Не создавать второй STT/TTS client и второй lifecycle менеджер.
- [ ] Тесты `TestLearningAudioNamespacesDoNotOverlapExam`, `TestStaleAudioWorkerCannotPublish`, `TestReimportKeepsVariantAudioRecoverable`.
- [ ] Проверить корректные API audio URLs после namespace-переноса, content type, закрытие stream и отсутствие утечки reference answer через аудио/metadata API.

**Результат:** оба режима используют общие transport adapters, но не отменяют задания/прогрев друг друга.

### Задача 7. Перенести старые assessment и диагностику без смены правил

**Восстановить/адаптировать:** `entities/{assessment,diagnostic}`, `features/{assessment,diagnostic,diagnosticfeedback}`; `internal/app/learning.go`; старые model adapters заменить gateway transport с сохранением prompts/validators.

- [ ] Перенести неизменённые по смыслу regression cases: `TestMainPerfectSkipsBasicsAndIdempotencySurvivesCompletion`, `TestRoleSpecificScoresAndBasicsContributeToDiagnostic`, `TestBasicsContributeToThreePointCompetencyTotal`.
- [ ] Добавить проверки трёхчастного feedback и запрета main score=2. Сравнить requests/results старого и нового adapters на одинаковых записанных completions.
- [ ] Сохранить reservation вне длительной DB-транзакции, lease, retry с сохранённой transcription и отсутствие повторного grade уже принятого ответа.
- [ ] Тесты foreign transcription/variant/session → прежний отказ; параллельные ответы → conflict; cancel/timeout → возможность корректного повтора.
- [ ] Фидбэк выводит confirmed/partial/unverified из сохранённых ответов по прежним правилам. Не подменять этот анализ `exam.Result.Gaps` и A–D.
- [ ] Не тянуть неиспользуемые hardcoded curriculum fallback из legacy API в новый путь отчёта. Любое исключение из переноса подтвердить reachability таблицей задачи 1; используемую учебную логику не переписывать молча.

**Результат:** старые шкалы/переходы/итоги работают в одном backend рядом с неизменённым Geek.

### Задача 8. План и тренировка обучения

**Восстановить/адаптировать:** `entities/learning`, `features/learning/{application,infrastructure,transport}`, learning model prompts и paragraph helpers, `features/{training,taskbank,taskgen}` и их queries/tests.

- [ ] Перенести `TestPlanUsesCheckedGapsAndOnlyFullyUnstudiedConstituent`, правила `OutcomeFindings`, snapshot fallback после переимпорта и тесты распределения часов.
- [ ] `Create` строит один план на owner+diagnostic; активная диагностика не подходит; повтор возвращает сохранённый план с прежними параметрами.
- [ ] `Open` сохраняет теорию после её успешной генерации, затем готовит/редактирует практику. Ошибка следующего шага не стирает теорию и не дублирует сообщения.
- [ ] Проверка ответа получает именно показанную практику, сохранённую теорию и прежние разборы. Технический retry не расходует дополнительную учебную попытку.
- [ ] Проверить правильный ответ, две ошибки и третью ошибку, корректный следующий случай, последний шаг, pending answer после сбоя и два параллельных `/open`/`answer`.
- [ ] Сохранить прежнюю семантику current/passed/review/optional items и UI-progress. Не «исправлять» её одновременно с объединением.
- [ ] Самостоятельную бесконечную тренировку/chat сохранить на уровне implemented API: snapshots, preview/start revision, history, retry/cancel turns и ограничения контекста. Использовать старые integration tests как отдельный набор; не перепутать её с практикой плана и не добавить в UI новый вход.
- [ ] Taskbank/taskgen/FTS сохранить на уровне implemented API и старых contract/integration tests: фильтры subject/outcome, idempotent generation, snapshot профиля, источники-примеры, изоляция от exam materials и parser. Запланированные, но не подключённые HTTP операции не реализовывать дополнительно.

**Результат:** полная достижимая цепочка обучения восстанавливается после перезапуска API и обновления страницы; история экзамена не изменяется.

### Задача 9. Общие конфигурация и lifecycle

**Изменить:** `internal/app/{app,config,config_test,server_test}.go`, `cmd/api/main.go`, root env templates, backend Compose и README.

- [ ] Настроить один DB pool, один auth, общие voice/storage transports, gateway connection и запуск всех нужных workers.
- [ ] Новые env — только реальные timeout/worker/context параметры; не возвращать backend model/base URL settings. Добавить их одновременно во все места по AGENTS.
- [ ] `HTTPWriteTimeout()` покрывает максимум действующих exam budget и learning STT+трёх последовательных tutor calls с запасом. Проверить и client-side timeout, и proxy, и gateway deadline.
- [ ] Shutdown не оставляет активный worker без cancel и не публикует результат с устаревшим lease. Restart возобновляет постоянные очереди.
- [ ] `/health` по-прежнему не выдавать за проверку работоспособности моделей. Отдельный functional smoke обязан вызвать LLM/STT/TTS.

### Задача 10. Перенести экраны обучения в одну frontend-композицию

**Создать:** slices из §4.4, `pages/learning-trainer`, `pages/learning-competency-map`; перенести их tests из тега. **Изменить:** `bootstrap/{dependencies,providers}.tsx/ts`, mode-specific API adapters и i18n.

- [ ] Переносить UI с сохранением разметки/SCSS и поведения. Новые адреса API и импорты не должны стать поводом переделывать экран.
- [ ] Тесты старой диагностики/плана запускаются на новых owning slices; существующие Geek route tests продолжают проходить.
- [ ] Query keys/storage включают режим и user; одинаковые synthetic IDs в двух режимах возвращают разные данные в одном QueryClient.
- [ ] Проверить sessionStorage intent, восстановление после reload, переключение режима во время незавершённого урока и возврат к нему.
- [ ] Сохранить preview подтверждённого голоса, audio player lifecycle, запрет двойной отправки, теорию до практики и polling сообщений без скачка прокрутки.
- [ ] Переиспользовать нынешний session epoch/отмену запросов при смене пользователя; не восстановить старую более слабую auth реализацию.
- [ ] Сравнить screenshots по задаче 1; допустимые различия — согласованные URL и только технически необходимые навигационные привязки.

### Задача 11. Стартовая, маршруты и единый вход

**Создать:** `pages/mode-selection`, routes `learn.tsx`, `exam.tsx`, административные namespaced routes. **Изменить:** `routes/index.tsx`, `bootstrap/root-shell.tsx`, `features/auth`, auth/portal/navigation tests.

- [ ] Перенести вид/поведение `deploy/dual/start/index.html` в React. Форма вызывает один auth API; после входа тот же выбор двух режимов.
- [ ] `SessionGate` различает общую стартовую, student modes и admin portal. Не оставлять автоматический `Navigate('/')` для любого авторизованного `/login`, ломающий intended state стартовой.
- [ ] «К выбору режима» ведёт на `/`, logout очищает общую student session/cache и приводит к форме. Ошибка сети не подменяется отсутствием сессии.
- [ ] Сохранить passwordless teacher entry и `X-AI-Tutor-Portal: admin`; разные cookie names/student/admin refresh locks остаются. Проверить учителя и студента одновременно в разных вкладках.
- [ ] Добавить тест student → admin → student в одном QueryClient: user/epoch/refresh state разнесены по portal, незавершённый запрос сохраняет исходный portal; shared account menu не принимает teacher за student и наоборот.
- [ ] Проверить Back/Forward, прямой URL, reload вложенной админки, неизвестный маршрут и URL с завершающим `/`.
- [ ] Перегенерировать route tree Vite-плагином. Старый `/login` можно оставить техническим redirect на `/`; отдельную вторую форму в режимах не сохранять.

### Задача 12. Один Compose stack и два launcher script

**Изменить:** `docker-compose.yaml`, создать `docker-compose.dev.yaml`, `dev.sh`, `prod.sh`, необходимые service Compose, Caddy, frontend Dockerfile, env examples.

- [ ] Корневой Compose сделать общей production-основой с финальными образами/стадиями; dev override добавляет Air/Vite, исходники, caches и polling. Существующие service definitions переиспользовать, не дублировать вручную.
- [ ] `prod.sh` валидирует единый stack и запускает его одним Compose project; `dev.sh` использует тот же stack + dev override. Ни один не делает Git fetch/worktree/switch.
- [ ] Перед сменой project/volume names описать явные defaults. Не запускать одновременно нынешние standalone проекты и новый root project против одного PG volume. Никакого автоматического `down -v`.
- [ ] Запуск печатает стартовую и обе новые админки. Параметр отключения browser opener сохранить; запуск не требует отдельного dual host/DNS.
- [ ] Один Caddy обслуживает `/api/v1`, docs/gateway и один frontend. Два режима не требуют разных upstream frontend/backend. TLS и SPA fallback проверяются в dev/prod.
- [ ] Gateway становится обязательной зависимостью объединённого приложения; swagger/observability могут остаться optional profiles. Не допустить состояния, когда UI обучения доступен, а нужный gateway не запущен из-за профиля.
- [ ] Сохранить выбор CPU/GPU course-graph, external S3 configuration, Docker build contexts/shared mounts и persistent model cache. Репозиторий `tts-stt` остаётся внешней зависимостью с прежним API; не объединять его с этим проектом.

**Проверки:** shell syntax, обе env templates, dev/prod Compose config, production image builds, поднятый stack с restart и реальным HTTPS microphone smoke. Windows shell compatibility отдельно не обещается: новый PowerShell launcher пользователь исключил.

### Задача 13. Удалить dual после проверки эквивалентности

**Удалить:** `deploy/linux-dual.sh`, `deploy/windows-dual.ps1`, `deploy/linux-dual-reset.sh`, `deploy/windows-dual-reset.ps1`, `deploy/dual/{Caddyfile,docker-compose.yaml,README.md,start/index.html}` и ставшую пустой структуру.

**Изменить:** root/service Compose, frontend Dockerfile, `.env.example`, `.env.prod.example`, frontend auth exports/tests, README, service READMEs, `documents/{README,auth,backend-architecture}.md`, `checklist.html`, CI references.

- [ ] Удалить `VITE_MODE_SELECT_URL`, `modeSelectUrl/openModeSelect`, cross-origin auth bridge, release-network/container names и генерацию `compose.dual.yaml` из поддерживаемого кода.
- [ ] Вместо старых dual redirect tests оставить тесты новой внутренней навигации; не удалять их assertions об обязательном едином входе/выходе.
- [ ] Записать в README новое решение: одна реализация, сохранённые два режима и отдельные учебные данные. Исторические документы не переписывать так, будто dual никогда не было.
- [ ] Runtime не читает старый тег; CI не собирает второй релиз; никакая команда не создаёт `ai-tutor-learn/ai-tutor-exam`.
- [ ] Только после этого убрать obsolete release logic. Локальные пользовательские worktree/volumes/сертификаты не удалять; при необходимости дать отдельную адресную команду очистки, без включения в dev/prod.

### Задача 14. CI, сквозное доказательство и итоговый аудит

**Изменить:** `.github/workflows/ci.yaml`, reusable workflows по зависимостям, frontend E2E script/config; добавить stack smoke runner в `tests/integration/`, не в набор deployment scripts.

- [ ] Изменение mode routing/Compose/auth запускает backend+frontend+stack smoke; изменение shared/gateway — всех потребителей, включая Python. Добавить Python course-graph job, если его всё ещё нет в HEAD.
- [ ] В CI запускать backend DB tests с `TEST_DATABASE_URL`; реальный S3 smoke — с RustFS. Проверять финальные backend/frontend/gateway/course-graph CPU images, не только локальную компиляцию.
- [ ] E2E: один browser context, один login, оба режима, две админки, reload/restart, отказ модели, подтверждённый голос и logout. Проверка только контейнерного health недостаточна.
- [ ] Выполнить полную матрицу §7 и сравнить baseline screenshots/state/requests. Не обновлять golden expectations, чтобы скрыть несогласованное изменение.
- [ ] Проверить `git diff` целиком: нет второй bootstrap/server/schema history, runtime legacy ref, выключенных тестов или перенесённых секретов.
- [ ] В итоговом отчёте перечислить реальные проверки, skips и все внешние отличия. Не принимать работу, если «совпадение UI» подтверждено только чтением JSX.

## 7. Матрица обязательной приёмки

| ID | Условие | Ожидаемый результат | Владелец проверки |
| --- | --- | --- | --- |
| P01 | Register → learn → выбор режима → exam | Один пользователь и один login, прежние экраны обоих режимов. | 10–11, E2E |
| P02 | Одинаковые synthetic IDs курсов/сессий в тестовых API | Нет подмены данных в query cache/storage между режимами. | 3, 10 |
| P03 | Learning main=3 | Базовые пропущены, прежний score/max и переход. | 7 |
| P04 | Learning main=1/0/skip | Прежние базовые и порядок; не появляется Geek followup/rephrase. | 7 |
| P05 | Все нули, mixed constituents, пропуск, неполные данные | «Изучить полностью» только при прежних достаточных основаниях. | 7–8 |
| P06 | XLSX с разными первым и вторым листом | Режимы читают свои прежние данные, не один объединённый лист. | 4, 10 |
| P07 | Bloom анализировать/оценивать/создавать и равная важность | Learning и exam сохраняют каждый свои tie-breakers. | 4, exam regression |
| P08 | Переимпорт карты обучения при активной диагностике/плане | Работают старые snapshots; exam graph/session не меняются. | 4, 7–8 |
| P09 | Пересборка exam graph во время сессии | Существующая отмена Geek; learning session/plan не отменяются. | exam regression, 3 |
| P10 | Повтор start/answer key; другой payload с тем же key | Replay без дублей; прежний conflict для другого payload. Нет cross-mode replay. | 4, 7–8 |
| P11 | LLM timeout/cancel/invalid JSON/truncated completion | Нет ложного успеха/сдвига шага; retry по сохранённым данным. | 5, 7–9 |
| P12 | Теория готова, practice generation упала | Теория видна и сохранена, повтор не генерирует её заново. | 8, 10 |
| P13 | Два параллельных ответа/open и API restart | Нет двойной попытки/сообщения, корректное восстановление lease/pending. | 6–9 |
| P14 | Preview голоса → подтверждение → submit | Оценивается тот же owned text; повторный STT не меняет ответ. | 7–8, 10 |
| P15 | Потерян S3 object/bucket, stale worker, переимпорт | Правильное восстановление/ошибка без утечки данных и чужой перезаписи. | 6 |
| P16 | Student и teacher в соседних вкладках | Независимые portal sessions при общем backend/auth. | 11, E2E |
| P17 | Logout в learn, открытый exam, поздний ответ запроса | Student session отозвана; чужой/поздний ответ не восстанавливает UI/cache. | 10–11 |
| P18 | Первый graph build/непригодная карта/пустой каталог | Прежние empty/error/not-ready состояния каждого режима. | 3–4, 10 |
| P19 | Wrong owner/mode/course/material revision | Прежний отказ; нельзя получить эталон/чужое аудио/raw по угаданному ID. | 2–8 |
| P20 | Geek result, шкала/источники/closing audio | Без изменений существующего поведения. | exam suite, 10 |
| P21 | `/learn`, `/exam`, admin deep link, reload, Back/Forward | SPA route работает через Caddy и production nginx. | 11–12, E2E |
| P22 | Cold start и restart через каждый launcher | Один stack; данные сохраняются; workers и микрофон работают. | 12, 14 |

Пять наиболее опасных областей для ручной проверки: пересечение cache/session между режимами; случайная замена legacy шкалы на Geek; неправильный лист/Bloom; потеря snapshot после переимпорта; смена модели или обрезание многошаговой практики таймаутом. Для каждой есть отдельные automated cases выше; ручной прогон дополняет их.

## 8. Команды проверки при реализации

Из backend:

```bash
go test -race ./... -count=1
go vet ./...
go build -o bin/api ./cmd/api
bash scripts/generate-sql.sh
bash scripts/generate-openapi.sh
```

DB/S3 tests выполнять с одноразовыми `TEST_DATABASE_URL` и `TEST_S3_*`. Генераторы после повторного запуска не оставляют изменений относительно уже сгенерированного состояния; проверять также новые untracked generated files.

Из frontend:

```bash
npm run format:check
npm run lint
npm run typecheck
npm test -- --run
npm run build
npm run test:e2e
```

Последняя команда появится в задаче 1. Для browser audio tests использовать управляемый microphone fixture; реальный микрофон/сертификат проверить отдельно вручную.

Из gateway:

```bash
GOWORK=off go test -race ./... -count=1
GOWORK=off go vet ./...
GOWORK=off go build ./cmd/llm-gateway
bash scripts/generate-openapi.sh
```

Из корня — `services/shared/tools/generate.sh --check` и `services/shared/tools/check-compat.sh <зафиксированный-base-sha>`. Из course-graph — `uv run --locked --extra cpu --extra ocr python -m pytest tests`.

Для целевой Compose-структуры:

```bash
sh -n dev.sh
sh -n prod.sh
docker compose --env-file .env.example config --quiet
docker compose --env-file .env.prod.example config --quiet
docker compose --env-file .env.example -f docker-compose.yaml -f docker-compose.dev.yaml config --quiet
```

Отдельно сохранить проверки каждого затронутого service Compose с обоими env templates. Проверить выбранные CPU/GPU и local/external S3 варианты; реальный GPU прогон делать только при наличии подходящего оборудования, не выдавая `config --quiet` за него.

## 9. Последовательность, риски и граница завершения

Порядок: **1 → 2 → 3 → 4 → 5 → 6 → 7 → 8 → 9 → 10 → 11 → 12 → 14 (первый полный прогон) → 13 → 14 (финальный прогон после удаления)**. Вся работа выполняется последовательно одним исполнителем. Сначала доказать работоспособность замены, затем удалить старую обвязку; пользователю не предлагается новое постоянное сосуществование двух версий.

Это заметный перенос учебной подсистемы, а не правка нескольких proxy rules: только diff старого и нынешнего backend/frontend/API охватывает 493 файла. Число файлов не является оценкой трудоёмкости, но объясняет необходимость небольших проверяемых этапов. Время в часах без baseline-прогона и замкнутого dependency inventory было бы выдумкой.

| Риск | Мера |
| --- | --- |
| Возврат десятков тысяч строк целиком вместе с историческим мусором | Перенос по owners и реальному call graph, сохранение конкретных regression cases, запрет копировать app/auth/initial migration. |
| Желание «улучшить» старый UX/алгоритм по дороге | Любое такое предложение отдельно; parity tests обновлять только при отдельном продуктовом решении. |
| Общий сервер стал точкой конкуренции workers | Ограничить worker concurrency и проверить одновременные learn/exam requests; не удваивать бесконтрольно TTS/LLM нагрузку. |
| Очередь gateway замедлила обучение | Отдельные operation budgets, compatibility routing, проверка совместной нагрузки с OCR/graph build; без обещания выдуманного latency SLA. |
| Изменение Compose names подключило неверный PG volume | Явная схема project/volume names, fresh/repeat checks, отсутствие автоматического удаления и одновременного открытия одного volume. |
| Неполный/недоступный реальный workbook или модели | Пометить конкретный непроверенный сценарий; не заявлять визуальную/модельную эквивалентность по synthetic fixtures. |

### Откат при разработке

Сохранять отдельные коммиты для schema, module port, UI switch и удаления dual. До удаления старой обвязки сравнение всегда можно повторить по зафиксированным Git refs; это не часть финального runtime. Если parity не доказана, не переходить к удалению и не объявлять интеграцию завершённой.

Откат исходников не означает автоматический откат PostgreSQL: старый бинарник не должен запускаться против произвольной новой схемы/volume. Для повторного эталона использовать отдельную одноразовую БД. Тестовые volumes нового стека можно пересоздать явной адресной командой после проверки их имён; `dev.sh`/`prod.sh` этого не делают. Удаление моделей course-graph и файлов `.env` для отката не нужно.

### Что сознательно не входит

- Перенос реальных пользователей и двух существующих production БД, массовый merge курсов и история старых cookies.
- Общий граф/каталог для двух педагогических моделей, адаптация старой диагностики к PDF graph или замена её на Geek.
- Новый дизайн, новый учебный алгоритм, новая шкала, новая политика teacher/password или исправление всех старых продуктовых спорных мест.
- Kubernetes, registry/release pipeline, автоматический CD, поддержка нового Windows launcher, переделка отдельного `tts-stt` репозитория.
- Удаление локальных volumes/worktree или секретов пользователя как часть обычного запуска.

### Когда задача действительно закончена

- [ ] Из checkout запускаются один frontend, один backend и одна PostgreSQL без зависимости от старых refs/worktree.
- [ ] На `/` прежний выбор режимов и один настоящий auth; доступны `/learn` и `/exam`.
- [ ] Преподаватель отдельно готовит данные режимов, каталоги не смешаны.
- [ ] Оба учебных процесса, экранные состояния, шкалы, snapshots, голос, восстановление и выход подтверждены матрицей приёмки.
- [ ] Прямые legacy model calls заменены совместимым gateway transport; backend не содержит model selection/адресов провайдеров.
- [ ] Dual scripts/proxy/start HTML/env hooks удалены, остаются `dev.sh` и `prod.sh` с одним stack.
- [ ] Документация описывает фактическое состояние; CI проверяет обе подсистемы и их совместную работу.
- [ ] Владелец проверил сравнение с эталоном; все несогласованные внешние изменения устранены, все пропуски проверок названы явно.

## 10. Самопроверка этого плана

- Последнее требование «внешне сохранить» применено ко всем разделам. Промежуточное предложение общего графа/каталога исключено.
- Не смешаны две диагностики и две разные сущности тренировки; полнота reachability вынесена в первый проверяемый результат.
- Учтены одинаковые пути/имена/номера миграций, листы книги, Bloom, role-specific scores, snapshot retention, generated files и body limits.
- Зафиксированы скрытые несовпадения S3 API, gateway urgency/provider policy, durable idempotency и таймаутов многошаговых операций.
- Админский вход не объединяется со student session; разделение учебных данных не требует двух физических БД.
- Удаление dual стоит после parity-проверки, а окончательное доказательство — после самого удаления.
- План не выдаёт чтение кода за browser/model/DB проверку. Docker daemon был недоступен; соответствующие работы ещё предстоят.

## Приложение A. Полная карта зарегистрированных HTTP-операций

Получена из `internal/app/app.go` обоих зафиксированных refs. Это целевое соответствие путей, а не утверждение, что новые namespace уже реализованы. Catch-all 404 не является отдельной операцией.

### Обучение: 48 операций

| Метод | Путь в исходном app mux | Целевой внешний API |
| --- | --- | --- |
| POST | `/admin/subjects/{subject_id}/competency-map/import` | `/api/v1/learn/admin/subjects/{subject_id}/competency-map/import` |
| POST | `/admin/subjects` | `/api/v1/learn/admin/subjects` |
| POST | `/assessments/evaluate` | `/api/v1/learn/assessments/evaluate` |
| POST | `/assessments/overall-feedback` | `/api/v1/learn/assessments/overall-feedback` |
| POST | `/diagnostic-sessions` | `/api/v1/learn/diagnostic-sessions` |
| GET | `/diagnostic-sessions/{id}` | `/api/v1/learn/diagnostic-sessions/{id}` |
| GET | `/subjects/{subject_id}/learning-state` | `/api/v1/learn/subjects/{subject_id}/learning-state` |
| GET | `/diagnostic-sessions/{id}/current/audio` | `/api/v1/learn/diagnostic-sessions/{id}/current/audio` |
| POST | `/diagnostic-sessions/{id}/current/audio/regenerate` | `/api/v1/learn/diagnostic-sessions/{id}/current/audio/regenerate` |
| GET | `/task-audio/{id}/file` | `/api/v1/learn/task-audio/{id}/file` |
| POST | `/diagnostic-sessions/{id}/answers/transcription` | `/api/v1/learn/diagnostic-sessions/{id}/answers/transcription` |
| POST | `/diagnostic-sessions/{id}/answers` | `/api/v1/learn/diagnostic-sessions/{id}/answers` |
| POST | `/diagnostic-sessions/{id}/skip` | `/api/v1/learn/diagnostic-sessions/{id}/skip` |
| GET | `/diagnostic-sessions/{id}/result` | `/api/v1/learn/diagnostic-sessions/{id}/result` |
| GET | `/diagnostic-sessions/{id}/feedback` | `/api/v1/learn/diagnostic-sessions/{id}/feedback` |
| GET | `/diagnostic-sessions/{id}/training/preview` | `/api/v1/learn/diagnostic-sessions/{id}/training/preview` |
| POST | `/diagnostic-sessions/{id}/training` | `/api/v1/learn/diagnostic-sessions/{id}/training` |
| GET | `/diagnostic-sessions/{id}/training` | `/api/v1/learn/diagnostic-sessions/{id}/training` |
| GET | `/training-sessions/{id}` | `/api/v1/learn/training-sessions/{id}` |
| POST | `/training-sessions/{id}/answers` | `/api/v1/learn/training-sessions/{id}/answers` |
| POST | `/training-sessions/{id}/skip` | `/api/v1/learn/training-sessions/{id}/skip` |
| POST | `/training-sessions/{id}/answers/reset` | `/api/v1/learn/training-sessions/{id}/answers/reset` |
| POST | `/training-sessions/{id}/exercises/{exercise_id}/next` | `/api/v1/learn/training-sessions/{id}/exercises/{exercise_id}/next` |
| GET | `/training-sessions/{id}/history` | `/api/v1/learn/training-sessions/{id}/history` |
| GET | `/training-sessions/{id}/current/audio` | `/api/v1/learn/training-sessions/{id}/current/audio` |
| POST | `/training-sessions/{id}/current/audio/regenerate` | `/api/v1/learn/training-sessions/{id}/current/audio/regenerate` |
| GET | `/training-sessions/{id}/exercises/{exercise_id}/chat` | `/api/v1/learn/training-sessions/{id}/exercises/{exercise_id}/chat` |
| POST | `/training-sessions/{id}/exercises/{exercise_id}/chat/turns` | `/api/v1/learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns` |
| POST | `/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/retry` | `/api/v1/learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/retry` |
| POST | `/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/cancel` | `/api/v1/learn/training-sessions/{id}/exercises/{exercise_id}/chat/turns/{turn_id}/cancel` |
| GET | `/tasks` | `/api/v1/learn/tasks` |
| GET | `/subjects/{subject_id}/competency-map` | `/api/v1/learn/subjects/{subject_id}/competency-map` |
| GET | `/subjects` | `/api/v1/learn/subjects` |
| GET | `/tasks/{id}` | `/api/v1/learn/tasks/{id}` |
| POST | `/tasks/generate` | `/api/v1/learn/tasks/generate` |
| POST | `/admin/materials` | `/api/v1/learn/admin/materials` |
| POST | `/variants` | `/api/v1/learn/variants` |
| GET | `/variants` | `/api/v1/learn/variants` |
| GET | `/variants/{id}` | `/api/v1/learn/variants/{id}` |
| GET | `/variants/{id}/tasks/{task_id}` | `/api/v1/learn/variants/{id}/tasks/{task_id}` |
| POST | `/diagnostic-sessions/{id}/learning-plan` | `/api/v1/learn/diagnostic-sessions/{id}/learning-plan` |
| GET | `/diagnostic-sessions/{id}/learning-plan` | `/api/v1/learn/diagnostic-sessions/{id}/learning-plan` |
| GET | `/learning-plans/{id}` | `/api/v1/learn/learning-plans/{id}` |
| GET | `/learning-plans/{id}/items/{index}/messages` | `/api/v1/learn/learning-plans/{id}/items/{index}/messages` |
| POST | `/learning-plans/{id}/open` | `/api/v1/learn/learning-plans/{id}/open` |
| POST | `/learning-plans/{id}/answers/transcription` | `/api/v1/learn/learning-plans/{id}/answers/transcription` |
| POST | `/learning-plans/{id}/answers` | `/api/v1/learn/learning-plans/{id}/answers` |
| POST | `/learning-plans/{id}/answers/retry` | `/api/v1/learn/learning-plans/{id}/answers/retry` |

### Экзамен: 19 операций

| Метод | Путь в исходном app mux | Целевой внешний API |
| --- | --- | --- |
| GET | `/admin/material-kinds` | `/api/v1/exam/admin/material-kinds` |
| PATCH | `/admin/subjects/{subject_id}/materials/{material_id}` | `/api/v1/exam/admin/subjects/{subject_id}/materials/{material_id}` |
| PUT | `/admin/subjects/{subject_id}/materials/{material_id}/raw` | `/api/v1/exam/admin/subjects/{subject_id}/materials/{material_id}/raw` |
| POST | `/admin/subjects/{subject_id}/materials` | `/api/v1/exam/admin/subjects/{subject_id}/materials` |
| GET | `/admin/subjects/{subject_id}/materials` | `/api/v1/exam/admin/subjects/{subject_id}/materials` |
| GET | `/admin/subjects/{subject_id}/graph` | `/api/v1/exam/admin/subjects/{subject_id}/graph` |
| POST | `/admin/subjects/{subject_id}/graph/build` | `/api/v1/exam/admin/subjects/{subject_id}/graph/build` |
| POST | `/admin/subjects/{subject_id}/graph/competency-map` | `/api/v1/exam/admin/subjects/{subject_id}/graph/competency-map` |
| DELETE | `/admin/subjects/{subject_id}/materials/{material_id}` | `/api/v1/exam/admin/subjects/{subject_id}/materials/{material_id}` |
| GET | `/admin/subjects/{subject_id}/materials/{material_id}/raw` | `/api/v1/exam/admin/subjects/{subject_id}/materials/{material_id}/raw` |
| POST | `/admin/subjects` | `/api/v1/exam/admin/subjects` |
| GET | `/subjects` | `/api/v1/exam/subjects` |
| POST | `/diagnostic-sessions` | `/api/v1/exam/diagnostic-sessions` |
| GET | `/diagnostic-sessions` | `/api/v1/exam/diagnostic-sessions` |
| GET | `/diagnostic-sessions/{id}` | `/api/v1/exam/diagnostic-sessions/{id}` |
| POST | `/diagnostic-sessions/{id}/answers` | `/api/v1/exam/diagnostic-sessions/{id}/answers` |
| GET | `/diagnostic-sessions/{id}/result` | `/api/v1/exam/diagnostic-sessions/{id}/result` |
| GET | `/diagnostic-sessions/{id}/materials/{material_id}/raw` | `/api/v1/exam/diagnostic-sessions/{id}/materials/{material_id}/raw` |
| GET | `/diagnostic-sessions/{id}/turns/{turn_id}/audio` | `/api/v1/exam/diagnostic-sessions/{id}/turns/{turn_id}/audio` |

### Общие: 9 операций

| Метод | Целевой путь |
| --- | --- |
| POST | `/api/v1/auth/register` |
| POST | `/api/v1/auth/login` |
| POST | `/api/v1/auth/teacher` |
| POST | `/api/v1/auth/refresh` |
| POST | `/api/v1/auth/logout` |
| GET | `/api/v1/auth/me` |
| POST | `/api/v1/voice/transcriptions` |
| POST | `/api/v1/voice/syntheses` |
| GET | `/api/v1/health` |

Контроль полноты: 48 learning + 19 exam + 9 common = **76 операций app API**. В задаче 2 закрепить этот manifest contract-тестом с явным списком; отсутствие хотя бы одного перенесённого implemented endpoint должно давать failure. Служебные Caddy gateway/docs routes проверяются дополнительно.
