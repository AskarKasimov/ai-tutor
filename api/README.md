# Единый HTTP API

Контракт: [openapi.yaml](openapi.yaml), OpenAPI 3.1. Все пути относительны к `/api/v1`.

- `x-implementation: implemented` — подключённая операция: auth, предметы, voice, удаление и скачивание материалов, диагностика Geek (`/diagnostic-sessions`, `/{id}`, `/{id}/answers`, `/{id}/result`, `/{id}/turns/{turn_id}/audio`), health. Операции прежней модели (карта, банк заданий, assessment, taskgen, variantgen, план обучения, тренировка, озвучка заданий) удалены 16.10.2026 вместе с кодом.
- `x-implementation: planned` — проект до реализации; наличие planned-операции в Swagger не означает наличие handler.

Контракт Geek согласован 16.10.2026 и описан как planned, схемы — с префиксом `geek.`:

- материалы: загрузка с неделей и видом, список, правка `PATCH /admin/subjects/{subject_id}/materials/{material_id}`;
- граф курса: `POST`/`GET /admin/subjects/{subject_id}/graph/build`, `GET /admin/subjects/{subject_id}/graph`;
- диагностика — реализована 16.10.2026. Ограничение «ровно одно из `transcription_id`, `text`, `dont_know`» в `geek.AnswerRequest` проверяет код: генератор не выражает `oneOf`;
- пакет для преподавателя: `GET /admin/subjects/{subject_id}/diagnostic-sessions`, `GET /admin/diagnostic-sessions/{id}/report`.

Загрузка и список материалов уже подключены прежними handler без недели и вида; контрактный тест `TestOpenAPIResponses` проверяет только implemented-операции, поэтому их ответы не сверяются, пока операции не переведут в implemented.

Внутренний API Python-сервиса course-graph — отдельный контракт [course-graph.yaml](course-graph.yaml): OCR, задание на сборку графа и его статус. Он не публикуется через Caddy.

Фикстура графа — [fixtures/course-graph.json](fixtures/course-graph.json): `Graph` по `course-graph.yaml`, 10 тем демокурса (поиск, недели 3 и 5–7) на русском. В ней есть цепочка пререквизитов глубиной 3, общие пререквизиты у тем верхнего уровня (`heuristic`, `uninformed-search`, `exploration-exploitation`), тема без связей (`agents-peas`) и одна тема с непустыми ОР и вопросами (`a-star`). Верхний уровень: `a-star`, `simulated-annealing`, `mcts`, `agents-peas`. Подходит для тестов экзамена, моков frontend и как запасной граф для показа.

Полная русская фикстура курса Introduction to AI — [fixtures/intro-to-ai.ru.graph.json](fixtures/intro-to-ai.ru.graph.json): 132 учебные темы и 20 диагностических групп, каждая сериализована как обычный `GraphNode`; порядок вершин топологический. Пять тем без учебного материала исключены. Направление 145 исходных учебных рёбер сохранено; у каждой диагностической группы её верховые темы указаны как `prerequisites`. В группах есть черновые комплексные вопросы. Это фикстура переведённого содержимого для интеграции с Geek, не новая версия OpenAPI-контракта.

Из `services/backend` контракт обновляется командой `bash scripts/generate-openapi.sh`. Генератор обновляет только операции, уже присутствующие в контракте, и сохраняет planned. Новую работающую ручку сначала явно добавьте в `openapi.yaml` со статусом `implemented`, затем запустите генератор. CI проверяет повторяемость генерации; контрактные тесты проверяют схемы и реальные ответы действующих операций. Swagger читает `api/openapi.yaml` через Compose.

Правила продукта — [README](../README.md).
