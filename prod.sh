#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

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
services_order='postgresql s3 backend frontend caddy'
if [ "$s3_local_enabled" = false ]; then
    services_order='postgresql backend frontend caddy'
fi
# Validate every project before changing the server.
for service in $services_order; do
    compose "$service" config --quiet
done

for service in $services_order; do
    compose "$service" up --build --wait "$@"
done
