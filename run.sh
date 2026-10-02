#!/bin/sh
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_dir"

for dependency in mkcert docker; do
    if ! command -v "$dependency" >/dev/null 2>&1; then
        printf '%s\n' "$dependency is required. See README.md for installation instructions." >&2
        exit 1
    fi
done
if [ ! -f .env ]; then
    printf '%s\n' 'Create .env from .env.example and set DB_PASSWORD, BACKEND_STT_URL and BACKEND_TTS_URL.' >&2
    exit 1
fi
docker compose version >/dev/null
docker compose config --quiet

mkcert -install
mkdir -p certs
mkcert -cert-file certs/server.pem -key-file certs/server-key.pem localhost 127.0.0.1 ::1

exec docker compose up --build "$@"
