#!/bin/sh
# Runs two releases side by side behind one HTTPS entry point:
#   https://$DUAL_HOST        start page with one login for both
#   https://learn.$DUAL_HOST  learning (DUAL_LEARN_REF, v0.0.2.3)
#   https://exam.$DUAL_HOST   exam simulation (DUAL_EXAM_REF, latest master)
# Each release runs from its own git worktree with its own .env, volumes and
# network. Linux (and macOS); on Windows use windows-dual.ps1. See dual/README.md.
set -eu

dual_dir=$(CDPATH= cd -- "$(dirname -- "$0")/dual" && pwd)
repo_dir=$(CDPATH= cd -- "$dual_dir/../.." && pwd)

: "${DUAL_HOST:=127-0-0-1.sslip.io}"
: "${DUAL_HTTPS_BIND:=127.0.0.1}"
: "${DUAL_HTTPS_PORT:=443}"
: "${DUAL_MODE:=prod}"
: "${DUAL_LEARN_REF:=v0.0.2.3}"
: "${DUAL_EXAM_REF:=master}"
: "${DUAL_WORKTREES:=$(dirname -- "$repo_dir")}"

usage() {
    cat >&2 <<'EOF'
Usage: deploy/linux-dual.sh up|down|status|logs [learn|exam|front]

Environment (defaults in brackets):
  DUAL_HOST        base host name [127-0-0-1.sslip.io]; on a VM: <ip-with-dashes>.sslip.io
  DUAL_HTTPS_BIND  address to publish HTTPS on [127.0.0.1]; on a VM: 0.0.0.0
  DUAL_HTTPS_PORT  HTTPS port [443]
  DUAL_MODE        prod (built frontend and binary) or dev (Vite and Air) [prod]
  DUAL_LEARN_REF   git ref of the learning release [v0.0.2.3]
  DUAL_EXAM_REF    git ref of the exam simulation: a tag before the show [master]
  DUAL_WORKTREES   directory for the release worktrees [parent of the repository]
  DUAL_OPEN_BROWSER=false skips opening the browser
EOF
    exit 2
}

[ $# -ge 1 ] || usage
command=$1
shift

case "$DUAL_MODE" in
prod | dev) ;;
*) printf '%s\n' "DUAL_MODE must be prod or dev, got: $DUAL_MODE" >&2; exit 2 ;;
esac

if [ "$DUAL_HTTPS_PORT" = 443 ]; then
    origin_host=$DUAL_HOST
else
    origin_host="$DUAL_HOST:$DUAL_HTTPS_PORT"
fi
DUAL_ORIGIN="https://$origin_host"
# up picks the certificate in prepare_front_tls; other commands only need a value.
DUAL_TLS=internal
export DUAL_HOST DUAL_HTTPS_BIND DUAL_HTTPS_PORT DUAL_ORIGIN DUAL_TLS

# release <learn|exam>: sets ref, tree, project and published ports.
release() {
    case "$1" in
    learn) ref=$DUAL_LEARN_REF; ports='5442 8012 9010 8444'; profiles= ;;
    exam) ref=$DUAL_EXAM_REF; ports='5443 8013 9011 8445'; profiles=gateway ;;
    esac
    tree="$DUAL_WORKTREES/ai-tutor-$1"
    project="tutor-$1"
    set -- $ports
    db_port=$1 api_port=$2 s3_port=$3 proxy_port=$4
}

release_compose() {
    release "$1"
    shift
    docker compose -p "$project" --project-directory "$tree" --env-file "$tree/.env" \
        -f "$tree/docker-compose.yaml" -f "$tree/compose.dual.yaml" "$@"
}

front_compose() {
    docker compose -p tutor-dual -f "$dual_dir/docker-compose.yaml" "$@"
}

# set_env FILE KEY VALUE replaces or appends KEY=VALUE.
set_env() {
    tmp=$(mktemp "$1.XXXXXX")
    if grep -q "^$2=" "$1"; then
        awk -v k="$2" -v v="$3" 'index($0, k "=") == 1 { print k "=" v; next } { print }' "$1" > "$tmp"
    else
        cat "$1" > "$tmp"
        printf '%s=%s\n' "$2" "$3" >> "$tmp"
    fi
    mv -- "$tmp" "$1"
}

random_hex() {
    od -An -N"$1" -tx1 /dev/urandom | tr -d ' \n'
}

