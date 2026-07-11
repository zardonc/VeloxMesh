#!/usr/bin/env sh
set -eu

env_file="${VELOXMESH_ENV_FILE:-deploy/env/simple.env}"

if [ ! -f "$env_file" ]; then
  env_file="deploy/env/full.env"
fi

if [ -f "$env_file" ]; then
  exec docker compose --env-file "$env_file" -f deploy/compose/veloxmesh.yml down "$@"
fi

exec docker compose -f deploy/compose/veloxmesh.yml down "$@"
