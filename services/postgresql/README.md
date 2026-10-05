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
