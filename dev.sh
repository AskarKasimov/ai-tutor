#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

if [ ! -f .env ]; then
    printf '%s\n' '.env is missing. Copy and configure it: cp .env.example .env' >&2
    exit 1
fi

docker compose version >/dev/null
docker compose --env-file .env -f docker-compose.yaml config --quiet

mkcert -install
mkdir -p certs
mkcert -cert-file certs/server.pem -key-file certs/server-key.pem localhost 127.0.0.1 ::1

# The root Compose creates its managed dev network before starting containers.
exec docker compose --env-file .env -f docker-compose.yaml up --build "$@"
