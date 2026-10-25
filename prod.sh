#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

gateway_config=services/llm-gateway/config.json
if [ ! -f "$gateway_config" ]; then
    cp services/llm-gateway/config.example.json "$gateway_config"
fi

if ! command -v docker >/dev/null 2>&1; then
    printf '%s\n' 'Docker with Compose is required. See README.md.' >&2
    exit 1
fi
if [ ! -f .env ]; then
    printf '%s\n' \
        '.env is missing. Copy the complete template: cp .env.prod.example .env' \
        'Review all settings in .env before starting; see README.md for requirements.' >&2
    exit 1
fi
docker compose version >/dev/null
docker compose --env-file .env -f docker-compose.yaml config --quiet

# Build and start the application as one Compose project. Persistent volume
# names are declared in docker-compose.yaml so a project-name change cannot
# silently switch the database or stored audio.
docker compose --env-file .env -f docker-compose.yaml up --build --detach --wait "$@"

https_port=$(sed -n 's/^PROXY_HTTPS_PORT=//p' .env | tail -n 1)
app_url=https://localhost
if [ -n "$https_port" ] && [ "$https_port" != 443 ]; then
    app_url="$app_url:$https_port"
fi
printf '%s\n' \
    "Start page:   $app_url" \
    "Course admin: $app_url/admin/courses"

profiles=${COMPOSE_PROFILES-$(sed -n 's/^COMPOSE_PROFILES=//p' .env | tail -n 1)}
case ",$profiles," in
    *,observability,*) printf '%s\n' 'Grafana:      http://localhost:3000/d/llm-gateway-routing' ;;
esac

opener=
if command -v xdg-open >/dev/null 2>&1; then
    opener=xdg-open
elif command -v open >/dev/null 2>&1; then
    opener=open
fi
if [ "${PROD_OPEN_BROWSER:-true}" != false ] && [ -n "$opener" ]; then
    "$opener" "$app_url" >/dev/null 2>&1 || true
    "$opener" "$app_url/admin/courses" >/dev/null 2>&1 || true
fi
