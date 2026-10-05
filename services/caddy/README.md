# Caddy

Собственный Compose запускает только HTTPS-прокси. Из этой папки:

```bash
docker compose --env-file ../../.env up -d
docker compose --env-file ../../.env exec proxy caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
```

Нужны существующая сеть `ai-tutor_default`,
backend `api:8002`, frontend `frontend:80` и сертификаты в `../../certs/`.
Для отдельных хостов настройте upstream в `Caddyfile`.
Swagger запускается Compose backend при `COMPOSE_PROFILES=swagger`.
Caddy использует то же значение env: включает reverse proxy `/docs/` в
`swagger:8080`, а при пустом значении возвращает 404 на `/docs` и `/docs/*`.
UI и спецификацию отдаёт Swagger; YAML не монтируется в Caddy. Порт и bind задают `PROXY_HTTPS_PORT`
и `PROXY_HTTPS_BIND`. Volumes сохраняют имена `ai-tutor_caddy_data` и
`ai-tutor_caddy_config`. Сертификаты остаются в корневой папке `certs/`,
из которой Caddy загружает их при запуске. Общий dev-стек включает этот же Compose.
