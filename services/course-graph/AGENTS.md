# Правила course-graph

Дополняют [корневой AGENTS.md](../../AGENTS.md) для `services/course-graph`. Этапы конвейера и их статус — в [README](README.md) и [корневом README](../../README.md#course-graph-граф-знаний-курса). Команды выполняются из `services/course-graph`.

- Сервис выполняет только этапы обработки. Оркестрацию, хранение графа и админку делает backend — не пишите из сервиса в PostgreSQL и S3.
- Изменили или добавили эндпоинт — обновите [api/course-graph.yaml](../../api/course-graph.yaml) и адаптер backend `internal/features/materials/infrastructure/ocr`. Сервис внутренний: не публикуйте его через Caddy.
- Тяжёлые зависимости (Docling, PyTorch) держите только здесь; новую добавьте в `requirements.txt`.
- LLM вызывайте только через LLM gateway с `complexity`, без `model`.
- Граф на выходе — по `Graph` из контракта: названия и тексты вершин на русском, без циклов. Недели и источников у вершины нет, пока это не решено в README.
- Тесты кладите в `tests/` (pytest). Они не скачивают модели Docling: используйте Markdown или текст либо подменяйте конвертер.
- Проверки: `python -m pytest tests`, `docker compose --env-file ../../.env.example config --quiet`.
