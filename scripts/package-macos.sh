#!/usr/bin/env bash

set -euo pipefail

SOURCE_DIR="${SOURCE_DIR:-source}"
OUTPUT_DIR="${OUTPUT_DIR:-dist/macos}"
VERSION="${VERSION:?VERSION is required}"
PRODUCT_NAME="${PRODUCT_NAME:-$(jq -r '.outputfilename // .name' "$SOURCE_DIR/wails.json")}"
MACOS_WAILS_PLATFORM="${MACOS_WAILS_PLATFORM:-darwin/universal}"
MACOS_BUNDLE_ID="${MACOS_BUNDLE_ID:-com.futrixdata.app}"

mkdir -p "$OUTPUT_DIR"

pushd "$SOURCE_DIR" >/dev/null
wails build -clean -platform "$MACOS_WAILS_PLATFORM"
popd >/dev/null

APP_PATH="$SOURCE_DIR/build/bin/${PRODUCT_NAME}.app"
if [ ! -d "$APP_PATH" ]; then
  echo "Expected app bundle not found at $APP_PATH" >&2
  exit 1
fi

PLIST_PATH="$APP_PATH/Contents/Info.plist"
if [ -f "$PLIST_PATH" ]; then
  /usr/libexec/PlistBuddy -c "Set :CFBundleIdentifier $MACOS_BUNDLE_ID" "$PLIST_PATH" \
    || /usr/libexec/PlistBuddy -c "Add :CFBundleIdentifier string $MACOS_BUNDLE_ID" "$PLIST_PATH"
fi

codesign \
  --force \
  --deep \
  --options runtime \
  --timestamp \
  --sign "$MACOS_SIGNING_IDENTITY" \
  "$APP_PATH"

codesign --verify --deep --strict --verbose=2 "$APP_PATH"

DMG_NAME="${PRODUCT_NAME}-${VERSION}-macos.dmg"
ZIP_NAME="${PRODUCT_NAME}-${VERSION}-macos.zip"
DMG_PATH="$OUTPUT_DIR/$DMG_NAME"
ZIP_PATH="$OUTPUT_DIR/$ZIP_NAME"

STAGING_DIR="$(mktemp -d)"
cp -R "$APP_PATH" "$STAGING_DIR/"
hdiutil create \
  -volname "$PRODUCT_NAME" \
  -srcfolder "$STAGING_DIR" \
  -ov \
  -format UDZO \
  "$DMG_PATH"
rm -rf "$STAGING_DIR"

xcrun notarytool submit \
  "$DMG_PATH" \
  --key "$MACOS_NOTARY_KEY_FILE" \
  --key-id "$MACOS_NOTARY_KEY_ID" \
  --issuer "$MACOS_NOTARY_ISSUER" \
  --wait

xcrun stapler staple "$APP_PATH"
xcrun stapler staple "$DMG_PATH"

ditto -c -k --keepParent "$APP_PATH" "$ZIP_PATH"
