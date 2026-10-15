# Единый HTTP API

Контракт: [openapi.yaml](openapi.yaml), OpenAPI 3.1. Все пути относительны к `/api/v1`.

- `x-implementation: implemented` — подключённая операция. Сейчас все операции контракта implemented: auth, предметы, импорт и чтение карты, банк заданий, voice, assessment (включая `/assessments/overall-feedback`), taskgen и материалы, variantgen, диагностические сессии и их фидбэк, план обучения (`/learning-plans`), прежняя тренировка (`/training-sessions`, без UI), озвучка заданий (`/task-audio`) и health.
- `x-implementation: planned` — проект до реализации; наличие planned-операции в Swagger не означает наличие handler.

Из `services/backend` контракт обновляется командой `bash scripts/generate-openapi.sh`. Генератор обновляет только операции, уже присутствующие в контракте, и сохраняет planned. Новую работающую ручку сначала явно добавьте в `openapi.yaml` со статусом `implemented`, затем запустите генератор. CI проверяет повторяемость генерации; контрактные тесты проверяют схемы и реальные ответы действующих операций. Swagger читает `api/openapi.yaml` через Compose.

Правила продукта — [README](../README.md); правила отбора — [алгоритм](../documents/Алгоритм_составления_варианта.md).
