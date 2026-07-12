#!/usr/bin/env sh
set -eu
if (set -o pipefail) 2>/dev/null; then
  set -o pipefail
fi

PROFILE="${VELOXMESH_PROFILE:-simple}"
INSTALL_DIR="${VELOXMESH_INSTALL_DIR:-$(pwd)/VeloxMesh}"
PROJECT_NAME="${VELOXMESH_PROJECT_NAME:-veloxmesh}"
BUILD_CONTEXT="${VELOXMESH_BUILD_CONTEXT:-}"
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
HOST_UID="${VELOXMESH_HOST_UID:-}"
HOST_GID="${VELOXMESH_HOST_GID:-}"

usage() {
  cat <<'EOF'
Usage: install.sh [options]

Options:
  --profile simple|full|compare|postgres
  --install-dir ./VeloxMesh
  --project-name veloxmesh
  --build-context https://github.com/zardonc/VeloxMesh.git#main
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
    --project-name) PROJECT_NAME="$2"; shift 2 ;;
    --build-context) BUILD_CONTEXT="$2"; shift 2 ;;
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

refuse_root() {
  if command -v id >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
    echo "Do not run install.sh with sudo/root." >&2
    echo "Run it as the user who will edit VeloxMesh config files." >&2
    exit 2
  fi
}

current_id() {
  flag="$1"
  if command -v id >/dev/null 2>&1; then
    id "$flag"
    return
  fi
  echo "1000"
}

random_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  random_hex 24
}

random_key32() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 16
    return
  fi
  random_hex 16
}

random_hex() {
  bytes="$1"
  od -An -N "$bytes" -tx1 /dev/urandom | tr -d ' \n'
  echo
}

download() {
  src="$1"
  dst="$2"
  mkdir -p "$(dirname "$dst")"
  prepare_file_target "$dst"
  curl -fsSL "$RAW_BASE/$src" -o "$dst"
}

