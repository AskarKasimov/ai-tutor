# Правила course-graph

Дополняют [корневой AGENTS.md](../../AGENTS.md) для `services/course-graph`. Этапы конвейера и их статус — в [README](README.md) и [корневом README](../../README.md#course-graph-граф-знаний-курса). Команды выполняются из `services/course-graph`.

- Сервис выполняет только этапы обработки. Оркестрацию, хранение графа и админку делает backend — не пишите из сервиса в PostgreSQL и S3.
- Изменили или добавили эндпоинт — обновите [api/course-graph.yaml](../../api/course-graph.yaml) и адаптер backend `internal/features/materials/infrastructure/ocr`. Сервис внутренний: не публикуйте его через Caddy.
- Тяжёлые зависимости (Docling, PyTorch) держите только здесь; зависимости ведите в `pyproject.toml` через `uv add` (Docling — extra `ocr`), обновляйте `uv.lock`.
- LLM вызывайте только через LLM gateway с `complexity`, без `model`.
- Граф на выходе — по `Graph` из контракта: названия и тексты вершин на русском, без циклов. Источники хранят точные цитаты и provenance snapshot материала (неделя, вид, подпись, имя, revision); при кэшированных темах Python сверяет цитаты с текущим Markdown и обновляет эти поля только из входного snapshot backend. Новые сборки backend запускает только для обработанных и классифицированных материалов.
- Тесты кладите в `tests/` (pytest). Они не скачивают модели Docling: используйте Markdown или текст либо подменяйте конвертер.
- Проверки: `uv lock --check`, `uv run --locked --extra cpu --extra ocr python -m pytest tests`, `docker compose --env-file ../../.env.example config --quiet`.
