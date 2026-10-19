#!/bin/sh
# Deletes the data of the two-mode stand: stops it and removes the volumes of
# a release (database, S3, Caddy state, Redpanda, course-graph). The models
# course-graph downloaded and the certificates of the start page stay unless
# asked. The main stand (ai-tutor_*) is not touched. On Windows use
# windows-dual-reset.ps1. See dual/README.md.
set -eu

deploy_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

usage() {
    cat >&2 <<'EOF'
Usage: deploy/linux-dual-reset.sh [learn|exam|all] [--yes]

  learn|exam|all  whose data to delete [all]
  --yes           do not ask for confirmation

Environment:
  DUAL_RESET_MODELS=true  also delete the models course-graph downloaded
  DUAL_RESET_TLS=true     also delete the certificates of the start page
The stand stays stopped; start it again with deploy/linux-dual.sh up.
EOF
    exit 2
}

target=all
yes=false
for arg in "$@"; do
    case "$arg" in
    learn | exam | all) target=$arg ;;
    --yes | -y) yes=true ;;
    *) usage ;;
    esac
done

command -v docker >/dev/null 2>&1 || { printf '%s\n' 'Docker is required.' >&2; exit 1; }

case "$target" in
all) projects='tutor-learn tutor-exam' ;;
*) projects="tutor-$target" ;;
esac
[ "${DUAL_RESET_TLS:-false}" = true ] && projects="$projects tutor-dual"

# The volumes to delete: every volume of the chosen projects, but the models
# unless asked.
volumes=
for project in $projects; do
    for volume in $(docker volume ls -q); do
        case "$volume" in
        "${project}"_course_graph_models) [ "${DUAL_RESET_MODELS:-false}" = true ] || continue ;;
        "${project}"_*) ;;
        *) continue ;;
        esac
        volumes="$volumes $volume"
    done
done

if [ -z "$volumes" ]; then
    printf '%s\n' "No data of $target to delete."
    exit 0
fi

printf '%s\n' 'These volumes will be deleted for good:' $volumes
if [ "$yes" != true ]; then
    printf '%s' 'Type yes to delete: '
    read -r answer
    [ "$answer" = yes ] || { printf '%s\n' 'Nothing deleted.'; exit 1; }
fi

# Containers hold their volumes; stop the whole stand first.
"$deploy_dir/linux-dual.sh" down
# shellcheck disable=SC2086 # the list is split on purpose
docker volume rm $volumes
printf '%s\n' 'Deleted. Start the stand again with: deploy/linux-dual.sh up'
