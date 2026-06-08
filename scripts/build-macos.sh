#!/usr/bin/env bash
# Build, sign, package (.dmg), notarize, and staple the macOS GUI.
# Run ON A MAC. Required env:
#   VER                     release version, e.g. v0.1.0 (default: git describe)
#   APPLE_SIGN_IDENTITY     "Developer ID Application: NAME (TEAMID)"
#   APPLE_ID                Apple account email
#   APPLE_TEAM_ID           10-char team id
#   APPLE_APP_PASSWORD      app-specific password for notarytool
set -euo pipefail

VER="${VER:-$(git describe --tags --always)}"
: "${APPLE_SIGN_IDENTITY:?set APPLE_SIGN_IDENTITY}"
: "${APPLE_ID:?set APPLE_ID}"
: "${APPLE_TEAM_ID:?set APPLE_TEAM_ID}"
: "${APPLE_APP_PASSWORD:?set APPLE_APP_PASSWORD}"

OUT="build/release"
APP="build/bin/vless-client.app"
DMG="${OUT}/vless-client-${VER}-macos-universal.dmg"
mkdir -p "$OUT"

wails build -platform darwin/universal -tags wails \
  -ldflags "-X main.version=${VER}"

codesign --deep --force --options runtime --timestamp \
  --sign "${APPLE_SIGN_IDENTITY}" "${APP}"

# Build the DMG (uses hdiutil; a staging dir keeps the layout minimal).
STAGE="$(mktemp -d)"
cp -R "${APP}" "${STAGE}/"
ln -s /Applications "${STAGE}/Applications"
hdiutil create -volname "VLESS Client" -srcfolder "${STAGE}" \
  -ov -format UDZO "${DMG}"
rm -rf "${STAGE}"

codesign --force --timestamp --sign "${APPLE_SIGN_IDENTITY}" "${DMG}"

xcrun notarytool submit "${DMG}" \
  --apple-id "${APPLE_ID}" --team-id "${APPLE_TEAM_ID}" \
  --password "${APPLE_APP_PASSWORD}" --wait

xcrun stapler staple "${DMG}"

echo "macOS artifact: ${DMG} (signed + notarized)"
