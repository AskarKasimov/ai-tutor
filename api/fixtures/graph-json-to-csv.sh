#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Использование:
  api/fixtures/graph-json-to-csv.sh INPUT.json

CSV создаётся рядом с JSON, с тем же именем и расширением .csv.
Нужна утилита jq.
EOF
}

if [[ $# -ne 1 ]]; then
  usage
  exit 2
fi

input=$1
input_name=$(basename -- "$input")
output="$(dirname -- "$input")/${input_name%.*}.csv"

if ! command -v jq >/dev/null 2>&1; then
  echo "Ошибка: установите jq." >&2
  exit 1
fi

if [[ ! -f "$input" ]]; then
  echo "Ошибка: файл не найден: $input" >&2
  exit 1
fi

if ! jq -e '(.nodes | type) == "array"' "$input" >/dev/null; then
  echo "Ошибка: ожидается JSON-объект с массивом nodes." >&2
  exit 1
fi

write_csv() {
  jq -r '
    .nodes as $nodes
    | ($nodes | map({key: .id, value: .title}) | from_entries) as $titles
    | ([
        "Компетенция / тема",
        "Материал курса",
        "Образовательный результат",
        "Вопрос / критерий оценки",
        "Предварительные темы"
      ] | @csv),
      (
        $nodes[] as $node
        | ($node.questions // [] | join("\n\n")) as $questions
        | (
            $questions
            + (if ($node.assessment_guidance // "") == "" then ""
               else (if $questions == "" then "" else "\n\n" end)
                    + "Критерий оценки: " + $node.assessment_guidance
               end)
          ) as $question_and_guidance
        | ([
            ($node.prerequisites // [])[]
            | $titles[.] // .
          ] | if length == 0 then "Нет" else join("\n") end) as $prerequisites
        | (($node.outcomes // []) | if length == 0 then [""] else . end)[] as $outcome
        | [
            ($node.title // ""),
            ($node.content // ""),
            (if $outcome == "" then "" else "• " + $outcome end),
            $question_and_guidance,
            $prerequisites
          ]
        | @csv
      )
  ' "$input"
}

tmp=$(mktemp "${output}.tmp.XXXXXX")
trap 'rm -f -- "$tmp"' EXIT

printf '\xEF\xBB\xBF' > "$tmp"
write_csv >> "$tmp"
mv -- "$tmp" "$output"
trap - EXIT

printf 'Создано: %s\n' "$output"