prepare_worktree() {
    release "$1"
    # A branch name follows origin (master is the latest master on every up),
    # a tag stays where it is.
    git -C "$repo_dir" fetch --tags --quiet origin ||
        printf '%s\n' "Warning: git fetch failed; using local refs." >&2
    if git -C "$repo_dir" rev-parse --verify --quiet "refs/remotes/origin/$ref" >/dev/null; then
        want=$(git -C "$repo_dir" rev-parse "refs/remotes/origin/$ref^{commit}")
    else
        want=$(git -C "$repo_dir" rev-parse --verify "$ref^{commit}")
    fi
    if [ ! -d "$tree" ]; then
        git -C "$repo_dir" worktree add --detach "$tree" "$want"
    fi
    have=$(git -C "$tree" rev-parse HEAD)
    if [ "$want" != "$have" ]; then
        if [ -n "$(git -C "$tree" status --porcelain --untracked-files=no)" ]; then
            printf '%s\n' "$tree has uncommitted changes; commit or discard them to switch to $ref." >&2
            exit 1
        fi
        git -C "$tree" switch --quiet --detach "$want"
        printf '%s\n' "Switched $tree to $ref ($(git -C "$tree" log --oneline -1))." \
            "New settings of $ref go to $tree/.env by hand; compare it with .env.example."
    fi

    if [ ! -f "$tree/.env" ]; then
        cp "$tree/.env.example" "$tree/.env"
        set_env "$tree/.env" DB_PASSWORD "$(random_hex 16)"
        set_env "$tree/.env" S3_ACCESS_KEY "$(random_hex 10)"
        set_env "$tree/.env" S3_SECRET_KEY "$(random_hex 20)"
        # Voice settings follow the main checkout when it has its own .env.
        if [ -f "$repo_dir/.env" ]; then
            grep -E '^BACKEND_(STT|TTS)_[A-Z_]+=' "$repo_dir/.env" | while IFS= read -r line; do
                set_env "$tree/.env" "${line%%=*}" "${line#*=}"
            done
        fi
        printf '%s\n' "Created $tree/.env from its template; review it if models or voice differ."
    fi
    set_env "$tree/.env" DB_PORT "$db_port"
    set_env "$tree/.env" S3_PORT "$s3_port"
    set_env "$tree/.env" PROXY_HTTPS_BIND 127.0.0.1
    set_env "$tree/.env" PROXY_HTTPS_PORT "$proxy_port"
    set_env "$tree/.env" COMPOSE_PROFILES "$profiles"
    # «К выбору режима» in the profile menu and logout through the start page.
    set_env "$tree/.env" VITE_MODE_SELECT_URL "$DUAL_ORIGIN"

    gateway_config="$tree/services/llm-gateway/config.json"
    if [ -f "$tree/services/llm-gateway/config.example.json" ] && [ ! -f "$gateway_config" ]; then
        if [ -f "$repo_dir/services/llm-gateway/config.json" ]; then
            cp "$repo_dir/services/llm-gateway/config.json" "$gateway_config"
        else
            cp "$tree/services/llm-gateway/config.example.json" "$gateway_config"
        fi
    fi

    # The release's own Caddy only talks to the front proxy; any certificate will do.
    if [ ! -f "$tree/certs/server.pem" ]; then
        mkdir -p "$tree/certs"
        openssl req -x509 -newkey rsa:2048 -nodes -days 825 -subj /CN=localhost \
            -addext subjectAltName=DNS:localhost \
            -keyout "$tree/certs/server-key.pem" -out "$tree/certs/server.pem" 2>/dev/null
    fi

    write_override "$1"
}

