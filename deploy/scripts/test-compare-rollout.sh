#!/usr/bin/env sh
set -eu

[ -d deploy/compose ] || {
  echo "Run this script from the VeloxMesh repository root." >&2
  exit 1
}

env_file="${VELOXMESH_ENV_FILE:-deploy/env/compare.env}"
base_run_id="compare-rollout-$(date +%Y%m%d-%H%M%S)"
base_report_dir="deploy/reports/$base_run_id"
python_bin="${PYTHON_BIN:-python3}"
mkdir -p "$base_report_dir"

sh deploy/scripts/veloxmesh-up.sh compare

for rollout in 100 0; do
  config_file="$base_report_dir/scheduler.compare.rollout-$rollout.json"
  ROLLOUT="$rollout" CONFIG_FILE="$config_file" "$python_bin" -c '
import json, os
source = "deploy/config/scheduler.compare.json"
target = os.environ["CONFIG_FILE"]
with open(source, encoding="utf-8") as handle:
    data = json.load(handle)
data["onnx_rollout_percent"] = int(os.environ["ROLLOUT"])
with open(target, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
'

  compose_config="../reports/$base_run_id/scheduler.compare.rollout-$rollout.json"
  VELOXMESH_SCHEDULER_CONFIG="$compose_config" docker compose \
    --env-file "$env_file" \
    -f deploy/compose/veloxmesh.yml \
    --profile compare \
    up -d --build

  "$python_bin" deploy/scripts/run-gateway-dataset.py \
    --env-file "$env_file" \
    --dataset deploy/testdata/compare-rollout.jsonl \
    --report-dir "$base_report_dir/rollout-$rollout" \
    --concurrency 1
done
