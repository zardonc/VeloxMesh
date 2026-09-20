#!/usr/bin/env sh
set -eu

if [ -d deploy/compose ]; then
  env_file="${VELOXMESH_ENV_FILE:-deploy/env/simple.env}"
  dataset="deploy/testdata/simple-smoke.jsonl"
  runner="deploy/scripts/run-gateway-dataset.py"
  report_base="deploy/reports"
  sh deploy/scripts/veloxmesh-up.sh simple
elif [ -d compose ]; then
  env_file="${VELOXMESH_ENV_FILE:-env/veloxmesh.env}"
  dataset="testdata/simple-smoke.jsonl"
  runner="scripts/run-gateway-dataset.py"
  report_base="reports"
  project="$(sed -n 's/^VELOXMESH_PROJECT_NAME=//p' "$env_file" | tail -n 1)"
  docker compose -p "${project:-veloxmesh}" --env-file "$env_file" -f compose/veloxmesh.yml --profile simple up -d --build
else
  echo "Run this script from the VeloxMesh repository root or installed VeloxMesh directory." >&2
  exit 1
fi

run_id="simple-smoke-$(date +%Y%m%d-%H%M%S)"
python_bin="${PYTHON_BIN:-python3}"

"$python_bin" "$runner" \
  --env-file "$env_file" \
  --dataset "$dataset" \
  --report-dir "$report_base/$run_id" \
  --concurrency 1
