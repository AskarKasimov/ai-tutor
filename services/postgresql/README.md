# PostgreSQL

Собственный Compose запускает только БД. Из этой папки:

```bash
docker compose --env-file ../../.env up -d
docker compose --env-file ../../.env ps
```

Сеть `ai-tutor_default` должна существовать.
БД доступна внутри неё как `db:5432`; порт на хосте задаёт `DB_PORT`.
Данные хранятся в volume `ai-tutor_postgres_data`.
Корневой dev-стек включает этот же Compose и создаёт сеть автоматически.
При изменении пароля в `.env` пароль существующей БД сам не изменяется.
Обычный `down` сохраняет volume; `down -v` удаляет данные.

Goose при старте API штатно применяет к существующему volume миграции
`00001`–`00010` (список — в [backend README](../backend/README.md#бд-и-импорт)).
`00003_task_audio.sql` создаёт очередь и ссылки на задания/снимки, но не генерирует
WAV: backend worker обрабатывает backlog после старта. Для состояния очереди выполните:

```sql
SELECT id, status, attempts, last_error_code, bucket, storage_uri, updated_at
FROM audio_assets
ORDER BY created_at, id;
```

Не выбирайте `instruction` в общих диагностических запросах. Правила отбора:
[алгоритм](../../documents/Алгоритм_составления_варианта.md).
