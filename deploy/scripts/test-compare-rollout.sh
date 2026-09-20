#!/usr/bin/env sh
set -eu

if [ -d deploy/compose ]; then
  env_file="${VELOXMESH_ENV_FILE:-deploy/env/compare.env}"
  dataset="deploy/testdata/compare-rollout.jsonl"
  runner="deploy/scripts/run-gateway-dataset.py"
  scheduler_config="deploy/config/scheduler.compare.json"
  compose_file="deploy/compose/veloxmesh.yml"
  report_prefix="deploy/reports"
  compose_report_prefix="../reports"
  sh deploy/scripts/veloxmesh-up.sh compare
elif [ -d compose ]; then
  env_file="${VELOXMESH_ENV_FILE:-env/veloxmesh.env}"
  dataset="testdata/compare-rollout.jsonl"
  runner="scripts/run-gateway-dataset.py"
  scheduler_config="config/scheduler.compare.json"
  compose_file="compose/veloxmesh.yml"
  report_prefix="reports"
  compose_report_prefix="../reports"
  project="$(sed -n 's/^VELOXMESH_PROJECT_NAME=//p' "$env_file" | tail -n 1)"
  docker compose -p "${project:-veloxmesh}" --env-file "$env_file" -f "$compose_file" --profile compare up -d --build
else
  echo "Run this script from the VeloxMesh repository root or installed VeloxMesh directory." >&2
  exit 1
fi

base_run_id="compare-rollout-$(date +%Y%m%d-%H%M%S)"
base_report_dir="$report_prefix/$base_run_id"
python_bin="${PYTHON_BIN:-python3}"
mkdir -p "$base_report_dir"
failed=0

for rollout in 100 0 50; do
  config_file="$base_report_dir/scheduler.compare.rollout-$rollout.json"
  ROLLOUT="$rollout" CONFIG_FILE="$config_file" SCHEDULER_SOURCE="$scheduler_config" "$python_bin" -c '
import json, os
source = os.environ["SCHEDULER_SOURCE"]
target = os.environ["CONFIG_FILE"]
with open(source, encoding="utf-8") as handle:
    data = json.load(handle)
data["onnx_rollout_percent"] = int(os.environ["ROLLOUT"])
with open(target, "w", encoding="utf-8") as handle:
    json.dump(data, handle, indent=2)
    handle.write("\n")
'

  compose_config="$compose_report_prefix/$base_run_id/scheduler.compare.rollout-$rollout.json"
  project="$(sed -n 's/^VELOXMESH_PROJECT_NAME=//p' "$env_file" | tail -n 1)"
  VELOXMESH_SCHEDULER_CONFIG="$compose_config" docker compose -p "${project:-veloxmesh}" \
    --env-file "$env_file" \
    -f "$compose_file" \
    --profile compare \
    up -d --build

  if ! "$python_bin" "$runner" \
    --env-file "$env_file" \
    --dataset "$dataset" \
    --report-dir "$base_report_dir/rollout-$rollout" \
    --concurrency 1; then
    failed=1
  fi
done

exit "$failed"
