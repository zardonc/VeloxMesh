#!/usr/bin/env sh
set -eu

exec docker compose -f deploy/compose/veloxmesh.yml down "$@"
