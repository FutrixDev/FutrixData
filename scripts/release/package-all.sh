#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
OUTPUT_DIR=""
ONLY=""
SIGNED_MACOS=0
REBUILD_LINUX_IMAGE=0

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --project-dir DIR             FutrixData project root (default: current directory)
  --only macos|windows|linux   Package a single platform group
  --signed-macos               Sign and notarize macOS packages
  --rebuild-linux-image        Rebuild the Linux Docker builder image
  --output-dir DIR             Collect final artifacts in DIR
  -h, --help                   Show this help

Examples:
  $(basename "$0")
  $(basename "$0") --project-dir /path/to/FutrixData --signed-macos
  $(basename "$0") --only linux --rebuild-linux-image
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    --only)
      ONLY="${2:-}"
      shift 2
      ;;
    --signed-macos)
      SIGNED_MACOS=1
      shift
      ;;
    --rebuild-linux-image)
      REBUILD_LINUX_IMAGE=1
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

if [ ! -d "$PROJECT_DIR" ]; then
  echo "❌ Project directory not found: $PROJECT_DIR" >&2
  exit 1
fi

if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$PROJECT_DIR/build/packages"
fi

mkdir -p "$OUTPUT_DIR"

run_macos() {
  if [ "$SIGNED_MACOS" = "1" ]; then
    "$SCRIPT_DIR/package-macos.sh" --project-dir "$PROJECT_DIR" --signed --output-dir "$OUTPUT_DIR"
  else
    "$SCRIPT_DIR/package-macos.sh" --project-dir "$PROJECT_DIR" --unsigned --output-dir "$OUTPUT_DIR"
  fi
}

run_windows() {
  "$SCRIPT_DIR/package-windows.sh" --project-dir "$PROJECT_DIR" --output-dir "$OUTPUT_DIR"
}

run_linux() {
  if [ "$REBUILD_LINUX_IMAGE" = "1" ]; then
    "$SCRIPT_DIR/package-linux.sh" --project-dir "$PROJECT_DIR" --rebuild-image --output-dir "$OUTPUT_DIR"
  else
    "$SCRIPT_DIR/package-linux.sh" --project-dir "$PROJECT_DIR" --output-dir "$OUTPUT_DIR"
  fi
}

case "$ONLY" in
  "")
    run_macos
    run_windows
    run_linux
    ;;
  macos)
    run_macos
    ;;
  windows)
    run_windows
    ;;
  linux)
    run_linux
    ;;
  *)
    echo "❌ Unsupported --only value: $ONLY" >&2
    usage >&2
    exit 1
    ;;
esac

echo "🧮 Writing checksums..."
(
  cd "$OUTPUT_DIR"
  TMP_CHECKSUM_FILE=".SHA256SUMS.txt.tmp"
  find . -maxdepth 1 -type f \
    ! -name "SHA256SUMS.txt" \
    ! -name "$TMP_CHECKSUM_FILE" \
    ! -name "RELEASE_NOTES.md" \
    -print \
    | sed 's#^\./##' \
    | LC_ALL=C sort \
    | while IFS= read -r artifact; do
        [ -n "$artifact" ] || continue
        shasum -a 256 "$artifact"
      done > "$TMP_CHECKSUM_FILE"
  mv "$TMP_CHECKSUM_FILE" "SHA256SUMS.txt"
)
echo "✅ All requested packages are ready in: $OUTPUT_DIR"
