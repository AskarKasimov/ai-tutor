# Локальное S3-хранилище заданий

`services/s3/docker-compose.yaml` запускает закреплённый образ RustFS с постоянным томом `ai-tutor_task_audio_data`. Сервис доступен backend внутри `ai-tutor_default` как `http://s3:9000`; опубликованный порт привязан только к loopback. Корзину `task-audio` backend создаёт через S3 API, если `BACKEND_S3_CREATE_BUCKET=true`.

Импорт и taskgen сохраняют pending metadata в PostgreSQL; startup worker с ограниченным числом lanes синтезирует WAV, проверяет его и загружает в приватную корзину. После пяти попыток asset получает `failed`; после рестарта истёкшие leases возвращаются в очередь. `GET /diagnostic-sessions/{id}/current/audio` читает только статус/URL, а `/task-audio/{id}/file` отдаёт существующий объект. HTTP handlers не вызывают TTS.

Для локального Compose задайте уникальные `S3_ACCESS_KEY` и `S3_SECRET_KEY`; backend получает их через `BACKEND_S3_ACCESS_KEY` и `BACKEND_S3_SECRET_KEY`. Bucket закрыт, браузер обращается только к API. Для внешнего S3 задайте `BACKEND_S3_ENDPOINT`, регион, существующий bucket и его credentials, затем установите `BACKEND_S3_CREATE_BUCKET=false` и `S3_LOCAL_ENABLED=false`.

Переключение endpoint само не переносит объекты. Скопируйте каждый сохранённый bucket/key в новое хранилище и обновите bucket/storage_uri в `audio_assets` перед переключением, иначе готовые ссылки останутся недоступными. Обычный `docker compose down` сохраняет данные; не удаляйте volume без осознанного удаления аудио.

Проверка конфигурации: `docker compose --env-file ../../.env.example config --quiet`. Проверка API RustFS: `curl --fail http://localhost:9000/health`.
