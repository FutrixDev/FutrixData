#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
OUTPUT_DIR=""
IMAGE_TAG="${LINUX_BUILDER_IMAGE:-futrixdata-linux-builder:latest}"
IMAGE_PLATFORM="linux/amd64"
REBUILD_IMAGE=0

# shellcheck source=/dev/null
source "$SCRIPT_DIR/release-common.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --project-dir DIR    FutrixData project root (default: current directory)
  --output-dir DIR     Copy final artifact to DIR
  --rebuild-image      Rebuild the Linux builder image before packaging
  -h, --help           Show this help

Environment:
  LINUX_BUILDER_IMAGE  Docker image tag to use (default: futrixdata-linux-builder:latest)
  NODE_OPTIONS         Optional Node.js flags passed into the Linux builder
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    --rebuild-image)
      REBUILD_IMAGE=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ ! -d "$PROJECT_DIR" ]; then
  echo "❌ Project directory not found: $PROJECT_DIR" >&2
  exit 1
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Missing required command: $1" >&2
    exit 1
  fi
}

require_command docker

ensure_builder_image() {
  if [ "$REBUILD_IMAGE" = "1" ] || ! docker image inspect "$IMAGE_TAG" >/dev/null 2>&1; then
    echo "🐳 Building Linux builder image: $IMAGE_TAG"
    docker build \
      --platform "$IMAGE_PLATFORM" \
      -f "$SCRIPT_DIR/Dockerfile.linux-builder" \
      -t "$IMAGE_TAG" \
      "$PROJECT_DIR"
  fi
}

if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$PROJECT_DIR/build/packages"
fi

mkdir -p "$OUTPUT_DIR"
ensure_builder_image

TMP_BUILD_ROOT="$(mktemp -d /tmp/futrix-linux-package.XXXXXX)"
CONTAINER_OUTPUT_DIR="$TMP_BUILD_ROOT/container-output"
BUNDLE_DIRNAME="$(linux_bundle_dirname "$PROJECT_DIR")"
STAGING_DIR="$TMP_BUILD_ROOT/$BUNDLE_DIRNAME"
ARTIFACT_NAME="$(artifact_filename "$PROJECT_DIR" "linux-amd64")"
ARTIFACT_PATH="$OUTPUT_DIR/$ARTIFACT_NAME"
mkdir -p "$CONTAINER_OUTPUT_DIR" "$STAGING_DIR"

cleanup() {
  rm -rf "$TMP_BUILD_ROOT"
}

trap cleanup EXIT

echo "📦 Packaging Linux amd64 bundle..."
DOCKER_ENV_ARGS=()
if [ -n "${NODE_OPTIONS:-}" ]; then
  DOCKER_ENV_ARGS+=(-e "NODE_OPTIONS=$NODE_OPTIONS")
fi

docker run --rm \
  --platform "$IMAGE_PLATFORM" \
  -v "$PROJECT_DIR:/input:ro" \
  -v "$CONTAINER_OUTPUT_DIR:/out" \
  "${DOCKER_ENV_ARGS[@]}" \
  "$IMAGE_TAG" \
  bash -lc '
    set -euo pipefail
    export PATH=/go/bin:/usr/local/go/bin:$PATH
    rm -rf /src
    mkdir -p /src
    cp -a /input/. /src/
    cd /src/frontend
    rm -rf node_modules
    npm install
    cd /src
    ./scripts/build.sh --platform linux/amd64
    cp build/bin/FutrixData /out/
    cp build/bin/futrixdata-cli /out/
  '

cp "$CONTAINER_OUTPUT_DIR/FutrixData" "$STAGING_DIR/"
cp "$CONTAINER_OUTPUT_DIR/futrixdata-cli" "$STAGING_DIR/"
cat > "$STAGING_DIR/README.txt" <<'EOF'
FutrixData Linux amd64 package

Contents:
- FutrixData
- futrixdata-cli

Run the desktop app with:
./FutrixData
EOF

tar -czf "$ARTIFACT_PATH" -C "$TMP_BUILD_ROOT" "$(basename "$STAGING_DIR")"
echo "✅ Ready: $ARTIFACT_PATH"
