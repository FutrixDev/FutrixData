#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
SITE_URL="https://futrixdata.com"
DOWNLOAD_DIR=""
KEEP_DOWNLOADS=0
OWN_DOWNLOAD_DIR=0
PLATFORMS=()
PLIST_BUDDY_BIN="${PLIST_BUDDY_BIN:-/usr/libexec/PlistBuddy}"

# shellcheck source=/dev/null
source "$SCRIPT_DIR/release-common.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") [options]

Options:
  --project-dir DIR         FutrixData project root (default: current directory)
  --site-url URL            Download site base URL (default: https://futrixdata.com)
  --platform KEY            Repeat to verify specific platforms only
  --download-dir DIR        Keep downloads in DIR instead of a temp directory
  --keep-downloads          Do not delete the download directory on exit
  -h, --help                Show this help

Platforms:
  macos-arm64
  macos-amd64
  windows-amd64
  linux-amd64
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    --site-url)
      SITE_URL="${2:-}"
      shift 2
      ;;
    --platform)
      PLATFORMS+=("${2:-}")
      shift 2
      ;;
    --download-dir)
      DOWNLOAD_DIR="${2:-}"
      shift 2
      ;;
    --keep-downloads)
      KEEP_DOWNLOADS=1
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

require_command curl
require_command python3
require_command tar

if [ "${#PLATFORMS[@]}" -eq 0 ]; then
  PLATFORMS=("macos-arm64" "macos-amd64" "windows-amd64" "linux-amd64")
fi

if [ -z "$DOWNLOAD_DIR" ]; then
  DOWNLOAD_DIR="$(mktemp -d /tmp/futrix-live-downloads.XXXXXX)"
  OWN_DOWNLOAD_DIR=1
else
  mkdir -p "$DOWNLOAD_DIR"
fi

cleanup() {
  if [ "$KEEP_DOWNLOADS" != "1" ] && [ "$OWN_DOWNLOAD_DIR" = "1" ]; then
    rm -rf "$DOWNLOAD_DIR"
  fi
}

trap cleanup EXIT

RELEASE_VERSION="$(release_version_tag "$PROJECT_DIR")"
PRODUCT_VERSION="$(read_product_version "$PROJECT_DIR")"
PRODUCT_VERSION="${PRODUCT_VERSION#v}"

download_and_assert() {
  local platform="$1"
  local expected_file="$2"
  local expected_url="$SITE_URL/api/download/$platform"
  local final_url
  local final_url_path
  local destination="$DOWNLOAD_DIR/$expected_file"

  echo "⬇️  Downloading $platform..."
  final_url="$(curl -fsSL -o /dev/null -w '%{url_effective}' "$expected_url")"
  final_url_path="${final_url%%\?*}"

  case "$final_url_path" in
    *"/releases/download/$RELEASE_VERSION/$expected_file")
      ;;
    "https://release-assets.githubusercontent.com/"*)
      case "$final_url" in
        *"filename%3D$expected_file"*|*"filename=$expected_file"*)
          ;;
        *)
          echo "❌ Unexpected release asset filename for $platform: $final_url" >&2
          exit 1
          ;;
      esac
      ;;
    *)
      echo "❌ Unexpected final URL for $platform: $final_url" >&2
      exit 1
      ;;
  esac

  curl -fsSL "$expected_url" -o "$destination"

  if [ ! -s "$destination" ]; then
    echo "❌ Downloaded file is empty: $destination" >&2
    exit 1
  fi

  echo "✅ Downloaded: $destination"
}

verify_macos_dmg_version() {
  local dmg_path="$1"
  local mount_point
  local plist_path
  local shipped_version

  require_command hdiutil
  if [ ! -x "$PLIST_BUDDY_BIN" ]; then
    echo "❌ PlistBuddy not found or not executable: $PLIST_BUDDY_BIN" >&2
    exit 1
  fi
  mount_point="$(mktemp -d /tmp/futrix-live-verify.XXXXXX)"
  hdiutil attach -nobrowse -readonly -mountpoint "$mount_point" "$dmg_path" >/dev/null
  plist_path="$mount_point/FutrixData.app/Contents/Info.plist"

  if [ ! -f "$plist_path" ]; then
    hdiutil detach "$mount_point" >/dev/null 2>&1 || true
    rmdir "$mount_point" >/dev/null 2>&1 || true
    echo "❌ Missing Info.plist in mounted DMG: $plist_path" >&2
    exit 1
  fi

  shipped_version="$("$PLIST_BUDDY_BIN" -c 'Print :CFBundleShortVersionString' "$plist_path")"
  hdiutil detach "$mount_point" >/dev/null
  rmdir "$mount_point" >/dev/null 2>&1 || true

  if [ "$shipped_version" != "$PRODUCT_VERSION" ]; then
    echo "❌ DMG version mismatch: expected $PRODUCT_VERSION, got $shipped_version" >&2
    exit 1
  fi

  echo "✅ DMG app version matches: $shipped_version"
}

verify_linux_bundle_layout() {
  local archive_path="$1"
  local expected_root
  local root_entry

  expected_root="$(linux_bundle_dirname "$PROJECT_DIR")"
  root_entry="$(tar -tzf "$archive_path" | head -n 1 | cut -d/ -f1)"

  if [ "$root_entry" != "$expected_root" ]; then
    echo "❌ Linux archive root mismatch: expected $expected_root, got $root_entry" >&2
    exit 1
  fi

  echo "✅ Linux archive root matches: $root_entry"
}

for platform in "${PLATFORMS[@]}"; do
  expected_file="$(artifact_filename "$PROJECT_DIR" "$platform")"
  download_and_assert "$platform" "$expected_file"

  case "$platform" in
    macos-arm64|macos-amd64)
      verify_macos_dmg_version "$DOWNLOAD_DIR/$expected_file"
      ;;
    linux-amd64)
      verify_linux_bundle_layout "$DOWNLOAD_DIR/$expected_file"
      ;;
    windows-amd64)
      echo "✅ Windows installer filename contains release version"
      ;;
    *)
      echo "❌ Unsupported platform: $platform" >&2
      exit 1
      ;;
  esac
done

echo "🎉 Live download verification passed for release $RELEASE_VERSION"
