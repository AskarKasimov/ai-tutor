#!/bin/sh
# Exercise startup decisions without starting containers or changing host services.
set -eu
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -r "$test_dir"' 0
mkdir -p "$test_dir/bin" "$test_dir/services/llm-gateway"
cp "$repo_dir/dev.sh" "$test_dir/dev.sh"
printf '%s\n' 'COMPOSE_PROFILES=swagger' 'DB_PORT=5432' > "$test_dir/.env"
printf '{}\n' > "$test_dir/services/llm-gateway/config.json"
cat > "$test_dir/bin/docker" <<'MOCK'
#!/bin/sh
printf '%s\n' "$*" >> "$STARTUP_TEST_LOG"
case "$*" in
  *'config --environment'*) printf 'DB_PORT=5432\n' ;;
  *'ps -q db'*) if [ "$STARTUP_TEST_DB_RUNNING" = true ]; then printf 'project-db\n'; fi ;;
esac
exit 0
MOCK
cat > "$test_dir/bin/ss" <<'MOCK'
#!/bin/sh
if [ "$STARTUP_TEST_PORT_BUSY" = true ]; then printf 'LISTEN 0 200 127.0.0.1:5432\n'; fi
exit 0
MOCK
cat > "$test_dir/bin/mkcert" <<'MOCK'
#!/bin/sh
printf 'mkcert %s\n' "$*" >> "$STARTUP_TEST_LOG"
MOCK
chmod +x "$test_dir/bin/docker" "$test_dir/bin/ss" "$test_dir/bin/mkcert"
export PATH="$test_dir/bin:$PATH" DEV_OPEN_BROWSER=false
export STARTUP_TEST_LOG="$test_dir/commands.log"
export STARTUP_TEST_DB_RUNNING=false STARTUP_TEST_PORT_BUSY=false
sh "$test_dir/dev.sh" -d > "$test_dir/output" 2>&1
grep -q -- '--profile gateway up --build -d' "$STARTUP_TEST_LOG"
: > "$STARTUP_TEST_LOG"
export STARTUP_TEST_PORT_BUSY=true
if sh "$test_dir/dev.sh" -d > "$test_dir/output" 2>&1; then
    printf '%s\n' 'Expected startup to reject an occupied database port.' >&2
    exit 1
fi
grep -q 'Host port 5432 is already in use' "$test_dir/output"
if grep -q -e mkcert -e 'up --build' "$STARTUP_TEST_LOG"; then
    printf '%s\n' 'Port conflict must be detected before certificates and containers.' >&2
    exit 1
fi
export STARTUP_TEST_DB_RUNNING=true
sh "$test_dir/dev.sh" -d > "$test_dir/output" 2>&1
grep -q -- '--profile gateway up --build -d' "$STARTUP_TEST_LOG"
printf '%s\n' 'Dev startup checks passed: old .env, occupied port, existing project database.'
