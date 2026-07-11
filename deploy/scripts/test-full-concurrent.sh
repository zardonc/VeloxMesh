#!/usr/bin/env sh
set -eu

[ -d deploy/compose ] || {
  echo "Run this script from the VeloxMesh repository root." >&2
  exit 1
}

env_file="${VELOXMESH_ENV_FILE:-deploy/env/full.env}"
run_id="full-concurrent-$(date +%Y%m%d-%H%M%S)"
python_bin="${PYTHON_BIN:-python3}"

sh deploy/scripts/veloxmesh-up.sh full

"$python_bin" deploy/scripts/run-gateway-dataset.py \
  --env-file "$env_file" \
  --dataset deploy/testdata/full-concurrent.jsonl \
  --report-dir "deploy/reports/$run_id" \
  --concurrency 8
