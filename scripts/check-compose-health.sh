#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_file=$(mktemp)
trap 'rm -f "$config_file"' EXIT HUP INT TERM

docker compose \
    --file "$project_dir/deployments/docker-compose.yml" \
    config \
    --format json >"$config_file"

if ! jq -e '
    .services.api.environment.HEALTHCHECK_PORT == "8080" and
    .services.gateway.environment.HEALTHCHECK_PORT == "8081"
' "$config_file" >/dev/null; then
    echo "expected explicit API and gateway health-check ports in resolved Compose config" >&2
    jq '{
        api: .services.api.environment.HEALTHCHECK_PORT,
        gateway: .services.gateway.environment.HEALTHCHECK_PORT
    }' "$config_file" >&2
    exit 1
fi
