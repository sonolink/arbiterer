#!/bin/sh
set -eu

cd "$1"
export DOCKER_CONFIG="$PWD/.docker"
trap 'rm -rf "$DOCKER_CONFIG"' EXIT

printf 'ARBITERER_IMAGE=%s\n' "$2" > release.env

compose() {
    docker compose --env-file .env --env-file release.env "$@"
}

compose pull --quiet
compose up --detach --wait postgres
compose run --rm migrator
compose up --detach --wait --remove-orphans

docker images --quiet "${2%@*}" | sort -u | xargs -r docker rmi >/dev/null 2>&1 || true
