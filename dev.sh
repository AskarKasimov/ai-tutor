#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

gateway_config=services/llm-gateway/config.json
if [ ! -f "$gateway_config" ]; then
    cp services/llm-gateway/config.example.json "$gateway_config"
fi

if [ ! -f .env ]; then
    printf '%s\n' '.env is missing. Copy and configure it: cp .env.example .env' >&2
    exit 1
fi

docker compose version >/dev/null
# Geek and material processing require the gateway, including with an older .env.
compose() {
    docker compose --env-file .env -f docker-compose.yaml --profile gateway "$@"
}
compose config --quiet

# Diagnose a host PostgreSQL conflict before certificates, builds and containers.
db_host_port=$(compose config --environment | sed -n 's/^DB_PORT=//p')
if command -v ss >/dev/null 2>&1 && [ -n "$db_host_port" ] && \
    [ -z "$(compose ps -q db)" ] && \
    [ -n "$(ss -H -ltn "sport = :$db_host_port")" ]; then
    printf '%s\n' \
        "Host port $db_host_port is already in use. Stop the local PostgreSQL service or change DB_PORT in .env." >&2
    exit 1
fi

mkcert -install
mkdir -p certs
mkcert -cert-file certs/server.pem -key-file certs/server-key.pem localhost 127.0.0.1 ::1

app_url=https://localhost:8443
admin_url="$app_url/admin"
printf '%s\n' "Student site: $app_url" "Admin panel:  $admin_url"

# Open both sites once health answers; DEV_OPEN_BROWSER=false skips it.
opener=
if command -v xdg-open >/dev/null 2>&1; then
    opener=xdg-open
elif command -v open >/dev/null 2>&1; then
    opener=open
fi
if [ "${DEV_OPEN_BROWSER:-true}" != false ] && [ -n "$opener" ]; then
    (
        # The first build of course-graph can take several minutes.
        attempts=180
        until curl -fsk -o /dev/null "$app_url/api/v1/health"; do
            attempts=$((attempts - 1))
            if [ "$attempts" -le 0 ]; then
                printf '%s\n' 'Health did not answer in 15 minutes; open the sites manually.' >&2
                exit 0
            fi
            sleep 5
        done
        "$opener" "$app_url" >/dev/null 2>&1
        "$opener" "$admin_url" >/dev/null 2>&1
    ) &
fi

# The root Compose creates its managed dev network before starting containers.
exec docker compose --env-file .env -f docker-compose.yaml --profile gateway up --build "$@"
