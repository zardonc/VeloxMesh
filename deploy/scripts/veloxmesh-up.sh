#!/usr/bin/env sh
set -eu

[ -d deploy/compose ] || {
  echo "Run this script from the VeloxMesh repository root." >&2
  exit 1
}

mode="${1:-simple}"
extra_env=""
created=0

copy_if_missing() {
  src="$1"
  dst="$2"
  if [ ! -f "$dst" ]; then
    cp "$src" "$dst"
    created=1
    echo "  Created $dst from example; edit before first real run" >&2
  fi
}

case "$mode" in
  simple)
    env_file="deploy/env/simple.env"
    profiles="--profile simple"
    copy_if_missing deploy/env/simple.example.env "$env_file"
    copy_if_missing deploy/config/app.simple.example.json deploy/config/app.simple.json
    copy_if_missing deploy/config/scheduler.simple.example.json deploy/config/scheduler.simple.json
    copy_if_missing deploy/config/cache.simple.example.json deploy/config/cache.simple.json
    ;;
  full)
    env_file="deploy/env/full.env"
    profiles="--profile full"
    copy_if_missing deploy/env/full.example.env "$env_file"
    copy_if_missing deploy/config/app.full.example.json deploy/config/app.full.json
    copy_if_missing deploy/config/scheduler.full.example.json deploy/config/scheduler.full.json
    copy_if_missing deploy/config/cache.full.example.json deploy/config/cache.full.json
    ;;
  compare)
    env_file="deploy/env/compare.env"
    profiles="--profile compare"
    copy_if_missing deploy/env/compare.example.env "$env_file"
    copy_if_missing deploy/config/app.compare.example.json deploy/config/app.compare.json
    copy_if_missing deploy/config/scheduler.compare.example.json deploy/config/scheduler.compare.json
    copy_if_missing deploy/config/cache.compare.example.json deploy/config/cache.compare.json
    ;;
  postgres)
    env_file="deploy/env/full.env"
    extra_env="--env-file deploy/env/postgres.env"
    profiles="--profile full --profile postgres"
    copy_if_missing deploy/env/full.example.env deploy/env/full.env
    copy_if_missing deploy/env/postgres.example.env deploy/env/postgres.env
    copy_if_missing deploy/config/app.full.example.json deploy/config/app.full.json
    copy_if_missing deploy/config/scheduler.full.example.json deploy/config/scheduler.full.json
    copy_if_missing deploy/config/cache.full.example.json deploy/config/cache.full.json
    ;;
  *)
    echo "usage: deploy/scripts/veloxmesh-up.sh [simple|full|compare|postgres]" >&2
    exit 2
    ;;
esac

if [ "$created" = "1" ]; then
  echo "Config created for '$mode'. Edit generated files under deploy/env and deploy/config, then re-run this command." >&2
  exit 0
fi

echo "Config ready for '$mode'. Starting Docker Compose."

# shellcheck disable=SC2086
exec docker compose --env-file "$env_file" ${extra_env} -f deploy/compose/veloxmesh.yml $profiles up -d --build
