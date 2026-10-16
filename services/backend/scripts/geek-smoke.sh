#!/usr/bin/env bash
# End-to-end run of a Geek diagnostic over the public API, for rehearsals.
# Creates a course from the admin panel account, registers a student and
# answers every Geek turn by the chosen profile until the result is ready.
#
#   scripts/geek-smoke.sh [base-url] [profile]
#   base-url  default https://localhost:8443/api/v1
#   profile   strong | weak | mixed (default mixed: strong, weak and "не знаю" in turn)
set -euo pipefail

base=${1:-https://localhost:8443/api/v1}
profile=${2:-mixed}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl_() { curl -sS -k --fail-with-body "$@"; }

admin=(-c "$work/admin" -b "$work/admin" -H 'X-AI-Tutor-Portal: admin' -H 'Content-Type: application/json')
curl_ "${admin[@]}" -X POST "$base/auth/teacher" >/dev/null
subject=$(curl_ "${admin[@]}" -X POST "$base/admin/subjects" -d "{\"name\":\"Демо Geek $(date +%H%M%S)\"}" | jq -r .id)
echo "Курс: $subject"

student=(-c "$work/student" -b "$work/student" -H 'Content-Type: application/json')
email="geek-$(date +%s%N)@example.edu"
curl_ "${student[@]}" -X POST "$base/auth/register" \
  -d "{\"email\":\"$email\",\"password\":\"Надёжная фраза для репетиции 42\",\"display_name\":\"Студент\"}" >/dev/null

session=$(curl_ "${student[@]}" -H "Idempotency-Key: start-$email" -X POST "$base/diagnostic-sessions" -d "{\"subject_id\":\"$subject\"}")
id=$(jq -r .id <<<"$session")
strong='Эвристика оценивает стоимость пути до цели, и если она не переоценивает расстояние, алгоритм находит оптимальное решение, а иначе может ошибиться.'
weak='Ну это поиск'
step=0
while [[ $(jq -r .status <<<"$session") == active ]]; do
  turn=$(jq -r .current_turn_id <<<"$session")
  echo "Geek [$(jq -r '.progress_percent' <<<"$session")%]: $(jq -r '.turns[-1].text' <<<"$session")"
  case "$profile:$((step % 3))" in
    strong:* | mixed:0) body=$(jq -nc --arg t "$turn" --arg a "$strong" '{turn_id: $t, text: $a}') ;;
    weak:* | mixed:1) body=$(jq -nc --arg t "$turn" --arg a "$weak" '{turn_id: $t, text: $a}') ;;
    *) body=$(jq -nc --arg t "$turn" '{turn_id: $t, dont_know: true}') ;;
  esac
  echo "Студент: $(jq -r '.text // "(не знаю)"' <<<"$body")"
  session=$(curl_ "${student[@]}" -H "Idempotency-Key: answer-$turn" -X POST "$base/diagnostic-sessions/$id/answers" -d "$body")
  step=$((step + 1))
done
echo "Geek: $(jq -r '.turns[-1].text' <<<"$session")"
curl_ "${student[@]}" "$base/diagnostic-sessions/$id/result" |
  jq -r '"Оценка: \(.grade // "нет") (\(.score_percent // "-")%)", "Пробелы:", (.gaps[] | "  - \(.title)"), "Темы:", (.nodes[] | "  \(.status)\t\(.grade // "-")\t\(.title)")'
