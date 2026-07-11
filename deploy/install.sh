#!/usr/bin/env sh
set -eu

PROFILE="${VELOXMESH_PROFILE:-simple}"
INSTALL_DIR="${VELOXMESH_INSTALL_DIR:-/opt/veloxmesh}"
REPO_URL="${VELOXMESH_REPO_URL:-https://github.com/zardonc/VeloxMesh.git}"
BRANCH="${VELOXMESH_BRANCH:-main}"
RAW_BASE="${VELOXMESH_RAW_BASE:-}"
DEV_API_KEY="${VELOXMESH_DEV_API_KEY:-}"
ADMIN_API_KEY="${VELOXMESH_ADMIN_API_KEY:-}"
PROVIDER_API_KEY="${OPENAI_PRIMARY_API_KEY:-}"
PROVIDER_BASE_URL="${OPENAI_PRIMARY_BASE_URL:-https://api.example.invalid/v1}"
PROVIDER_MODEL="${OPENAI_PRIMARY_MODEL:-example-model}"
GRAFANA_PASSWORD="${GRAFANA_ADMIN_PASSWORD:-}"
POSTGRES_USER="${POSTGRES_USER:-veloxmesh}"
POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-}"
POSTGRES_DB="${POSTGRES_DB:-veloxmesh}"
POSTGRES_PORT="${POSTGRES_PORT:-5432}"

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --profile simple|full|compare|postgres
  --install-dir /opt/veloxmesh
  --repo-url https://github.com/zardonc/VeloxMesh.git
  --branch main
  --raw-base https://raw.githubusercontent.com/zardonc/VeloxMesh/main
  --dev-api-key value
  --admin-api-key value
  --provider-api-key value
  --provider-base-url https://api.example.invalid/v1
  --provider-model example-model
  --grafana-password value
  --postgres-user value
  --postgres-password value
  --postgres-db value
  --postgres-port 5432
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --profile) PROFILE="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --repo-url) REPO_URL="$2"; shift 2 ;;
    --branch) BRANCH="$2"; shift 2 ;;
    --raw-base) RAW_BASE="$2"; shift 2 ;;
    --dev-api-key) DEV_API_KEY="$2"; shift 2 ;;
    --admin-api-key) ADMIN_API_KEY="$2"; shift 2 ;;
    --provider-api-key) PROVIDER_API_KEY="$2"; shift 2 ;;
    --provider-base-url) PROVIDER_BASE_URL="$2"; shift 2 ;;
    --provider-model) PROVIDER_MODEL="$2"; shift 2 ;;
    --grafana-password) GRAFANA_PASSWORD="$2"; shift 2 ;;
    --postgres-user) POSTGRES_USER="$2"; shift 2 ;;
    --postgres-password) POSTGRES_PASSWORD="$2"; shift 2 ;;
    --postgres-db) POSTGRES_DB="$2"; shift 2 ;;
    --postgres-port) POSTGRES_PORT="$2"; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$PROFILE" in
  simple|full|compare|postgres) ;;
  *) echo "profile must be simple, full, compare, or postgres" >&2; exit 2 ;;
esac

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

random_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 48
  echo
}

random_key32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -base64 48 | tr -dc 'A-Za-z0-9' | head -c 32
    echo
    return
  fi
  LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32
  echo
}

download() {
  src="$1"
  dst="$2"
  mkdir -p "$(dirname "$dst")"
  curl -fsSL "$RAW_BASE/$src" -o "$dst"
}

download_if_missing() {
  src="$1"
  dst="$2"
  if [ -f "$dst" ]; then
    return 1
  fi
  download "$src" "$dst"
  return 0
}

sed_escape() {
  printf '%s' "$1" | sed 's/[\/&]/\\&/g'
}

write_env_if_missing() {
  env_file="$INSTALL_DIR/env/veloxmesh.env"
  if [ -f "$env_file" ]; then
    # shellcheck disable=SC1090
    . "$env_file"
    return
  fi
  cat >"$env_file" <<EOF
DEV_API_KEY=$DEV_API_KEY
OPENAI_PRIMARY_API_KEY=$PROVIDER_API_KEY
VELOXMESH_BUILD_CONTEXT=$REPO_URL#$BRANCH
VELOXMESH_APP_CONFIG=../config/app.$PROFILE_NAME.json
VELOXMESH_SCHEDULER_CONFIG=../config/scheduler.$PROFILE_NAME.json
VELOXMESH_CACHE_CONFIG=../config/cache.$PROFILE_NAME.json
VELOXMESH_PIPELINE_CONFIG=../config/pipeline.yaml
VELOXMESH_PROMETHEUS_CONFIG=../observability/$PROMETHEUS_FILE
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$GRAFANA_PASSWORD
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=$POSTGRES_DB
POSTGRES_PORT=$POSTGRES_PORT
EOF
}

