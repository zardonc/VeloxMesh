#!/usr/bin/env sh
set -eu
if (set -o pipefail) 2>/dev/null; then
  set -o pipefail
fi

env_args=""
project_name="${VELOXMESH_PROJECT_NAME:-veloxmesh}"

if [ -n "${VELOXMESH_ENV_FILE:-}" ]; then
  env_args="--env-file $VELOXMESH_ENV_FILE"
else
  for env_file in deploy/env/simple.env deploy/env/full.env deploy/env/compare.env deploy/env/postgres.env; do
    if [ -f "$env_file" ]; then
      env_args="$env_args --env-file $env_file"
    fi
  done
fi

# shellcheck disable=SC2086
if [ -n "$env_args" ]; then
  exec docker compose -p "$project_name" $env_args -f deploy/compose/veloxmesh.yml down --remove-orphans "$@"
fi

exec docker compose -p "$project_name" -f deploy/compose/veloxmesh.yml down --remove-orphans "$@"