# The releases hard-code the names ai-tutor_default and ai-tutor_*; the
# override gives each release its own network, volumes and host ports.
write_override() {
    {
        printf '%s\n' "# Generated by deploy/linux-dual.sh; edits are overwritten." \
            networks: '  default:' "    name: $project" volumes:
        for volume in postgres_data task_audio_data caddy_data caddy_config redpanda_data; do
            printf '  %s:\n    name: %s_%s\n' "$volume" "$project" "$volume"
        done
        printf '%s\n' services: '  api:' '    ports: !override' "      - \"127.0.0.1:$api_port:8002\""
        if [ "$DUAL_MODE" = prod ]; then
            # Final Dockerfile stage, no source mounts: the built binary.
            printf '%s\n' '    build:' '      target: !reset null'
            if [ -d "$tree/api/fixtures" ]; then
                printf '%s\n' '    volumes: !override' '      - ./api/fixtures:/fixtures:ro'
            else
                printf '%s\n' '    volumes: !reset []'
            fi
            printf '%s\n' '    healthcheck:' '      test: ["CMD", "/api", "healthcheck"]' '      start_period: 60s'
            printf '%s\n' '  frontend:' '    build:' '      target: !reset null' '    volumes: !reset []'
        fi
        printf '%s\n' '  s3:' '    ports: !override' "      - \"127.0.0.1:$s3_port:9000\""
    } > "$tree/compose.dual.yaml"
    if ! grep -q 'redpanda:' "$tree/docker-compose.yaml"; then
        # Compose rejects names for volumes the release does not declare.
        awk '/^  redpanda_data:/ { skip = 2 } skip > 0 { skip--; next } { print }' \
            "$tree/compose.dual.yaml" > "$tree/compose.dual.yaml.tmp"
        mv -- "$tree/compose.dual.yaml.tmp" "$tree/compose.dual.yaml"
    fi
}

prepare_front_tls() {
    mkdir -p "$dual_dir/certs"
    if command -v mkcert >/dev/null 2>&1; then
        mkcert -install >/dev/null 2>&1 || true
        mkcert -cert-file "$dual_dir/certs/dual.pem" -key-file "$dual_dir/certs/dual-key.pem" \
            "$DUAL_HOST" "learn.$DUAL_HOST" "exam.$DUAL_HOST" >/dev/null 2>&1
        DUAL_TLS='/certs/dual.pem /certs/dual-key.pem'
    else
        # Caddy's own CA: the browser warns once per host.
        DUAL_TLS=internal
    fi
    export DUAL_TLS
}

check_dns() {
    for host in "$DUAL_HOST" "learn.$DUAL_HOST" "exam.$DUAL_HOST"; do
        if ! getent hosts "$host" >/dev/null 2>&1; then
            printf '%s\n' "Warning: $host does not resolve (no internet or DNS rebinding protection)." \
                "Add it to /etc/hosts with the machine's address, for example:" \
                "  127.0.0.1 $DUAL_HOST learn.$DUAL_HOST exam.$DUAL_HOST" >&2
            return
        fi
    done
}

open_browser() {
    [ "${DUAL_OPEN_BROWSER:-true}" != false ] || return 0
    if command -v xdg-open >/dev/null 2>&1; then
        opener=xdg-open
    elif command -v open >/dev/null 2>&1; then
        opener=open
    else
        return 0
    fi
    "$opener" "https://learn.$origin_host/admin/competency-map" >/dev/null 2>&1 || true
    "$opener" "https://exam.$origin_host/admin" >/dev/null 2>&1 || true
    "$opener" "$DUAL_ORIGIN" >/dev/null 2>&1 || true
}

case "$command" in
up)
    command -v docker >/dev/null 2>&1 || { printf '%s\n' 'Docker with Compose is required.' >&2; exit 1; }
    for name in learn exam; do
        prepare_worktree "$name"
        release_compose "$name" config --quiet
    done
    prepare_front_tls
    front_compose config --quiet
    check_dns
    for name in learn exam; do
        printf '%s\n' "Starting $name ($ref, $DUAL_MODE); the first build takes several minutes."
        release_compose "$name" up --build --wait --detach "$@"
    done
    front_compose up --wait --detach --force-recreate
    printf '%s\n' "Start page:       $DUAL_ORIGIN" \
        "Learning:         https://learn.$origin_host  (admin: /admin/competency-map)" \
        "Exam simulation:  https://exam.$origin_host  (admin: /admin)"
    open_browser
    ;;
down)
    front_compose down
    for name in exam learn; do
        release "$name"
        if [ -f "$tree/compose.dual.yaml" ]; then
            release_compose "$name" down
        fi
    done
    ;;
status)
    front_compose ps
    for name in learn exam; do
        release "$name"
        if [ -f "$tree/compose.dual.yaml" ]; then
            release_compose "$name" ps
        fi
    done
    ;;
logs)
    case "${1:-}" in
    learn | exam) name=$1; shift; release_compose "$name" logs --tail 200 -f "$@" ;;
    front) shift; front_compose logs --tail 200 -f ;;
    *) usage ;;
    esac
    ;;
*) usage ;;
esac