prepare_file_target() {
  dst="$1"
  if [ ! -e "$dst" ] || [ -f "$dst" ]; then
    return
  fi
  if [ -d "$dst" ] && rmdir "$dst" 2>/dev/null; then
    return
  fi
  echo "Refusing to overwrite non-file path: $dst" >&2
  echo "Remove it manually, then rerun install.sh." >&2
  exit 2
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

read_env_value() {
  key="$1"
  file="$2"
  sed -n "s/^$key=//p" "$file" | tail -n 1
}

read_app_admin_key() {
  file="$INSTALL_DIR/config/app.$APP_PROFILE_NAME.json"
  if [ -f "$file" ]; then
    sed -n 's/.*"admin_api_key"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p; t done; b; :done; q' "$file"
  fi
}

load_existing_env() {
  env_file="$1"
  check_existing_profile "$env_file"
  loaded="$(read_env_value VELOXMESH_PROJECT_NAME "$env_file")"; if [ -n "$loaded" ]; then PROJECT_NAME="$loaded"; fi
  loaded="$(read_env_value VELOXMESH_HOST_UID "$env_file")"; if [ -n "$loaded" ]; then HOST_UID="$loaded"; fi
  loaded="$(read_env_value VELOXMESH_HOST_GID "$env_file")"; if [ -n "$loaded" ]; then HOST_GID="$loaded"; fi
  loaded="$(read_env_value DEV_API_KEY "$env_file")"; if [ -n "$loaded" ]; then DEV_API_KEY="$loaded"; fi
  loaded="$(read_env_value OPENAI_PRIMARY_API_KEY "$env_file")"; if [ -n "$loaded" ]; then PROVIDER_API_KEY="$loaded"; fi
  loaded="$(read_env_value GRAFANA_ADMIN_PASSWORD "$env_file")"; if [ -n "$loaded" ]; then GRAFANA_PASSWORD="$loaded"; fi
  loaded="$(read_env_value POSTGRES_PASSWORD "$env_file")"; if [ -n "$loaded" ]; then POSTGRES_PASSWORD="$loaded"; fi
  loaded="$(read_app_admin_key)"; if [ -n "$loaded" ]; then ADMIN_API_KEY="$loaded"; fi
}

ensure_env_value() {
  key="$1"
  value="$2"
  file="$3"
  if grep -q "^$key=" "$file"; then
    return
  fi
  printf '%s=%s\n' "$key" "$value" >>"$file"
}

check_existing_profile() {
  env_file="$1"
  existing_profile="$(read_env_value VELOXMESH_PROFILE "$env_file")"
  if [ -n "$existing_profile" ] && [ "$existing_profile" != "$PROFILE" ]; then
    echo "Existing install env uses profile '$existing_profile', but requested '$PROFILE'." >&2
    echo "Use a different --install-dir, edit $env_file, or uninstall before changing profiles." >&2
    exit 2
  fi
  expected_app="../config/app.$APP_PROFILE_NAME.json"
  existing_app="$(read_env_value VELOXMESH_APP_CONFIG "$env_file")"
  if [ -n "$existing_app" ] && [ "$existing_app" != "$expected_app" ]; then
    echo "Existing install env uses $existing_app, but profile '$PROFILE' expects $expected_app." >&2
    echo "Use a different --install-dir, edit $env_file, or uninstall before changing profiles." >&2
    exit 2
  fi
}

write_env_if_missing() {
  env_file="$INSTALL_DIR/env/veloxmesh.env"
  if [ -f "$env_file" ]; then
    load_existing_env "$env_file"
    ensure_env_value VELOXMESH_PROJECT_NAME "$PROJECT_NAME" "$env_file"
    ensure_env_value VELOXMESH_GATEWAY_BIND_ADDR "0.0.0.0" "$env_file"
    ensure_env_value VELOXMESH_ADMIN_BIND_ADDR "127.0.0.1" "$env_file"
    ensure_env_value VELOXMESH_LOCAL_BIND_ADDR "127.0.0.1" "$env_file"
    ensure_env_value VELOXMESH_HOST_UID "$HOST_UID" "$env_file"
    ensure_env_value VELOXMESH_HOST_GID "$HOST_GID" "$env_file"
    chmod 600 "$env_file" 2>/dev/null || true
    return
  fi
  prepare_file_target "$env_file"
  cat >"$env_file" <<EOF
VELOXMESH_PROFILE=$PROFILE
VELOXMESH_PROJECT_NAME=$PROJECT_NAME
VELOXMESH_GATEWAY_BIND_ADDR=0.0.0.0
VELOXMESH_ADMIN_BIND_ADDR=127.0.0.1
VELOXMESH_LOCAL_BIND_ADDR=127.0.0.1
VELOXMESH_HOST_UID=$HOST_UID
VELOXMESH_HOST_GID=$HOST_GID
DEV_API_KEY=$DEV_API_KEY
OPENAI_PRIMARY_API_KEY=$PROVIDER_API_KEY
VELOXMESH_BUILD_CONTEXT=$BUILD_CONTEXT
VELOXMESH_APP_CONFIG=../config/app.$APP_PROFILE_NAME.json
VELOXMESH_SCHEDULER_CONFIG=../config/scheduler.$SCHEDULER_PROFILE_NAME.json
VELOXMESH_CACHE_CONFIG=../config/cache.$CACHE_PROFILE_NAME.json
VELOXMESH_PIPELINE_CONFIG=../config/pipeline.yaml
VELOXMESH_PROMETHEUS_CONFIG=../observability/$PROMETHEUS_FILE
GRAFANA_ADMIN_USER=admin
GRAFANA_ADMIN_PASSWORD=$GRAFANA_PASSWORD
POSTGRES_USER=$POSTGRES_USER
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
POSTGRES_DB=$POSTGRES_DB
POSTGRES_PORT=$POSTGRES_PORT
EOF
  chmod 600 "$env_file" 2>/dev/null || true
}

patch_app_config() {
  file="$INSTALL_DIR/config/app.$APP_PROFILE_NAME.json"
  if [ ! -f "$file" ]; then
    echo "Expected app config file, got non-file path: $file" >&2
    exit 2
  fi
  base_url="$(sed_escape "$PROVIDER_BASE_URL")"
  model="$(sed_escape "$PROVIDER_MODEL")"
  admin_key="$(sed_escape "$ADMIN_API_KEY")"
  enc_key="$(sed_escape "$CONTROL_STATE_ENCRYPTION_KEY")"
  postgres_dsn="$(sed_escape "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@postgres:5432/$POSTGRES_DB?sslmode=disable")"
  sed -i "s/https:\/\/api.example.invalid\/v1/$base_url/g" "$file"
  sed -i "s/example-model/$model/g" "$file"
  sed -i "s/replace-with-local-admin-token/$admin_key/g" "$file"
  sed -i "s/replace-with-32-byte-local-key!!/$enc_key/g" "$file"
  sed -i "s/postgres:\/\/replace-with-postgres-user:replace-with-postgres-password@postgres:5432\/replace-with-postgres-database?sslmode=disable/$postgres_dsn/g" "$file"
  chmod 600 "$file" 2>/dev/null || true
}

refuse_root
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
if [ -z "$HOST_UID" ]; then HOST_UID="$(current_id -u)"; fi
if [ -z "$HOST_GID" ]; then HOST_GID="$(current_id -g)"; fi
CONTROL_STATE_ENCRYPTION_KEY="$(random_key32)"

if [ -z "$RAW_BASE" ]; then
  repo_slug="$(printf '%s' "$REPO_URL" | sed -E 's#^https://github.com/##; s#^git@github.com:##; s#\.git$##')"
  RAW_BASE="https://raw.githubusercontent.com/$repo_slug/$BRANCH"
fi
if [ -z "$BUILD_CONTEXT" ]; then
  BUILD_CONTEXT="$REPO_URL#$BRANCH"
fi

APP_PROFILE_NAME="$PROFILE"
SCHEDULER_PROFILE_NAME="$PROFILE"
CACHE_PROFILE_NAME="$PROFILE"
PROFILES="--profile $PROFILE"
PROMETHEUS_FILE="prometheus.yml"
GATEWAY_SERVICE="gateway"
if [ "$PROFILE" = "postgres" ]; then
  APP_PROFILE_NAME="postgres"
  SCHEDULER_PROFILE_NAME="full"
  CACHE_PROFILE_NAME="postgres"
  PROFILES="--profile full --profile postgres"
fi
if [ "$PROFILE" = "compare" ]; then
  PROMETHEUS_FILE="prometheus.compare.yml"
  GATEWAY_SERVICE="gateway-compare"
fi

if [ -f "$INSTALL_DIR/env/veloxmesh.env" ]; then
  check_existing_profile "$INSTALL_DIR/env/veloxmesh.env"
fi

mkdir -p "$INSTALL_DIR/compose" "$INSTALL_DIR/env" "$INSTALL_DIR/config" "$INSTALL_DIR/models/current" "$INSTALL_DIR/data" "$INSTALL_DIR/reports" "$INSTALL_DIR/observability"

if [ ! -w "$INSTALL_DIR" ]; then
  echo "Install dir is not writable by the current user: $INSTALL_DIR" >&2
  echo "Fix ownership first, for example: sudo chown -R \"\$(id -u):\$(id -g)\" \"$INSTALL_DIR\"" >&2
  exit 1
fi

download deploy/compose/veloxmesh.yml "$INSTALL_DIR/compose/veloxmesh.yml"
if download_if_missing "deploy/config/app.$APP_PROFILE_NAME.example.json" "$INSTALL_DIR/config/app.$APP_PROFILE_NAME.json"; then
  patch_app_config
fi
download_if_missing "deploy/config/scheduler.$SCHEDULER_PROFILE_NAME.example.json" "$INSTALL_DIR/config/scheduler.$SCHEDULER_PROFILE_NAME.json" || true
download_if_missing "deploy/config/cache.$CACHE_PROFILE_NAME.example.json" "$INSTALL_DIR/config/cache.$CACHE_PROFILE_NAME.json" || true
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
docker compose -p "$PROJECT_NAME" --env-file "$INSTALL_DIR/env/veloxmesh.env" -f "$INSTALL_DIR/compose/veloxmesh.yml" $PROFILES up -d --build

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
  docker compose -p $PROJECT_NAME --env-file $INSTALL_DIR/env/veloxmesh.env -f $INSTALL_DIR/compose/veloxmesh.yml ps
  docker compose -p $PROJECT_NAME --env-file $INSTALL_DIR/env/veloxmesh.env -f $INSTALL_DIR/compose/veloxmesh.yml logs -f $GATEWAY_SERVICE
  curl http://localhost:8080/healthz
  curl http://localhost:8080/v1/models -H "Authorization: Bearer $DEV_API_KEY"
EOF
