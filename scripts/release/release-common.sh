#!/bin/bash
set -euo pipefail

read_product_version() {
  local project_dir="$1"
  local wails_config="$project_dir/wails.json"

  if [ ! -f "$wails_config" ]; then
    echo "❌ Missing wails.json: $wails_config" >&2
    return 1
  fi

  python3 - "$wails_config" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as fh:
    data = json.load(fh)

version = data.get("info", {}).get("productVersion") or data.get("productVersion")
if not version:
    raise SystemExit("Missing productVersion in wails.json")

print(version)
PY
}

release_version_tag() {
  local raw_version
  raw_version="$(read_product_version "$1")"
  raw_version="${raw_version#v}"
  printf 'v%s\n' "$raw_version"
}

artifact_filename() {
  local project_dir="$1"
  local platform="$2"
  local release_version
  release_version="$(release_version_tag "$project_dir")"

  case "$platform" in
    macos-arm64)
      printf 'FutrixData-%s-macos-arm64.dmg\n' "$release_version"
      ;;
    macos-amd64)
      printf 'FutrixData-%s-macos-amd64.dmg\n' "$release_version"
      ;;
    windows-amd64)
      printf 'FutrixData-%s-windows-amd64-installer.exe\n' "$release_version"
      ;;
    linux-amd64)
      printf 'FutrixData-%s-linux-amd64.tar.gz\n' "$release_version"
      ;;
    *)
      echo "❌ Unsupported platform key: $platform" >&2
      return 1
      ;;
  esac
}

linux_bundle_dirname() {
  local project_dir="$1"
  local release_version
  release_version="$(release_version_tag "$project_dir")"
  printf 'FutrixData-%s-linux-amd64\n' "$release_version"
}
