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

system_identity_state=$(compose exec --no-TTY postgres psql \
    --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --tuples-only \
    --no-align --field-separator '|' --command \
    "SELECT p.type::text,
            (SELECT COUNT(*) FROM human_users WHERE participant_id = p.id),
            (SELECT COUNT(*) FROM agent_identities WHERE participant_id = p.id),
            (SELECT COUNT(*) FROM api_keys WHERE agent_id = p.id),
            (SELECT COUNT(*) FROM refresh_tokens WHERE participant_id = p.id)
     FROM participants p
     WHERE p.id = 'a1110000-0000-4000-8000-000000000001'::uuid")
if [ "$system_identity_state" != "system|0|0|0|0" ]; then
    echo "bootstrap owner is not a credential-free system identity: $system_identity_state" >&2
    exit 1
fi

# Prove the bootstrap does not rely on the privileged Compose database owner.
# A forced-RLS credential collision must be visible to a restricted role and
# fail closed; after cleanup, the same role must complete an idempotent rerun.
compose exec --no-TTY postgres psql \
    --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set ON_ERROR_STOP=1 \
    --command "CREATE ROLE bootstrap_smoke LOGIN PASSWORD 'bootstrap_smoke_password'" \
    --command "GRANT CONNECT ON DATABASE \"$POSTGRES_DB\" TO bootstrap_smoke" \
    --command "GRANT app_service TO bootstrap_smoke" \
    --command "INSERT INTO refresh_tokens (participant_id, token_hash, expires_at)
               VALUES ('a1110000-0000-4000-8000-000000000001'::uuid,
                       'bootstrap-smoke-collision', NOW() + INTERVAL '1 hour')"
restricted_database_url="postgres://bootstrap_smoke:bootstrap_smoke_password@postgres:5432/$POSTGRES_DB?sslmode=disable"
visible_credentials=$(compose exec --no-TTY \
    --env PGPASSWORD=bootstrap_smoke_password postgres psql \
    --username bootstrap_smoke --dbname "$POSTGRES_DB" --tuples-only --no-align \
    --command "SELECT COUNT(*) FROM refresh_tokens")
if [ "$visible_credentials" != "0" ]; then
    echo "app_service membership exposed credentials before SET ROLE" >&2
    exit 1
fi
if compose run --rm --no-deps --env DATABASE_URL="$restricted_database_url" bootstrap >/dev/null 2>&1; then
    echo "restricted-role bootstrap accepted a credential-bearing system identity" >&2
    exit 1
fi
compose exec --no-TTY postgres psql \
    --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set ON_ERROR_STOP=1 \
    --command "DELETE FROM refresh_tokens WHERE token_hash = 'bootstrap-smoke-collision'" >/dev/null
restricted_rerun=$(compose run --rm --no-deps \
    --env DATABASE_URL="$restricted_database_url" bootstrap)
printf '%s' "$restricted_rerun" |
    jq -e '.communities_created == 0 and .ownership_transferred == 0' >/dev/null

catalog=$(curl --fail --silent --show-error \
    "http://127.0.0.1:$WEB_PORT/api/v1/communities?limit=500")
community_id=$(printf '%s' "$catalog" | jq -er \
    'if length > 0 then .[0].id else error("fresh install returned an empty community catalog") end')
catalog_fingerprint=$(printf '%s' "$catalog" | jq -S -r 'sort_by(.slug) | map([.id, .slug])')

# The supported bootstrap command must be safe for operators to rerun. The
# public catalog is the seam: successful no-op reruns preserve every seed ID
# and slug instead of duplicating or replacing communities.
bootstrap_rerun_one=$(compose run --rm --no-deps bootstrap)
printf '%s' "$bootstrap_rerun_one" |
    jq -e '.communities_created == 0 and .ownership_transferred == 0' >/dev/null
bootstrap_rerun_two=$(compose run --rm --no-deps bootstrap)
printf '%s' "$bootstrap_rerun_two" |
    jq -e '.communities_created == 0 and .ownership_transferred == 0' >/dev/null
catalog_after_rerun=$(curl --fail --silent --show-error \
    "http://127.0.0.1:$WEB_PORT/api/v1/communities?limit=500&sort=alphabetical")
catalog_fingerprint_after_rerun=$(printf '%s' "$catalog_after_rerun" | jq -S -r 'sort_by(.slug) | map([.id, .slug])')
if [ "$catalog_fingerprint" != "$catalog_fingerprint_after_rerun" ]; then
    echo "repeated bootstrap changed the public community catalog" >&2
    exit 1
fi

registration=$(curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --data '{"email":"compose-federation@example.com","password":"compose-smoke-password","display_name":"Compose Federation"}' \
    "http://127.0.0.1:$WEB_PORT/api/v1/auth/register")
access_token=$(printf '%s' "$registration" | jq -er '.access_token')
participant_id=$(printf '%s' "$registration" | jq -er '.participant.id')

post_body=$(jq -n --arg community_id "$community_id" '{
    community_id: $community_id,
    title: "First post on a fresh Loomfeed install",
    body: "This post verifies that a newly registered human can participate in a bootstrapped community.",
    post_type: "text"
}')
curl --fail --silent --show-error \
    --header 'Content-Type: application/json' \
    --header "Authorization: Bearer $access_token" \
    --data "$post_body" \
    "http://127.0.0.1:$WEB_PORT/api/v1/posts" |
    jq -e --arg community_id "$community_id" '.community_id == $community_id and (.id | length > 0)' >/dev/null

ownership_result=$(compose run --rm --no-deps \
    --env DATABASE_URL="$restricted_database_url" bootstrap \
    --owner-email compose-federation@example.com)
printf '%s' "$ownership_result" | jq -e '.ownership_transferred > 0' >/dev/null
catalog_after_transfer=$(curl --fail --silent --show-error \
    "http://127.0.0.1:$WEB_PORT/api/v1/communities?limit=500")
printf '%s' "$catalog_after_transfer" | jq -e \
    --arg owner_id "$participant_id" \
    --arg system_id 'a1110000-0000-4000-8000-000000000001' \
    'length > 0 and
     any(.[]; .created_by == $owner_id) and
     all(.[]; .created_by != $system_id)' >/dev/null
ownership_rerun=$(compose run --rm --no-deps \
    --env DATABASE_URL="$restricted_database_url" bootstrap \
    --owner-email compose-federation@example.com)
printf '%s' "$ownership_rerun" | jq -e '.ownership_transferred == 0' >/dev/null

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
