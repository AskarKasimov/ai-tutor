# Локальное S3-хранилище

`services/s3/docker-compose.yaml` запускает закреплённый образ RustFS с постоянным томом `ai-tutor_task_audio_data`. Сервис доступен backend внутри `ai-tutor_default` как `http://s3:9000`; опубликованный порт привязан только к loopback. Корзину `task-audio` backend создаёт через S3 API, если `storage.s3.create_bucket` в `services/backend/config/dev.yaml` равно `true`.

Backend хранит здесь исходные файлы материалов курса и их распознанный Markdown (ключи объектов записаны в `subject_materials`), а также кэш озвучки реплик Geek — целая реплика `geek/{session}/{turn}.wav` и части реплики (реакция, вопрос) по тексту и настройкам голоса `geek/speech/{sha256}.wav`. При старте API в фоне проверяет корзину и создаёт её, если это разрешено. Имя тома и корзины `task-audio` осталось от прежней озвучки заданий.

Для локального Compose задайте уникальные `S3_ACCESS_KEY` и `S3_SECRET_KEY`; backend получает их через `BACKEND_S3_ACCESS_KEY` и `BACKEND_S3_SECRET_KEY`. Bucket закрыт, браузер обращается только к API. Для внешнего S3 задайте endpoint, region и bucket в `services/backend/config/dev.yaml`, положите credentials в локальный `.env` и установите `storage.s3.create_bucket: false`. `docker-compose.external-s3.yaml` отключает RustFS; используется с корневым Compose для dev-стека. Для override требуется Docker Compose 2.24.4 или новее.

Переключение endpoint само не переносит объекты. Скопируйте сохранённые объекты в новое хранилище перед переключением, иначе материалы курса станут недоступны (кэш озвучки пересоздаётся сам). Обычный `docker compose down` сохраняет данные; не удаляйте volume без осознанного удаления материалов.

Проверка конфигурации: `docker compose --env-file ../../.env.example config --quiet`. Проверка API RustFS: `curl --fail http://localhost:9000/health`.
