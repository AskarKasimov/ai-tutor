# Проект архитектуры фронтенда

Дата: 2026-10-08. Статус: реализовано в services/frontend; архитектура и проверки сверены с кодом.

## Цель и границы

Запрос: рефакторинг фронтенда по принципам чистой архитектуры.
Предлагаемый критерий успеха: сценарии приложения можно проверять без React,
HTTP и браузерного хранилища; UI получает данные через hooks; выбор реализаций
происходит при сборке приложения. Направление зависимостей проверяется автоматически.

Предположение для согласования: сохраняются текущие UI, маршруты, локализация,
HTTP-контракты и различия real/mock. Оценивание и выбор заданий остаются на backend.
Рефакторинг не добавляет пользовательское поведение и не меняет правила повторов.

Уточнение пользователя: приложение развивается как полноценный продукт.
Названия и комментарии текущих рабочих модулей не представляют приложение как
временную работу. Исторические имена в таблице ниже показывают переименование.
`mock` и `demo` сохраняют смысл конкретного режима и не заменяются словом
`production`: демонстрационный режим остаётся частью приложения.

## Продуктовые названия

| Сейчас | После рефакторинга | Назначение |
| --- | --- | --- |
| `useTrainerPrototype`, `use-trainer-prototype.ts` | `useTrainerVoice`, `use-trainer-voice.ts` | Голосовой сценарий локального тренажёра |
| `prototype-audio.ts` | `browser-audio.ts` | Браузерные адаптеры записи, воспроизведения и object URLs для обоих режимов |
| `styles.prototype`, `.prototype` | `styles.trainerLayout`, `.trainerLayout` | Разметка тренажёра |
| Комментарий `disposable voice trainer prototype` | Описание конкретных браузерных адаптеров | Ответственность модуля |

Переименовываются также соответствующие тестовые файлы, импорты и ссылки
в проверках архитектуры. Совместимые экспорты со старыми именами в итоговом
коде не сохраняются. Стандартное JavaScript-свойство `.prototype`
(`MediaRecorder.prototype`, `HTMLDialogElement.prototype`) остаётся без изменений.

## Начальное состояние до переноса

- `src/data/` содержит одновременно HTTP-транспорт, схемы ответов, Query-hooks,
  управление авторизацией, восстановление диагностики и запись голоса.
- `use-diagnostic-session.ts` связывает создание варианта, идемпотентные ключи,
  сохранение идентификаторов, отмену запросов и QueryClient.
- `use-trainer-session.ts` самостоятельно создаёт mock-источник, хотя контракт
  `TrainerSessionSource` уже существует.
- `routes/-session-summary.tsx` (744 строки) содержит разбор текстовой обратной
  связи, построение payload, прямой вызов `fetchOverallFeedback` и разметку.
- `routes/-diagnostic-trainer.tsx` (632 строки) содержит несколько экранных
  состояний и зависит от инфраструктурного `DiagnosticApiError`.
- Тест `network-boundaries.test.ts` защищает некоторые вызовы, но его список
  имён не включает `fetchOverallFeedback`, поэтому нарушение в итогах не обнаруживается.
- `shared/domain.ts` смешивает модели demo, авторизации и серверной диагностики;
  часть моделей непосредственно повторяет wire-format в snake_case.

## Соответствие Feature-Sliced Design

