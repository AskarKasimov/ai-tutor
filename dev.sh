#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

for config in services/backend/config/dev.yaml services/course-graph/config/dev.yaml services/llm-gateway/config/dev.yaml; do
    if [ ! -f "$config" ]; then
        printf '%s\n' "Missing $config; restore the service YAML configuration." >&2
        exit 1
    fi
done
if [ ! -f .env ]; then
    printf '%s\n' '.env is missing. Copy and configure it: cp .env.example .env' >&2
    exit 1
fi

docker compose version >/dev/null
docker compose --env-file .env -f docker-compose.yaml -f docker-compose.dev.yaml config --quiet

mkcert -install
mkdir -p certs
mkcert -cert-file certs/server.pem -key-file certs/server-key.pem localhost 127.0.0.1 ::1

# Compose resolves dotenv quoting, comments, interpolation, and shell overrides.
https_port=$(docker compose --env-file .env -f docker-compose.yaml -f docker-compose.dev.yaml config --environment | sed -n 's/^PROXY_HTTPS_PORT=//p')
case "$https_port" in
    ''|*[!0-9]*) printf '%s\n' 'PROXY_HTTPS_PORT must be numeric.' >&2; exit 1 ;;
esac
app_url=https://localhost:$https_port
admin_url="$app_url/admin/courses"
printf '%s\n' "Start page: $app_url" "Course admin: $admin_url"

# Grafana is optional; the LLM gateway itself is always part of the app stack.
grafana_url=http://localhost:3000/d/llm-gateway-routing
profiles=${COMPOSE_PROFILES-$(sed -n 's/^COMPOSE_PROFILES=//p' .env | tail -n 1)}
case ",$profiles," in
*,observability,*) printf '%s\n' "Grafana:      $grafana_url" ;;
*) grafana_url= ;;
esac

# Open the sites and Grafana once health answers; DEV_OPEN_BROWSER=false skips it.
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
        if [ -n "$grafana_url" ]; then
            attempts=24
            until curl -fs -o /dev/null http://localhost:3000/api/health; do
                attempts=$((attempts - 1))
                if [ "$attempts" -le 0 ]; then
                    printf '%s\n' "Grafana did not answer; open $grafana_url manually." >&2
                    exit 0
                fi
                sleep 5
            done
            "$opener" "$grafana_url" >/dev/null 2>&1
        fi
    ) &
fi

# The root Compose creates its managed dev network before starting containers.
exec docker compose --env-file .env -f docker-compose.yaml -f docker-compose.dev.yaml up --build "$@"
