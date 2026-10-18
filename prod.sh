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
compose() {
    compose_service=$1
    shift
    docker compose --env-file .env -f "services/$compose_service/docker-compose.yaml" "$@"
}

s3_local_enabled=$(sed -n 's/^S3_LOCAL_ENABLED=//p' .env | tail -n 1)
: "${s3_local_enabled:=true}"
services_order='postgresql s3 course-graph llm-gateway backend frontend caddy'
if [ "$s3_local_enabled" = false ]; then
    services_order='postgresql course-graph llm-gateway backend frontend caddy'
fi
# Validate every project before changing the server.
for service in $services_order; do
    compose "$service" config --quiet
done

for service in $services_order; do
    compose "$service" up --build --wait "$@"
done

# On a machine with a desktop, open the sites and, with the gateway profile,
# Grafana; a headless server has no opener. PROD_OPEN_BROWSER=false skips it.
https_port=$(sed -n 's/^PROXY_HTTPS_PORT=//p' .env | tail -n 1)
app_url=https://localhost
if [ -n "$https_port" ] && [ "$https_port" != 443 ]; then
    app_url="$app_url:$https_port"
fi
grafana_url=http://localhost:3000/d/llm-gateway-routing
profiles=${COMPOSE_PROFILES-$(sed -n 's/^COMPOSE_PROFILES=//p' .env | tail -n 1)}
case ",$profiles," in
*,gateway,*) ;;
*) grafana_url= ;;
esac
printf '%s\n' "Student site: $app_url" "Admin panel:  $app_url/admin"
if [ -n "$grafana_url" ]; then
    printf '%s\n' "Grafana:      $grafana_url"
fi
opener=
if command -v xdg-open >/dev/null 2>&1; then
    opener=xdg-open
elif command -v open >/dev/null 2>&1; then
    opener=open
fi
if [ "${PROD_OPEN_BROWSER:-true}" != false ] && [ -n "$opener" ]; then
    "$opener" "$app_url" >/dev/null 2>&1 || true
    "$opener" "$app_url/admin" >/dev/null 2>&1 || true
    if [ -n "$grafana_url" ]; then
        "$opener" "$grafana_url" >/dev/null 2>&1 || true
    fi
fi
