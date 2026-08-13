#!/bin/sh
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
fixture_dir=$(mktemp -d)
trap 'rm -rf "$fixture_dir"' EXIT HUP INT TERM

root_context="$fixture_dir/root"
mkdir -p \
    "$root_context/.git" \
    "$root_context/.github" \
    "$root_context/web/node_modules" \
    "$root_context/web/.next" \
    "$root_context/bin" \
    "$root_context/coverage" \
    "$root_context/internal" \
    "$root_context/uploads"
touch \
    "$root_context/.env" \
    "$root_context/debug.log" \
    "$root_context/internal/source 2.go" \
    "$root_context/kept.txt"
cp "$project_dir/.dockerignore" "$root_context/.dockerignore"

docker buildx build \
    --build-arg CONTEXT_KIND=root \
    --file "$project_dir/deployments/docker/context-check.Dockerfile" \
    --output type=cacheonly \
    --progress=plain \
    "$root_context"

web_context="$fixture_dir/web"
mkdir -p \
    "$web_context/node_modules" \
    "$web_context/.next" \
    "$web_context/coverage" \
    "$web_context/public" \
    "$web_context/src"
touch \
    "$web_context/.env.local" \
    "$web_context/npm-debug.log" \
    "$web_context/public/icon 2.png" \
    "$web_context/src/component 2.tsx" \
    "$web_context/tsconfig.tsbuildinfo" \
    "$web_context/src/app.ts" \
    "$web_context/kept.txt"
cp "$project_dir/web/.dockerignore" "$web_context/.dockerignore"

docker buildx build \
    --build-arg CONTEXT_KIND=web \
    --file "$project_dir/deployments/docker/context-check.Dockerfile" \
    --output type=cacheonly \
    --progress=plain \
    "$web_context"
