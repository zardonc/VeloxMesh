#!/usr/bin/env sh
set -eu
if (set -o pipefail) 2>/dev/null; then
  set -o pipefail
fi

INSTALL_DIR="${VELOXMESH_INSTALL_DIR:-$(pwd)/VeloxMesh}"
PROJECT_NAME="${VELOXMESH_PROJECT_NAME:-}"
YES=false
REMOVE_VOLUMES=false

usage() {
  cat <<'EOF'
Usage: uninstall.sh [options]

Options:
  --install-dir ./VeloxMesh  Installed VeloxMesh directory to remove
  --project-name veloxmesh   Docker Compose project name to stop
  --yes                     Do not prompt before deleting files
  --volumes                 Also remove Docker Compose named volumes
  -h, --help                Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --project-name) PROJECT_NAME="$2"; shift 2 ;;
    --yes|-y) YES=true; shift ;;
    --volumes) REMOVE_VOLUMES=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "unknown option: $1" >&2; usage >&2; exit 2 ;;
  esac
done

case "$INSTALL_DIR" in
  ""|"/"|".")
    echo "Refusing unsafe install dir: $INSTALL_DIR" >&2
    exit 2
    ;;
esac

if [ ! -d "$INSTALL_DIR" ]; then
  echo "Nothing to uninstall: $INSTALL_DIR does not exist."
  exit 0
fi

compose_file="$INSTALL_DIR/compose/veloxmesh.yml"
env_file="$INSTALL_DIR/env/veloxmesh.env"

if [ ! -f "$compose_file" ] || [ ! -f "$env_file" ]; then
  echo "Refusing to remove $INSTALL_DIR: not a VeloxMesh install directory." >&2
  echo "Expected $compose_file and $env_file." >&2
  exit 2
fi

read_env_value() {
  key="$1"
  file="$2"
  sed -n "s/^$key=//p" "$file" | tail -n 1
}

if [ -z "$PROJECT_NAME" ]; then
  PROJECT_NAME="$(read_env_value VELOXMESH_PROJECT_NAME "$env_file")"
fi
if [ -z "$PROJECT_NAME" ]; then
  PROJECT_NAME="veloxmesh"
fi

if command -v id >/dev/null 2>&1 && [ "$(id -u)" = "0" ]; then
  echo "Do not run uninstall.sh with sudo/root." >&2
  echo "Run it as the user who installed VeloxMesh and has Docker access." >&2
  exit 2
fi

if [ "$YES" != "true" ]; then
  if [ ! -t 0 ]; then
    echo "Refusing to prompt in non-interactive mode. Re-run with --yes to uninstall." >&2
    exit 2
  fi
  echo "This will stop VeloxMesh and delete: $INSTALL_DIR"
  echo "Docker Compose project: $PROJECT_NAME"
  if [ "$REMOVE_VOLUMES" = "true" ]; then
    echo "Docker Compose named volumes will also be removed."
  fi
  printf "Continue? [y/N] "
  read answer
  case "$answer" in
    y|Y|yes|YES) ;;
    *) echo "Uninstall cancelled."; exit 0 ;;
  esac
fi

if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "Docker Compose not found; refusing to delete files before containers are stopped." >&2
  exit 1
fi

down_args=""
if [ "$REMOVE_VOLUMES" = "true" ]; then
  down_args="-v"
fi

remaining_project_containers() {
  docker ps -aq --filter "label=com.docker.compose.project=$PROJECT_NAME"
}

remove_labeled_containers() {
  containers="$(remaining_project_containers)"
  if [ -z "$containers" ]; then
    return 0
  fi
  docker stop $containers >/dev/null 2>&1 || true
  docker rm -f $containers
}

remove_labeled_volumes() {
  volumes="$(docker volume ls -q --filter "label=com.docker.compose.project=$PROJECT_NAME")"
  if [ -n "$volumes" ]; then
    docker volume rm $volumes
  fi
}

# shellcheck disable=SC2086
if ! docker compose -p "$PROJECT_NAME" --env-file "$env_file" -f "$compose_file" down --remove-orphans $down_args; then
  echo "docker compose down failed; trying direct cleanup by Compose project label." >&2
  remove_labeled_containers
fi

remaining="$(remaining_project_containers)"
if [ -n "$remaining" ]; then
  echo "docker compose down left VeloxMesh containers; trying direct cleanup by Compose project label." >&2
  remove_labeled_containers
  remaining="$(remaining_project_containers)"
fi

if [ -n "$remaining" ]; then
  echo "Refusing to delete files because VeloxMesh containers still exist:" >&2
  docker ps -a --filter "label=com.docker.compose.project=$PROJECT_NAME" --format "  {{.ID}} {{.Names}} {{.Status}}" >&2
  exit 1
fi

if [ "$REMOVE_VOLUMES" = "true" ]; then
  remove_labeled_volumes
fi

rm -rf "$INSTALL_DIR"
echo "VeloxMesh uninstalled from $INSTALL_DIR"
