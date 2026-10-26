#!/bin/sh
set -eu

base_url=${APP_BASE_URL:-https://localhost:8443}
case "$base_url" in https://*) ;; *) printf '%s\n' 'APP_BASE_URL must use HTTPS for secure auth cookies.' >&2; exit 2 ;; esac
curl_flags='--silent --show-error --insecure --fail'
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT INT TERM
student_cookies="$tmp_dir/student.cookies"
admin_cookies="$tmp_dir/admin.cookies"
email="integration-$(date +%s)-$$@example.invalid"
password='integration-smoke-password'

request() {
    curl $curl_flags "$@"
}

request "$base_url/api/v1/health" | jq -e '.status == "ok"' >/dev/null
request -X POST -H 'Content-Type: application/json' \
    --cookie-jar "$student_cookies" --cookie "$student_cookies" \
    --data "{\"email\":\"$email\",\"password\":\"$password\",\"display_name\":\"Integration smoke\"}" \
    "$base_url/api/v1/auth/register" | jq -e '.user.role == "student"' >/dev/null

request --cookie "$student_cookies" "$base_url/api/v1/learn/subjects" | jq -e 'type == "array"' >/dev/null
request --cookie "$student_cookies" "$base_url/api/v1/exam/subjects" | jq -e 'type == "array"' >/dev/null
request --cookie "$student_cookies" "$base_url/api/v1/auth/me" | jq -e '.role == "student"' >/dev/null

request -X POST -H 'X-AI-Tutor-Portal: admin' \
    --cookie-jar "$admin_cookies" --cookie "$admin_cookies" \
    "$base_url/api/v1/auth/teacher" | jq -e '.user.role == "admin"' >/dev/null
request -H 'X-AI-Tutor-Portal: admin' --cookie "$admin_cookies" \
    "$base_url/api/v1/learn/subjects" | jq -e 'type == "array"' >/dev/null
request -H 'X-AI-Tutor-Portal: admin' --cookie "$admin_cookies" \
    "$base_url/api/v1/exam/subjects" | jq -e 'type == "array"' >/dev/null

request -X POST --cookie "$student_cookies" "$base_url/api/v1/auth/logout" >/dev/null
if curl --silent --insecure -o /dev/null -w '%{http_code}' --cookie "$student_cookies" \
    "$base_url/api/v1/auth/me" | grep -q '^200$'; then
    printf '%s\n' 'student logout left the session active' >&2
    exit 1
fi
request -H 'X-AI-Tutor-Portal: admin' --cookie "$admin_cookies" \
    "$base_url/api/v1/auth/me" | jq -e '.role == "admin"' >/dev/null

printf '%s\n' 'Unified stack smoke passed: shared API, separate mode catalogs, and isolated student/admin sessions.'
