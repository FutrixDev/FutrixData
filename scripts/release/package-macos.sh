#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
BUILD_DIR="$PROJECT_DIR/build/bin"
OUTPUT_DIR=""
BUILD_SCRIPT=""
MODE="unsigned"
PLATFORMS=()

# shellcheck source=/dev/null
source "$SCRIPT_DIR/release-common.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --project-dir DIR                     FutrixData project root (default: current directory)
  --platform darwin/arm64|darwin/amd64   Package a single macOS architecture
  --signed                               Sign and notarize the DMG
  --unsigned                             Build an unsigned DMG (default)
  --output-dir DIR                       Copy final artifacts to DIR
  -h, --help                             Show this help

Examples:
  $(basename "$0")
  $(basename "$0") --project-dir /path/to/FutrixData --platform darwin/arm64
  $(basename "$0") --signed --platform darwin/amd64
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    --platform)
      PLATFORMS+=("${2:-}")
      shift 2
      ;;
    --signed)
      MODE="signed"
      shift
      ;;
    --unsigned)
      MODE="unsigned"
      shift
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
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

BUILD_DIR="$PROJECT_DIR/build/bin"
BUILD_SCRIPT="${FUTRIXDATA_BUILD_SCRIPT:-$PROJECT_DIR/scripts/build.sh}"

if [ ! -d "$PROJECT_DIR" ]; then
  echo "❌ Project directory not found: $PROJECT_DIR" >&2
  exit 1
fi

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "❌ Build script not found or not executable: $BUILD_SCRIPT" >&2
  exit 1
fi

if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$PROJECT_DIR/build/packages"
fi

if [ "${#PLATFORMS[@]}" -eq 0 ]; then
  PLATFORMS=("darwin/arm64" "darwin/amd64")
fi

mkdir -p "$OUTPUT_DIR"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Missing required command: $1" >&2
    exit 1
  fi
}

require_command wails
require_command create-dmg

for platform in "${PLATFORMS[@]}"; do
  case "$platform" in
    darwin/arm64|darwin/amd64)
      ;;
    *)
      echo "❌ Unsupported macOS platform: $platform" >&2
      exit 1
      ;;
  esac

  arch="${platform#*/}"
  final_name="$(artifact_filename "$PROJECT_DIR" "macos-${arch}")"
  final_path="$BUILD_DIR/$final_name"

  echo "📦 Packaging macOS ${arch} (${MODE})..."

  if [ "$MODE" = "signed" ]; then
    "$SCRIPT_DIR/release-macos.sh" --project-dir "$PROJECT_DIR" --platform "$platform"
  else
    rm -f "$BUILD_DIR/FutrixData.dmg" "$final_path"
    "$BUILD_SCRIPT" --platform "$platform"
    FUTRIXDATA_SKIP_DMG_CODESIGN=1 "$SCRIPT_DIR/build-dmg.sh" --project-dir "$PROJECT_DIR"
    mv -f "$BUILD_DIR/FutrixData.dmg" "$final_path"
  fi

  cp -f "$final_path" "$OUTPUT_DIR/"
  echo "✅ Ready: $OUTPUT_DIR/$final_name"
done
