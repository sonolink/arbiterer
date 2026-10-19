#!/bin/sh
set -eu

cd "$1"
export DOCKER_CONFIG="$PWD/.docker"
trap 'rm -rf "$DOCKER_CONFIG" migrate.env' EXIT

# Run the migrations of the given image, or of the one already deployed when no
# image is given.
if [ -n "${2:-}" ]; then
    printf 'ARBITERER_IMAGE=%s\n' "$2" >migrate.env
    env_file=migrate.env
else
    env_file=deploy.env
fi

compose() {
    docker compose --env-file .env --env-file "$env_file" "$@"
}

compose up --detach --wait postgres
compose run --rm -T migrator </dev/null