patch_app_config() {
  file="$INSTALL_DIR/config/app.$PROFILE_NAME.json"
  base_url="$(sed_escape "$PROVIDER_BASE_URL")"
  model="$(sed_escape "$PROVIDER_MODEL")"
  admin_key="$(sed_escape "$ADMIN_API_KEY")"
  enc_key="$(sed_escape "$CONTROL_STATE_ENCRYPTION_KEY")"
  sed -i "s/https:\/\/api.example.invalid\/v1/$base_url/g" "$file"
  sed -i "s/example-model/$model/g" "$file"
  sed -i "s/replace-with-local-admin-token/$admin_key/g" "$file"
  sed -i "s/replace-with-32-byte-local-key!!/$enc_key/g" "$file"
}

need docker
need curl

docker compose version >/dev/null 2>&1 || {
  echo "Docker Compose v2 is required: docker compose version failed" >&2
  exit 1
}

if [ -z "$DEV_API_KEY" ]; then DEV_API_KEY="vx-$(random_token)"; fi
if [ -z "$ADMIN_API_KEY" ]; then ADMIN_API_KEY="adm-$(random_token)"; fi
if [ -z "$PROVIDER_API_KEY" ]; then PROVIDER_API_KEY="replace-with-provider-api-key"; fi
if [ -z "$GRAFANA_PASSWORD" ]; then GRAFANA_PASSWORD="$(random_token)"; fi
if [ -z "$POSTGRES_PASSWORD" ]; then POSTGRES_PASSWORD="$(random_token)"; fi
CONTROL_STATE_ENCRYPTION_KEY="$(random_key32)"

if [ -z "$RAW_BASE" ]; then
  repo_slug="$(printf '%s' "$REPO_URL" | sed -E 's#^https://github.com/##; s#^git@github.com:##; s#\.git$##')"
  RAW_BASE="https://raw.githubusercontent.com/$repo_slug/$BRANCH"
fi

PROFILE_NAME="$PROFILE"
PROFILES="--profile $PROFILE"
PROMETHEUS_FILE="prometheus.yml"
GATEWAY_SERVICE="gateway"
if [ "$PROFILE" = "postgres" ]; then
  PROFILE_NAME="full"
  PROFILES="--profile full --profile postgres"
fi
if [ "$PROFILE" = "compare" ]; then
  PROMETHEUS_FILE="prometheus.compare.yml"
  GATEWAY_SERVICE="gateway-compare"
fi

mkdir -p "$INSTALL_DIR/compose" "$INSTALL_DIR/env" "$INSTALL_DIR/config" "$INSTALL_DIR/models/current" "$INSTALL_DIR/data" "$INSTALL_DIR/reports" "$INSTALL_DIR/observability"

download deploy/compose/veloxmesh.yml "$INSTALL_DIR/compose/veloxmesh.yml"
if download_if_missing "deploy/config/app.$PROFILE_NAME.example.json" "$INSTALL_DIR/config/app.$PROFILE_NAME.json"; then
  patch_app_config
fi
download_if_missing "deploy/config/scheduler.$PROFILE_NAME.example.json" "$INSTALL_DIR/config/scheduler.$PROFILE_NAME.json" || true
download_if_missing "deploy/config/cache.$PROFILE_NAME.example.json" "$INSTALL_DIR/config/cache.$PROFILE_NAME.json" || true
download_if_missing deploy/config/pipeline.example.yaml "$INSTALL_DIR/config/pipeline.yaml" || true
download deploy/config/heuristic.example.json "$INSTALL_DIR/config/heuristic.example.json"
download "deploy/observability/$PROMETHEUS_FILE" "$INSTALL_DIR/observability/$PROMETHEUS_FILE"
download deploy/observability/scheduler-alerts.yml "$INSTALL_DIR/observability/scheduler-alerts.yml"
download deploy/observability/grafana-datasources.yml "$INSTALL_DIR/observability/grafana-datasources.yml"
download deploy/observability/otel-collector-config.yaml "$INSTALL_DIR/observability/otel-collector-config.yaml"
download deploy/observability/promtail.yml "$INSTALL_DIR/observability/promtail.yml"

write_env_if_missing

echo "Starting VeloxMesh profile '$PROFILE' in $INSTALL_DIR"
# shellcheck disable=SC2086
docker compose --env-file "$INSTALL_DIR/env/veloxmesh.env" -f "$INSTALL_DIR/compose/veloxmesh.yml" $PROFILES up -d --build

cat <<EOF

VeloxMesh deployed.

Gateway API:  http://localhost:8080
Admin API:    http://localhost:8081
Grafana:      http://localhost:3000
Prometheus:   http://localhost:9090

Config:       $INSTALL_DIR/config
Env:          $INSTALL_DIR/env/veloxmesh.env

DEV_API_KEY:  $DEV_API_KEY
ADMIN_API_KEY:$ADMIN_API_KEY
Grafana user: admin
Grafana pass: $GRAFANA_PASSWORD

Useful commands:
  docker compose --env-file $INSTALL_DIR/env/veloxmesh.env -f $INSTALL_DIR/compose/veloxmesh.yml ps
  docker compose --env-file $INSTALL_DIR/env/veloxmesh.env -f $INSTALL_DIR/compose/veloxmesh.yml logs -f $GATEWAY_SERVICE
  curl http://localhost:8080/healthz
  curl http://localhost:8080/v1/models -H "Authorization: Bearer $DEV_API_KEY"
EOF
