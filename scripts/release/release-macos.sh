#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
BUILD_DIR="$PROJECT_DIR/build/bin"
APP_PATH="$BUILD_DIR/FutrixData.app"
DMG_PATH="$BUILD_DIR/FutrixData.dmg"
APP_MAIN_BINARY="$APP_PATH/Contents/MacOS/FutrixData"
DEFAULT_NOTARY_DIR="${HOME}/macos"
TIMESTAMP_URL="${MACOS_TIMESTAMP_URL:-http://timestamp.apple.com/ts01}"
TARGET_PLATFORM=""
SIGN_IDENTITY="${MACOS_SIGN_IDENTITY:-}"
NOTARY_KEY="${NOTARY_KEY:-}"
NOTARY_KEY_ID="${NOTARY_KEY_ID:-}"
NOTARY_ISSUER_ID="${NOTARY_ISSUER_ID:-}"
BUILD_SCRIPT=""

# shellcheck source=/dev/null
source "$SCRIPT_DIR/release-common.sh"

usage() {
  cat <<EOF
Usage: $(basename "$0") --platform darwin/arm64|darwin/amd64 [--project-dir /path/to/FutrixData]

Environment overrides:
  MACOS_SIGN_IDENTITY   Explicit codesign identity
  MACOS_TIMESTAMP_URL   codesign timestamp URL (default: http://timestamp.apple.com/ts01)
  NOTARY_KEY            Path to App Store Connect .p8 key (default: ~/macos/AuthKey_*.p8)
  NOTARY_KEY_ID         App Store Connect key ID (default: parse ~/macos/key.md)
  NOTARY_ISSUER_ID      App Store Connect issuer ID (default: parse ~/macos/key.md)
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    --platform)
      TARGET_PLATFORM="${2:-}"
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

if [ -z "$TARGET_PLATFORM" ]; then
  echo "❌ --platform is required." >&2
  usage >&2
  exit 1
fi

BUILD_DIR="$PROJECT_DIR/build/bin"
APP_PATH="$BUILD_DIR/FutrixData.app"
DMG_PATH="$BUILD_DIR/FutrixData.dmg"
APP_MAIN_BINARY="$APP_PATH/Contents/MacOS/FutrixData"
BUILD_SCRIPT="${FUTRIXDATA_BUILD_SCRIPT:-$PROJECT_DIR/scripts/build.sh}"

if [ ! -d "$PROJECT_DIR" ]; then
  echo "❌ Project directory not found: $PROJECT_DIR" >&2
  exit 1
fi

if [ ! -x "$BUILD_SCRIPT" ]; then
  echo "❌ Build script not found or not executable: $BUILD_SCRIPT" >&2
  exit 1
fi

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "❌ Missing required command: $1" >&2
    exit 1
  fi
}

require_command codesign
require_command xcrun
require_command security
require_command awk
require_command sed
require_command find

if [ -z "$SIGN_IDENTITY" ]; then
  SIGN_IDENTITY="$(security find-identity -v -p codesigning 2>/dev/null | sed -n 's/.*"\(Developer ID Application:.*\)"/\1/p' | head -n 1)"
fi

if [ -z "$SIGN_IDENTITY" ]; then
  echo "❌ No Developer ID Application identity found in the local keychain." >&2
  exit 1
fi

KEY_MD="${DEFAULT_NOTARY_DIR}/key.md"
if [ -z "$NOTARY_KEY" ]; then
  NOTARY_KEY="$(find "${DEFAULT_NOTARY_DIR}" -maxdepth 1 -name 'AuthKey_*.p8' | head -n 1 || true)"
fi
if [ -z "$NOTARY_KEY_ID" ] && [ -f "$KEY_MD" ]; then
  NOTARY_KEY_ID="$(awk -F': ' '$1=="keyId"{print $2}' "$KEY_MD")"
fi
if [ -z "$NOTARY_ISSUER_ID" ] && [ -f "$KEY_MD" ]; then
  NOTARY_ISSUER_ID="$(awk -F': ' '$1=="issuerID"{print $2}' "$KEY_MD")"
fi

if [ ! -f "$NOTARY_KEY" ] || [ -z "$NOTARY_KEY_ID" ]; then
  echo "❌ Notarization credentials are incomplete." >&2
  echo "   Need NOTARY_KEY (.p8) and NOTARY_KEY_ID. NOTARY_ISSUER_ID is optional for individual App Store Connect keys." >&2
  exit 1
