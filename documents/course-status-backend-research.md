# Статусы подготовки курса: исследование backend

Исследование исходников для спецификации Luna. Это факты текущей реализации и рекомендации, а не изменение продуктовых правил. Runtime-запросы, модели и БД не проверялись.

## Подтверждённое текущее поведение

- Общий `Subject` содержит только `id`, `name`, `ready`, `learning_ready`, `exam_ready`: [model.go](../services/backend/internal/entities/subject/model.go#L3).
- Каталоги `/exam/subjects` и `/learn/subjects` используют один репозиторий. `ready` проецируется из соответствующего флага; admin получает все курсы, остальные — только готовые в текущем направлении: [service.go](../services/backend/internal/features/subject/application/service.go#L64), [handler.go](../services/backend/internal/features/subject/transport/http/handler.go#L87).
- Экзамен готов, если опубликован graph из карты либо graph из материалов при наличии хотя бы одного материала. Ни текущий job, ни ошибки OCR, ни stale не входят в `exam_ready`: [subject.sql](../services/backend/internal/shared/postgres/queries/subject.sql#L8). Это соответствует чтению доступного Geek graph: [course_graph.sql](../services/backend/internal/shared/postgres/queries/course_graph.sql#L96).
- Обучение готово при наличии минимум трёх различных подходящих ОР внутри одной составляющей с допустимыми заданиями. Наличие успешно импортированной карты само по себе не означает готовность: [subject.sql](../services/backend/internal/shared/postgres/queries/subject.sql#L14), [backend README](../services/backend/README.md#L32).

### Экзамен: durable состояния и источники

- `course_graph_jobs`: `pending`, `processing`, `ready`, `failed`; отсутствие job отображается `not_requested`. `pending` включает автоматические повторные попытки с задержкой, поэтому ненулевой `error_message` при `pending` не означает окончательный сбой: [course_graph.sql](../services/backend/internal/shared/postgres/queries/course_graph.sql#L73).
- `GET /exam/admin/subjects/{subject_id}/graph` уже возвращает snapshot: статус последней запрошенной сборки, старый опубликованный graph, `graph_source`, `build_source`, последнюю импортированную карту, `stale`, `inputs_ready`, `progress`: [model.go](../services/backend/internal/entities/coursegraph/model.go#L34), [handler.go](../services/backend/internal/features/coursegraph/transport/http/handler.go#L84).
- `graph_source` описывает опубликованный источник, `build_source` — последнюю запрошенную сборку. Они могут различаться. `stale` сравнивает fingerprint именно опубликованного источника с его текущими входами: [course_graph.sql](../services/backend/internal/shared/postgres/queries/course_graph.sql#L78).
- Импорт экзаменационной карты сохраняет карту и ставит graph job одной транзакцией. Если сборка уже queued/running, изменения не коммитятся: [repository.go](../services/backend/internal/features/coursegraph/infrastructure/postgres/repository.go#L68).
- Публикация новой версии атомарно заменяет graph только при совпадении текущего fingerprint и lease-токена. До успешной публикации старый graph сохраняется, в том числе при сбое новой сборки: [course_graph.sql](../services/backend/internal/shared/postgres/queries/course_graph.sql#L59).
- Сборка из материалов запускается кнопкой, автоматически после завершения OCR не стартует. `inputs_ready` требует непустой набор материалов, у каждого `parsed`, cleaning `ready` и заполненные допустимые week/kind: [course_graph.sql](../services/backend/internal/shared/postgres/queries/course_graph.sql#L1), [migration 00009](../services/backend/internal/shared/postgres/migrations/00009_material_metadata_graph_inputs.sql#L3).
- Постоянный state сборки переживает рестарт; подробный `progress` живёт в памяти API и может временно отсутствовать: [model.go](../services/backend/internal/entities/coursegraph/model.go#L51).

### Материалы

- Статус материала: `pending|parsed|failed`; это прежде всего результат OCR. OCR job и cleaning job имеют собственные `pending|processing|ready|failed`: [model.go](../services/backend/internal/entities/material/model.go#L17), [cleaning.go](../services/backend/internal/entities/material/cleaning.go#L3).
- List материалов агрегирует `processing` из OCR и активной очистки, но окончательный failure очистки нужно проверять отдельно через `cleaning.status`, а не только через `material.status` или `processing.status`: [repository.go](../services/backend/internal/features/materials/infrastructure/postgres/repository.go#L45).
- OCR/cleaning после временной ошибки возвращаются в pending, terminal failed устанавливается после трёх попыток. При окончательном OCR-сбое также обновляется material.status; при cleaning-сбое OCR-текст остаётся доступным: [material_ocr.sql](../services/backend/internal/shared/postgres/queries/material_ocr.sql#L45), [material_cleaning.sql](../services/backend/internal/shared/postgres/queries/material_cleaning.sql#L38).
- Замена исходника атомарно переводит материал обратно в pending и очищает прежнюю обработку; граф из материалов становится stale, но старое опубликованное содержимое не удаляется: [subject_materials.sql](../services/backend/internal/shared/postgres/queries/subject_materials.sql#L39), [backend README](../services/backend/README.md#L201).

### Обучение

- Импорт карты синхронный: parse до записи, затем replace одной транзакцией. Отдельного durable import job со статусом importing/failed нет: [service.go](../services/backend/internal/features/competency/application/service.go#L45), [repository.go](../services/backend/internal/features/competency/infrastructure/postgres/repository.go#L28).
- Активная редакция обновляется в конце той же транзакции. Ошибка до commit сохраняет предыдущую карту. Пока первая загрузка не закоммичена, другие запросы видят отсутствие карты; при замене — предыдущую активную редакцию: [repository.go](../services/backend/internal/features/competency/infrastructure/postgres/repository.go#L263), [learning_competency.sql](../services/backend/internal/shared/postgres/queries/learning_competency.sql#L4).
- Признак наличия карты — активная редакция > 0 / соответствующий import, а не количество компетенций или заданий: успешная карта может не давать ни одного подходящего диагностического блока.
- TTS ставится фоновыми заданиями при импорте, но готовность обучения не зависит от аудио: [repository.go](../services/backend/internal/features/competency/infrastructure/postgres/repository.go#L255), [subject.sql](../services/backend/internal/shared/postgres/queries/subject.sql#L14). Состояния TTS нельзя выдавать за подготовку курса.

## Рекомендуемая минимальная модель контракта

Это предложение исследователя; окончательные названия и UX фиксирует основная спецификация.

1. Сохранить существующие ready-поля и фильтрацию студентов без изменения семантики. Доступность и текущая подготовка ортогональны: `exam_ready=true` совместим с активной/провалившейся пересборкой.
2. Добавить optional admin-only `preparation: { exam: {...}, learn: {...} }` в общую модель Subject для обоих каталогов. Вычислять batch SQL без per-course вызовов graph/material endpoints и без новых таблиц/миграций.
3. Для exam достаточно итогового state, effective source и stale; исходные факты агрегировать на backend. Источник подготовки: последняя запрошенная сборка, иначе опубликованный graph, иначе импортированная карта, иначе материалы. Материалы не должны заставлять отображать processing/failed у подготовки по карте.
4. При текущих входах без публикации и без job отображать «нужно собрать граф», а не «идёт подготовка». Отдельно различать незаполненные metadata, терминальную ошибку и отсутствие исходников.
5. `pending` отображать как очередь/ожидание, `processing` — как выполняющуюся работу. Временную ошибку при pending не выдавать за terminal failure. Отсутствующий progress не меняет persisted processing на empty/ready.
6. Исторический failed job с fingerprint, отличающимся от текущих входов, не должен блокировать новую загрузку: показывать состояние текущих входов (обработка, metadata, необходимость сборки). Наличие старого job после удаления всех исходников не означает активную подготовку; сначала проверять наличие исходников выбранного направления.
7. Для learn устойчивые состояния сейчас: empty (активной редакции нет), ready (существующая проверка true), needs_action (карта есть, но диагностика недоступна). uploading/importing — только локальное состояние активного запроса импорта, не глобальный сохранённый статус. Ошибка импорта показывается пользователю как ошибка операции и не заменяет состояние сохранённой карты.
8. В UI ready-флаг сохраняет сообщение о доступной опубликованной версии параллельно processing/failed/stale. Не скрывать доступность курса только потому, что запускается обновление.

## Обязательные пограничные сценарии

- Новый курс без обеих карт и PDF.
- Только учебная карта; только экзаменационная карта; PDF без учебной карты: состояния направлений независимы.
- OCR pending/processing/retry/terminal failed; OCR ready и cleaning pending/processing/failed.
- Материалы обработаны, metadata неполные; metadata готовы, но сборка ещё не запрошена.
- Graph job pending/processing/failed без первой публикации.
- Старый опубликованный graph плюс pending/processing/failed пересборка; старый graph и изменённые входы без пересборки.
- Экзаменационная карта доступна, а посторонние PDF ещё обрабатываются или упали.
- Исторический failed job после новых входов или удаления последнего материала.
- Учебная карта успешно импортирована, но готовность false (недостаточно подходящих ОР); первая неуспешная загрузка; неуспешная замена прежней готовой карты.
- Учебное TTS pending/failed при learning_ready=true; каталог не меняет готовность.
- Запрос каталога/БД падает: вернуть ошибку, не ложное empty.

## Замеченное расхождение документации

Swag Description каталога экзамена по-прежнему упоминает готовность через фикстуру graph: [handler.go](../services/backend/internal/features/subject/transport/http/handler.go#L74). Текущие SQL и backend README определяют её через опубликованный graph и источник; при изменении DTO нужно исправить аннотацию и перегенерировать OpenAPI.

## Проверки этого исследования

Прочитаны исходники моделей, SQL, HTTP handlers, repositories и backend README. Код приложения не менялся, поэтому тесты приложения, DB-тесты и runtime-проверки не запускались. Ссылки указывают на реальные файлы и существующие строки на момент исследования.
