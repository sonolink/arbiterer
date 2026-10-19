#!/bin/sh
set -eu

cd "$1"
export DOCKER_CONFIG="$PWD/.docker"
trap 'rm -rf "$DOCKER_CONFIG"' EXIT

# What is running now, to go back to if the new one never turns healthy.
previous=
if [ -f deploy.env ]; then
    previous=$(sed -n 's/^ARBITERER_IMAGE=//p' deploy.env)
fi

printf 'ARBITERER_IMAGE=%s\n' "$2" >deploy.env

compose() {
    docker compose --env-file .env --env-file deploy.env "$@"
}

compose pull --quiet
compose up --detach --wait postgres

# Don't auto-migrate in production!!!
if [ "${3:-false}" = "true" ]; then
    compose run --rm -T migrator </dev/null
fi

if ! compose up --detach --wait --remove-orphans; then
    if [ -n "$previous" ]; then
        echo "deploy failed, going back to $previous" >&2
        printf 'ARBITERER_IMAGE=%s\n' "$previous" >deploy.env
        compose up --detach --wait --remove-orphans
    fi

    exit 1
fi

docker images --quiet "${2%@*}" | sort -u | xargs -r docker rmi >/dev/null 2>&1 || true
