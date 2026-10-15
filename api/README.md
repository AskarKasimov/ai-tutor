# Единый HTTP API

Контракт: [openapi.yaml](openapi.yaml), OpenAPI 3.1. Все пути относительны к `/api/v1`.

- `x-implementation: implemented` — подключённая операция: auth, предметы, импорт и чтение карты, банк заданий, voice, assessment, taskgen, удаление и скачивание материалов, variantgen, план обучения (`/learning-plans`, вход — `/diagnostic-sessions/{id}/learning-plan`), прежняя тренировка (`/training-sessions`, без UI), озвучка заданий (`/task-audio`) и health. В коде подключены, но в контракте не описаны: `POST /assessments/overall-feedback`, `POST /training-sessions/{id}/answers/reset`, `POST /training-sessions/{id}/current/audio/regenerate`.
- `x-implementation: planned` — проект до реализации; наличие planned-операции в Swagger не означает наличие handler.

Контракт Geek согласован 16.10.2026 и описан как planned, схемы — с префиксом `geek.`:

- материалы: загрузка с неделей и видом, список, правка `PATCH /admin/subjects/{subject_id}/materials/{material_id}`;
- граф курса: `POST`/`GET /admin/subjects/{subject_id}/graph/build`, `GET /admin/subjects/{subject_id}/graph`;
- диагностика на месте прежней: `POST /diagnostic-sessions`, `GET /diagnostic-sessions/{id}`, `POST /diagnostic-sessions/{id}/answers`, `GET /diagnostic-sessions/{id}/turns/{turn_id}/audio`, `GET /diagnostic-sessions/{id}/result`. Прежние операции диагностики по варианту (`current/audio`, `regenerate`, `answers/transcription`, `skip`, `feedback`) из контракта удалены;
- пакет для преподавателя: `GET /admin/subjects/{subject_id}/diagnostic-sessions`, `GET /admin/diagnostic-sessions/{id}/report`.

Пока код диагностики и загрузки материалов прежний, контрактный тест `TestOpenAPIResponses` для этих операций не проходит; он снова пройдёт, когда их реализуют.

Внутренний API Python-сервиса course-graph — отдельный контракт [course-graph.yaml](course-graph.yaml): OCR, задание на сборку графа и его статус. Он не публикуется через Caddy.

Из `services/backend` контракт обновляется командой `bash scripts/generate-openapi.sh`. Генератор обновляет только операции, уже присутствующие в контракте, и сохраняет planned. Новую работающую ручку сначала явно добавьте в `openapi.yaml` со статусом `implemented`, затем запустите генератор. CI проверяет повторяемость генерации; контрактные тесты проверяют схемы и реальные ответы действующих операций. Swagger читает `api/openapi.yaml` через Compose.

Правила продукта — [README](../README.md); правила отбора — [алгоритм](../documents/Алгоритм_составления_варианта.md).
