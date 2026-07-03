#!/usr/bin/env sh
set -eu

need() {
  eval "value=\${$1:-}"
  if [ -z "$value" ]; then
    echo "missing required env: $1" >&2
    exit 2
  fi
}

need POSTGRES_TEST_DSN
need PLAN4_CONTROL_STATE_ENCRYPTION_KEY
need PLAN4_PROVIDER_API_KEY
need PLAN4_DEV_API_KEY

go test -timeout 60s ./tests/integration -run TestPlan4PostgresSmoke -count=1
