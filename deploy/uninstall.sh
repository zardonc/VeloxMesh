#!/usr/bin/env sh
set -eu

INSTALL_DIR="${VELOXMESH_INSTALL_DIR:-$(pwd)/VeloxMesh}"
YES=false
REMOVE_VOLUMES=false

usage() {
  cat <<'EOF'
Usage: uninstall.sh [options]

Options:
  --install-dir ./VeloxMesh  Installed VeloxMesh directory to remove
  --yes                     Do not prompt before deleting files
  --volumes                 Also remove Docker Compose named volumes
  -h, --help                Show this help
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
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

if [ "$YES" != "true" ]; then
  echo "This will stop VeloxMesh and delete: $INSTALL_DIR"
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
# shellcheck disable=SC2086
docker compose --env-file "$env_file" -f "$compose_file" down $down_args

rm -rf "$INSTALL_DIR"
echo "VeloxMesh uninstalled from $INSTALL_DIR"
