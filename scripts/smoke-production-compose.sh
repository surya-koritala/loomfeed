#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
compose_file="$project_dir/deployments/docker-compose.prod.yml"
project_name=loomfeed-prod-smoke-$(date +%s)-$$

export POSTGRES_USER=${POSTGRES_USER:-loomfeed}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-prod_smoke_password}
export POSTGRES_DB=${POSTGRES_DB:-loomfeed}
export REDIS_PASSWORD=${REDIS_PASSWORD:-prod_smoke_redis_password}
export JWT_SECRET=${JWT_SECRET:-prod_smoke_jwt_secret}
export API_BIND_ADDRESS=${API_BIND_ADDRESS:-127.0.0.1}
export API_PORT=${API_PORT:-18080}
export WEB_BIND_ADDRESS=${WEB_BIND_ADDRESS:-127.0.0.1}
export WEB_PORT=${WEB_PORT:-13000}
export ALLOWED_ORIGINS=${ALLOWED_ORIGINS:-http://127.0.0.1:$WEB_PORT}
export SITE_URL=${SITE_URL:-http://127.0.0.1:$WEB_PORT}
export UPLOADS_ENABLED=${UPLOADS_ENABLED:-true}
export FEDERATION_ENABLED=true

compose() {
    docker compose --project-name "$project_name" --file "$compose_file" "$@"
}

cleanup() {
    exit_code=$?
    trap - EXIT HUP INT TERM
    if [ "$exit_code" -ne 0 ]; then
        compose ps --all >&2 || true
        compose logs --no-color >&2 || true
    fi
    compose down --volumes --remove-orphans --rmi local >/dev/null 2>&1 || true
    exit "$exit_code"
}
trap cleanup EXIT HUP INT TERM

wait_for_url() {
    name=$1
    url=$2
    attempts=${3:-60}
    count=1
    while [ "$count" -le "$attempts" ]; do
        if curl --fail --silent --location --output /dev/null "$url"; then
            return 0
        fi
        sleep 2
        count=$((count + 1))
    done
    echo "timed out waiting for $name at $url" >&2
    return 1
}

compose config --quiet
compose up --build --detach

wait_for_url "API readiness" "http://127.0.0.1:$API_PORT/readyz"
wait_for_url "web frontend" "http://127.0.0.1:$WEB_PORT/"
wait_for_url "web-to-API rewrite" "http://127.0.0.1:$WEB_PORT/api/v1/config"

curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data '{"email":"compose-federation@example.com","password":"compose-smoke-password","display_name":"Compose Federation"}' \
    "http://127.0.0.1:$WEB_PORT/api/v1/auth/register" >/dev/null
curl --fail --silent --show-error \
    "http://127.0.0.1:$WEB_PORT/.well-known/webfinger?resource=acct:composefederation@127.0.0.1" |
    jq -e '.subject | startswith("acct:composefederation@")' >/dev/null
curl --fail --silent --show-error \
    --header 'Accept: application/activity+json' \
    "http://127.0.0.1:$WEB_PORT/users/composefederation" |
    jq -e '.preferredUsername == "composefederation" and (.inbox | endswith("/users/composefederation/inbox"))' >/dev/null
inbox_status=$(curl --silent --output /dev/null --write-out '%{http_code}' \
    --request POST \
    --header 'Content-Type: application/activity+json' \
    --data '{}' \
    "http://127.0.0.1:$WEB_PORT/users/composefederation/inbox")
if [ "$inbox_status" != "401" ]; then
    echo "expected unsigned ActivityPub inbox request to reach API and return 401; got $inbox_status" >&2
    exit 1
fi

compose exec --no-TTY api sh -c 'printf smoke > /app/uploads/.compose-smoke-marker'
compose rm --stop --force api
compose up --detach api
wait_for_url "recreated API readiness" "http://127.0.0.1:$API_PORT/readyz"
compose exec --no-TTY api grep -q smoke /app/uploads/.compose-smoke-marker

echo "production Compose smoke test passed"
