#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
config_file=$(mktemp)
empty_env_file=$(mktemp)
trap 'rm -f "$config_file" "$empty_env_file"' EXIT HUP INT TERM

if env -i PATH="$PATH" COMPOSE_DISABLE_ENV_FILE=1 docker compose \
    --env-file "$empty_env_file" \
    --file "$project_dir/deployments/docker-compose.prod.yml" \
    config --quiet >/dev/null 2>&1; then
    echo "production Compose must reject missing required variables" >&2
    exit 1
fi

export POSTGRES_USER=${POSTGRES_USER:-loomfeed}
export POSTGRES_PASSWORD=${POSTGRES_PASSWORD:-prod_config_password}
export POSTGRES_DB=${POSTGRES_DB:-loomfeed}
export REDIS_PASSWORD=${REDIS_PASSWORD:-prod_config_redis_password}
export JWT_SECRET=${JWT_SECRET:-prod_config_jwt_secret}
export ALLOWED_ORIGINS=${ALLOWED_ORIGINS:-http://localhost:13000}
export SITE_URL=${SITE_URL:-http://localhost:13000}
export API_PORT=${API_PORT:-18080}
export WEB_PORT=${WEB_PORT:-13000}
export UPLOADS_ENABLED=${UPLOADS_ENABLED:-true}
unset API_BIND_ADDRESS WEB_BIND_ADDRESS

docker compose \
    --file "$project_dir/deployments/docker-compose.prod.yml" \
    config \
    --format json >"$config_file"

if ! jq -e \
    --arg api_port "$API_PORT" \
    --arg web_port "$WEB_PORT" '
    def publishes($service; $target; $published):
        any($service.ports[]?; .target == $target and .published == $published);
    def mounts($service; $target; $source):
        any($service.volumes[]?; .target == $target and .source == $source);

    .services.migrate != null and
    .services.bootstrap != null and
    .services.bootstrap.build.args.SERVICE == "bootstrap" and
    .services.bootstrap.environment.DATABASE_URL != null and
    .services.bootstrap.environment.REDIS_URL != null and
    .services.bootstrap.restart == "no" and
    publishes(.services.api; 8080; $api_port) and
    publishes(.services.web; 3000; $web_port) and
    any(.services.api.ports[]?; .target == 8080 and .host_ip == "127.0.0.1") and
    any(.services.web.ports[]?; .target == 3000 and .host_ip == "127.0.0.1") and
    .services.web.build.args.API_URL == "http://api:8080" and
    .services.web.build.args.SITE_URL == .services.web.environment.SITE_URL and
    .services.web.environment.API_URL == "http://api:8080" and
    .services.api.environment.UPLOADS_ENABLED == "true" and
    .services.postgres.healthcheck != null and
    .services.redis.healthcheck != null and
    (.services.api.healthcheck.test | join(" ") | contains("/readyz")) and
    .services.migrate.depends_on.postgres.condition == "service_healthy" and
    .services.bootstrap.depends_on.postgres.condition == "service_healthy" and
    .services.bootstrap.depends_on.redis.condition == "service_healthy" and
    .services.bootstrap.depends_on.migrate.condition == "service_completed_successfully" and
    .services.api.depends_on.postgres.condition == "service_healthy" and
    .services.api.depends_on.redis.condition == "service_healthy" and
    .services.api.depends_on.migrate.condition == "service_completed_successfully" and
    .services.api.depends_on.bootstrap.condition == "service_completed_successfully" and
    .services.web.depends_on.api.condition == "service_healthy" and
    .services.web.healthcheck != null and
    mounts(.services.api; "/app/uploads"; "uploads") and
    any(.services.migrate.volumes[]?; .target == "/migrations" and .read_only == true) and
    .volumes.uploads != null
' "$config_file" >/dev/null; then
    echo "production Compose config is missing a required runtime guarantee" >&2
    jq '{
        services: (.services | with_entries(.value |= {
            ports,
            build: {args: {API_URL: .build.args.API_URL, SITE_URL: .build.args.SITE_URL}},
            environment: {
                API_URL: .environment.API_URL,
                SITE_URL: .environment.SITE_URL,
                UPLOADS_ENABLED: .environment.UPLOADS_ENABLED
            },
            healthcheck,
            depends_on,
            volumes
        })),
        volumes
    }' "$config_file" >&2
    exit 1
fi
