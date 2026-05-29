#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
BUILD_DIR="$PROJECT_DIR/build/bin"
OUTPUT_DIR=""
BUILD_SCRIPT=""
PLATFORM="windows/amd64"
INSTALLER_NAME="FutrixData-amd64-installer.exe"

# shellcheck source=/dev/null
source "$SCRIPT_DIR/release-common.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --project-dir DIR   FutrixData project root (default: current directory)
  --output-dir DIR    Copy final artifact to DIR
  -h, --help          Show this help

Notes:
  This script builds the Windows NSIS installer.
  It does not sign the installer.
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

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Missing required command: $1" >&2
    exit 1
  fi
}

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

require_command wails
require_command makensis

if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$PROJECT_DIR/build/packages"
fi

mkdir -p "$OUTPUT_DIR"

echo "📦 Packaging Windows amd64 installer..."
"$BUILD_SCRIPT" --platform "$PLATFORM" --nsis

if [ ! -f "$BUILD_DIR/$INSTALLER_NAME" ]; then
  echo "❌ Expected installer not found: $BUILD_DIR/$INSTALLER_NAME" >&2
  exit 1
fi

FINAL_NAME="$(artifact_filename "$PROJECT_DIR" "windows-amd64")"
cp -f "$BUILD_DIR/$INSTALLER_NAME" "$OUTPUT_DIR/$FINAL_NAME"
echo "✅ Ready: $OUTPUT_DIR/$FINAL_NAME"