fi

ARCH_SUFFIX="${TARGET_PLATFORM#*/}"
FINAL_DMG_PATH="$BUILD_DIR/$(artifact_filename "$PROJECT_DIR" "macos-${ARCH_SUFFIX}")"
VERIFY_MOUNT_POINT=""

cleanup() {
  if [ -n "${VERIFY_MOUNT_POINT}" ] && mount | grep -Fq "on ${VERIFY_MOUNT_POINT} "; then
    hdiutil detach "${VERIFY_MOUNT_POINT}" >/dev/null 2>&1 || true
  fi
  if [ -n "${VERIFY_MOUNT_POINT}" ]; then
    rmdir "${VERIFY_MOUNT_POINT}" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

sign_path() {
  local target="$1"
  codesign \
    --force \
    --timestamp="$TIMESTAMP_URL" \
    --options runtime \
    --sign "$SIGN_IDENTITY" \
    "$target"
}

remove_signature() {
  local target="$1"
  codesign --remove-signature "$target" 2>/dev/null || true
}

echo "🔐 Using codesign identity: $SIGN_IDENTITY"
echo "⏱️  Using timestamp URL: $TIMESTAMP_URL"
echo "🧾 Using direct notary credentials from key file."
echo "🏗️  Building app for $TARGET_PLATFORM..."
"$BUILD_SCRIPT" --platform "$TARGET_PLATFORM"

if [ ! -d "$APP_PATH" ]; then
  echo "❌ App bundle not found at $APP_PATH" >&2
  exit 1
fi

echo "🧹 Removing stale signatures..."
remove_signature "$APP_PATH"
while IFS= read -r nested_binary; do
  remove_signature "$nested_binary"
done < <(find "$APP_PATH/Contents/MacOS" -type f | sort)

echo "✍️  Signing nested executables..."
while IFS= read -r nested_binary; do
  echo "   • $nested_binary"
  sign_path "$nested_binary"
done < <(find "$APP_PATH/Contents/MacOS" -type f ! -path "$APP_MAIN_BINARY" | sort)

echo "   • $APP_MAIN_BINARY"
sign_path "$APP_MAIN_BINARY"

echo "✍️  Signing app bundle..."
sign_path "$APP_PATH"

echo "🔎 Verifying app signature..."
codesign --verify --deep --strict --verbose=2 "$APP_PATH"

echo "📦 Building DMG..."
FUTRIXDATA_SKIP_DMG_CODESIGN=1 "$SCRIPT_DIR/build-dmg.sh" --project-dir "$PROJECT_DIR"

if [ ! -f "$DMG_PATH" ]; then
  echo "❌ DMG not found at $DMG_PATH" >&2
  exit 1
fi

echo "✍️  Signing DMG..."
codesign --force --timestamp="$TIMESTAMP_URL" --sign "$SIGN_IDENTITY" "$DMG_PATH"

echo "🔎 Verifying DMG signature..."
codesign --verify --verbose=2 "$DMG_PATH"

submit_args=(
  notarytool submit "$DMG_PATH"
  --key "$NOTARY_KEY"
  --key-id "$NOTARY_KEY_ID"
  --wait
)
if [ -n "$NOTARY_ISSUER_ID" ]; then
  submit_args+=(--issuer "$NOTARY_ISSUER_ID")
fi
echo "☁️  Submitting DMG for notarization..."
xcrun "${submit_args[@]}"

echo "📎 Stapling notarization ticket..."
xcrun stapler staple -v "$DMG_PATH"

echo "✅ Validating stapled ticket..."
xcrun stapler validate -v "$DMG_PATH"

echo "🛡️  Gatekeeper check on mounted app..."
VERIFY_MOUNT_POINT="$(mktemp -d /tmp/futrix-release-verify.XXXXXX)"
hdiutil attach -nobrowse -readonly -mountpoint "$VERIFY_MOUNT_POINT" "$DMG_PATH" >/dev/null
spctl -a -vv "$VERIFY_MOUNT_POINT/FutrixData.app"
hdiutil detach "$VERIFY_MOUNT_POINT" >/dev/null
rmdir "$VERIFY_MOUNT_POINT" >/dev/null 2>&1 || true
VERIFY_MOUNT_POINT=""

mv -f "$DMG_PATH" "$FINAL_DMG_PATH"

echo ""
echo "✅ Signed and notarized DMG ready: $FINAL_DMG_PATH"