Миграция следует официальным правилам FSD: верхние слои могут импортировать
нижние, slices одного слоя изолированы, а внешние импорты slice идут через его
public API `index.ts`. Слои создаются только там, где дают ответственность;
widgets не добавляются для единственной оболочки экрана. См. [обзор FSD](https://feature-sliced.design/docs/get-started/overview),
[слои и направления импортов](https://feature-sliced.design/docs/reference/layers),
[slices и segments](https://feature-sliced.design/docs/reference/slices-segments)
и [public API](https://feature-sliced.design/docs/reference/public-api).

| FSD слой / slice | Текущее владение и результат |
| --- | --- |
| `bootstrap` | Согласованное имя app-layer alias: entrypoint, providers, DI composition и глобальные стили. `src/app` и горизонтальный `src/application` не используются. |
| `pages/trainer` | Текущий `/` screen переключения demo/real и его уникальная UI композиция; диагностическая UI остаётся внутренней частью этого page slice. |
| `pages/login` | Экран входа/регистрации и route-level UI. |
| `pages/competency-map` | Административный экран карты компетенций и его локальные стили. |
| `features/auth` | Query/mutation hooks, auth form, login/logout interactions; свой порт и provider context. |
| `features/diagnostic-session` | Запуск/восстановление/отправка диагностики, session bootstrap scenario и diagnostic voice interaction; внедряемые API/storage/identity ports. |
| `features/trainer-session` | Demo trainer session и построение feedback payload из trainer-session; свой port/context. |
| `features/import-competency-map` | Чтение и атомарный import карты, включая Query hooks и отмену запросов. |
| `entities/user` | Пользователь, auth/session модели и session lifecycle. Account menu принадлежит feature auth, потому что вызывает auth hooks. |
| `entities/diagnostic-session` | Типы прогресса/результатов диагностики, ошибки и submission модели. |
| `entities/trainer-session` | Задания/ответы/сессия тренажёра и demo session source contract. |
| `entities/assessment` | Assessment модели и pure feedback parser. Feedback payload builder принадлежит feature trainer-session; голосовые действия — feature voice-answer. |
| `entities/competency-map` | Доменные модели и типизированная ошибка карты компетенций. |
| `shared/api` | Общий HTTP transport, auth-refresh retry/lock и нейтральные HTTP helpers; предметные endpoints и schemas остаются в owners. |
| `shared/lib` | Повторно используемые браузерные audio/waveform adapters и узкие платформенные helpers. |
| `shared/i18n`, `shared/config`, `shared/ui` | Локализация, общая конфигурация и действительно общая UI основа. Предметные DI ports не переносятся в shared. |
| `routes` | Вне FSD layers остаётся техническим TanStack route registry с прежними URL и generated routeTree. Routes только связывают путь с page public API. |

Сценарии и порты остаются в `model`/`lib` своих owning feature slices и не
получают зависимости React/HTTP, если были чистыми ранее. HTTP DTO/schema и
конкретные адаптеры находятся в `api` owner slices; bootstrap выбирает demo/real
реализации и передаёт контексты владельцам. Контексты и типы не агрегируются в
`shared`. Entities не импортируют друг друга; взаимные типы оформляются только
узкими документированными `@x/<consumer>` public APIs. Внешний slice импортирует
только из `index.ts`, без barrel на всё приложение.

## Порядок переноса

1. Переместить предметные модели из `domain` в соответствующие `entities`, сохранив
   public APIs и разрешив только необходимые `@x` cross references.
2. Переместить HTTP транспорт в `shared/api`, endpoint/schema реализации — в
   `entities` или owning `features`; распределить прежние query hooks, session
   lifecycle и чистые сценарии в `model/api` соответствующих feature slices.
3. Собрать per-slice providers/ports через `bootstrap`; сохранить real/demo
   implementations и независимость сценариев от React и HTTP.
4. Перенести текущие экраны в `pages`, локальные actions в `features`, общие UI и
   браузерные адаптеры в `shared`; оставить `routes` тонким registry.
5. Закрепить layer order, slice isolation, alias public APIs и source coverage
   тестом архитектуры; точечно обновить README и AGENTS, не меняя продуктовых правил.

Никакие правила API/UX не меняются. Названия файлов, экспортов и CSS-классов
остаются продуктовыми; `demo` сохраняет смысл режима приложения.

## Сохранение поведения и проверки

- Сохраняются реальные HTTP-пути, cookies, auth refresh, таймауты и режимы env.
- Сохраняются ключи идемпотентности при повторе и восстановлении диагностики.
- StrictMode не создаёт дублирующие сессии; выход и смена пользователя отменяют
  старые операции и не позволяют поздним ответам заполнить кеш нового пользователя.
- Отмена освобождает MediaStream, playback, object URLs и таймеры.
- Сохраняются query keys, инвалидация и отсутствие автоматического повтора
  административного импорта и отправки диагностического ответа.
- Существующие тесты маршрутов, auth refresh, real/mock и гонок сессии остаются
  обязательными. Добавляются сценарные тесты для перенесённой оркестрации и
  тест импортов по слоям с разрешением путей, а не только списком имён функций.
- После каждого переноса запускаются затронутые тесты и typecheck; перед
  передачей — format:check, lint, typecheck, все тесты и production build.

Исходная проверка 2026-10-08: lint, typecheck и build успешны;
Vitest: 21 файл, 119 тестов, все успешны. Проверка format:check ещё не запускалась.

## Критерий завершения

Все frontend modules находятся в согласованных FSD слоях и slices; межслойные
зависимости направлены вниз, межслайсовые импорты проходят через public APIs,
а чистые сценарии тестируются через переданные порты. Реализации real/demo
собирает `bootstrap`. Сохраняются UI, URL, HTTP контракты и перечисленные
сценарии восстановления/отмены. FSD guard охватывает TypeScript source,
направления импортов, public APIs, динамические импорты и прямой `fetch`.
Все финальные проверки проходят. В рабочих именах и описаниях не используются
обозначения прототипа; `.prototype` остаётся только как стандартное свойство JS.


## Фактический результат

- Приложение организовано по слоям `bootstrap`, `pages`, `features`, `entities` и `shared`; технический TanStack route registry сохранён отдельно.
- Чистый запуск диагностики использует внедряемые API/session/storage/identity порты; QueryClient остаётся в hook/provider orchestration. Сохранены idempotency и защита от позднего ответа при смене пользователя.
- HTTP transport находится в `shared/api`; предметные endpoints у owning entities/features. Demo mock handler регистрируется из bootstrap без импорта bootstrap в transport; исходная env-based проверка режима сохранена.
- Голосовые действия живут в `features/voice-answer`, чистый feedback parser — в `entities/assessment`, payload builder — в `features/trainer-session`.
- Архитектурный тест `services/frontend/tests/shared/api/fsd-boundaries.test.ts` проверяет layer order, slice isolation, cross-slice public APIs (включая relative и dynamic imports), source coverage и отсутствие прямого fetch вне shared transport; сценарии диагностики и parser имеют unit tests.
- Все frontend-тесты находятся в корневом `services/frontend/tests/`, повторяя иерархию `src/`. Настройка среды — `tests/setup.ts`, общие тестовые обёртки — `tests/support/`; в `src/` тестовых модулей нет. Vitest явно ищет тесты в `tests/`, TypeScript проверяет обе иерархии.
- Финальные проверки фиксируются после завершения миграции: format:check, lint, typecheck, Vitest и production build.
