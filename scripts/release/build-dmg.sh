#!/bin/bash
set -e

# Build macOS DMG installer for FutrixData
# Usage: build-dmg.sh [--project-dir /path/to/FutrixData]

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(pwd)"
BUILD_DIR="$PROJECT_DIR/build/bin"
DMG_OUTPUT="$BUILD_DIR/FutrixData.dmg"
APP_NAME="FutrixData.app"
APP_PATH="$BUILD_DIR/$APP_NAME"
INFO_PLIST="$APP_PATH/Contents/Info.plist"
RW_DMG="$BUILD_DIR/FutrixData-rw.dmg"
PATCHED_DMG="$BUILD_DIR/FutrixData-patched.dmg"
APP_LINK_ICON="/System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/ApplicationsFolderIcon.icns"

while [ "$#" -gt 0 ]; do
  case "$1" in
    --project-dir)
      PROJECT_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      echo "Usage: $(basename "$0") [--project-dir /path/to/FutrixData]"
      exit 0
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 1
      ;;
  esac
done

BUILD_DIR="$PROJECT_DIR/build/bin"
DMG_OUTPUT="$BUILD_DIR/FutrixData.dmg"
APP_PATH="$BUILD_DIR/$APP_NAME"
INFO_PLIST="$APP_PATH/Contents/Info.plist"
RW_DMG="$BUILD_DIR/FutrixData-rw.dmg"
PATCHED_DMG="$BUILD_DIR/FutrixData-patched.dmg"

if [ ! -d "$PROJECT_DIR" ]; then
  echo "❌ Project directory not found: $PROJECT_DIR" >&2
  exit 1
fi

cleanup() {
  if [ -n "${MOUNT_POINT:-}" ] && mount | grep -Fq "on ${MOUNT_POINT} "; then
    hdiutil detach "$MOUNT_POINT" >/dev/null 2>&1 || true
  fi
  rm -f "${RW_DMG}" "${RW_DMG}.dmgpart" "${PATCHED_DMG}" "${PATCHED_DMG}.dmgpart"
  rm -f "${TMP_ICON_FILE:-}" "${TMP_ICON_RSRC:-}"
  if [ -n "${MOUNT_POINT:-}" ]; then
    rmdir "$MOUNT_POINT" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT

if ! command -v create-dmg >/dev/null 2>&1; then
  echo "❌ create-dmg is not installed."
  echo "   Install it with: npm install -g create-dmg@8.1.0"
  exit 1
fi

if ! command -v npx >/dev/null 2>&1; then
  echo "❌ npx is required to create the Applications alias."
  exit 1
fi

CREATE_DMG_VERSION="$(create-dmg --version)"
CREATE_DMG_MAJOR="${CREATE_DMG_VERSION%%.*}"
if [ "${CREATE_DMG_MAJOR}" -lt 8 ]; then
  echo "❌ create-dmg ${CREATE_DMG_VERSION} is too old."
  echo "   Upgrade it with: npm install -g create-dmg@8.1.0"
  exit 1
fi

# Check that the .app exists
if [ ! -d "$APP_PATH" ]; then
  echo "❌ $APP_PATH not found."
  echo "   Run 'wails build -platform darwin/amd64' first."
  exit 1
fi

if ! plutil -lint "$INFO_PLIST" >/dev/null; then
  echo "❌ $INFO_PLIST is invalid."
  echo "   Rebuild the app after fixing wails.json or Info.dev.plist."
  exit 1
fi

DMG_ARGS=(
  --overwrite
  --no-version-in-filename
  --dmg-title "FutrixData Installer"
)

if [ "${FUTRIXDATA_SKIP_DMG_CODESIGN:-0}" = "1" ]; then
  echo "ℹ️  FUTRIXDATA_SKIP_DMG_CODESIGN=1. DMG signing will be skipped."
  DMG_ARGS+=(--no-code-sign)
elif ! security find-identity -v -p codesigning 2>/dev/null | grep -q "^[[:space:]]*[0-9]\+) "; then
  echo "ℹ️  No local code signing identity found. DMG signing will be skipped."
  DMG_ARGS+=(--no-code-sign)
fi

# Create DMG
echo "💿 Creating DMG..."
create-dmg "${DMG_ARGS[@]}" "$APP_PATH" "$BUILD_DIR"

echo "🎨 Replacing Applications target with a custom-icon alias..."
rm -f "$RW_DMG"
hdiutil convert "$DMG_OUTPUT" -format UDRW -o "$RW_DMG" >/dev/null
MOUNT_POINT="$(mktemp -d /tmp/futrixdmg-patch.XXXXXX)"
hdiutil attach -readwrite -noverify -noautoopen -mountpoint "$MOUNT_POINT" "${RW_DMG}" >/dev/null
rm -f "$MOUNT_POINT/Applications"
npx --yes mkalias /Applications "$MOUNT_POINT/Applications" >/dev/null
TMP_ICON_FILE="$(mktemp /tmp/futrix-applications-icon.XXXXXX.icns)"
TMP_ICON_RSRC="$(mktemp /tmp/futrix-applications-icon.XXXXXX.rsrc)"
cp "$APP_LINK_ICON" "$TMP_ICON_FILE"
sips -i "$TMP_ICON_FILE" >/dev/null
DeRez -only icns "$TMP_ICON_FILE" > "$TMP_ICON_RSRC"
Rez -append "$TMP_ICON_RSRC" -o "$MOUNT_POINT/Applications" >/dev/null
SetFile -a C "$MOUNT_POINT/Applications"
hdiutil detach "$MOUNT_POINT" >/dev/null
rmdir "$MOUNT_POINT" >/dev/null 2>&1 || true
MOUNT_POINT=""
rm -f "$PATCHED_DMG"
hdiutil convert "${RW_DMG}" -format UDZO -o "$PATCHED_DMG" >/dev/null
mv -f "$PATCHED_DMG" "$DMG_OUTPUT"
rm -f "$RW_DMG"

echo ""
echo "✅ DMG created: $DMG_OUTPUT"
echo "   Size: $(du -h "$DMG_OUTPUT" | cut -f1)"
